package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"gitlab.com/prestrafe/prestrafe-gsi/gsistore"
	"gitlab.com/prestrafe/prestrafe-gsi/model"
	"gitlab.com/prestrafe/prestrafe-gsi/smstore"
)

// Defines the public API for the Game State Integration server. The server acts as a rely between the CSGO GSI API,
// which sends game state data to a configured web-hook and potential clients, which may wish to consume this data as a
// service, without providing their own HTTP server. The GSI server supports multiple tenants, which are identified by
// their authentication token, that is send with each GSI web-hook call.
type Server interface {
	// Starts the server in the current thread and blocks until an error occurs.
	Start() error
	// Stops the server
	Stop() error
}

type server struct {
	addr       string
	port       int
	filter     TokenFilter
	logger     *log.Logger
	gsiStore   gsistore.Store
	smStore    smstore.Store
	httpServer *http.Server
	upgrader   *websocket.Upgrader
}

// Creates a new GSI server, listening on a given address and port. The TTL controls for how long game states should be
// kept, until they are considered stale.
func New(addr string, port, ttl int, filter TokenFilter) Server {
	return &server{
		addr,
		port,
		filter,
		log.New(os.Stdout, "GSI-Server > ", log.LstdFlags),
		gsistore.New(time.Duration(ttl) * time.Second),
		smstore.New(time.Duration(ttl) * time.Second),
		nil,
		nil,
	}
}

func (s *server) Start() error {
	router := mux.NewRouter()

	// TODO I really want to change these routes, but I should wait until the web frontend is out and users need to
	//  change their config anyways.
	// router.Path("/").Methods("GET").HandlerFunc(s.handleGet)
	// router.Path("/").Methods("POST").HandlerFunc(s.handlePost)

	// GSI Handlers
	router.Path("/gsi/get").Methods("GET").HandlerFunc(s.handleGSIGet)
	router.Path("/gsi/update").Methods("POST").HandlerFunc(s.handleGSIPost)

	router.Path("/websocket").Methods("GET").HandlerFunc(s.handleWebsocket)

	// SM Handlers
	router.Path("/sm/update").Methods("POST").HandlerFunc(s.handleServerPost)
	router.Path("/sm/get").Methods("GET").HandlerFunc(s.handleServerGet)
	router.NotFoundHandler = http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		s.logger.Printf("Unmatched request: %s %s\n", request.Method, request.URL)
		writer.WriteHeader(http.StatusNotFound)
	})

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.addr, s.port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	s.upgrader = &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(request *http.Request) bool {
			return true
		},
	}

	s.logger.Printf("Starting GSI server on %s:%d\n", s.addr, s.port)
	return s.httpServer.ListenAndServe()
}

func (s *server) Stop() error {
	s.logger.Printf("Stopping GSI server on %s:%d\n", s.addr, s.port)

	s.gsiStore.Close()
	s.smStore.Close()
	return s.httpServer.Shutdown(context.Background())
}

func (s *server) handleGSIGet(writer http.ResponseWriter, request *http.Request) {
	if !strings.HasPrefix(request.Header.Get("Authorization"), "GSI ") {
		s.logger.Printf("%s - Unauthorized GSI read (no token)\n", request.RemoteAddr)
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}

	authToken := request.Header.Get("Authorization")[4:]
	if !s.filter.Accept(authToken) {
		s.logger.Printf("%s - Unauthorized GSI read (rejected token)\n", request.RemoteAddr)
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}

	gameState, hasGameState := s.gsiStore.Get(authToken)
	if !hasGameState {
		s.logger.Printf("%s - Unknown GSI read to %s\n", request.RemoteAddr, authToken)
		writer.WriteHeader(http.StatusNotFound)
		return
	}

	response, jsonError := json.Marshal(gameState)
	if jsonError != nil {
		s.logger.Printf("%s - Could not serialize game state %s: %s\n", request.RemoteAddr, authToken, jsonError)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)

	if _, ioError := writer.Write(response); ioError != nil {
		s.logger.Printf("%s - Could not write game state %s: %s\n", request.RemoteAddr, authToken, ioError)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *server) handleGSIPost(writer http.ResponseWriter, request *http.Request) {
	body, ioError := ioutil.ReadAll(request.Body)
	if ioError != nil || body == nil || len(body) <= 0 {
		s.logger.Printf("%s - Empty GSI update received: %s\n", request.RemoteAddr, ioError)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	gameState := new(model.GameState)
	if jsonError := json.Unmarshal(body, gameState); jsonError != nil {
		if jsonError.Error() != "json: cannot unmarshal bool into Go struct field GameState.previously.map of type model.MapState" {
			// Upon map change, instead of returning a map object the GSI client return a bool.
			// It's not necessary to log this error; we send 400 anyway to mark that the game state is not updated.
			s.logger.Printf("%s - Could not de-serialize game state: %s\n", request.RemoteAddr, jsonError)
		}
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	if gameState.Auth == nil {
		s.logger.Printf("%s - Game state did not contain auth information\n", request.RemoteAddr)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	authToken := gameState.Auth.Token
	gameState.Auth = nil

	if !s.filter.Accept(authToken) {
		s.logger.Printf("%s - Unauthorized GSI read (rejected token)\n", request.RemoteAddr)
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}

	if gameState.Provider != nil {
		s.gsiStore.Put(authToken, gameState)
	} else {
		s.gsiStore.Remove(authToken)
	}

	writer.WriteHeader(http.StatusOK)
}

func (s *server) handleServerGet(writer http.ResponseWriter, request *http.Request) {
	if !strings.HasPrefix(request.Header.Get("Authorization"), "SM ") {
		s.logger.Printf("%s - Unauthorized SM read (no token)\n", request.RemoteAddr)
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}

	authToken := request.Header.Get("Authorization")[3:]
	if !s.filter.Accept(authToken) {
		s.logger.Printf("%s - Unauthorized SM read (rejected token)\n", request.RemoteAddr)
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}

	fullPlayerState, hasFullPlayerState := s.smStore.Get(authToken)
	if !hasFullPlayerState {
		s.logger.Printf("%s - Unknown SM read to %s\n", request.RemoteAddr, authToken)
		writer.WriteHeader(http.StatusNotFound)
		return
	}

	response, jsonError := json.Marshal(fullPlayerState)
	if jsonError != nil {
		s.logger.Printf("%s - Could not serialize game state %s: %s\n", request.RemoteAddr, authToken, jsonError)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)

	if _, ioError := writer.Write(response); ioError != nil {
		s.logger.Printf("%s - Could not write game state %s: %s\n", request.RemoteAddr, authToken, ioError)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *server) handleServerPost(writer http.ResponseWriter, request *http.Request) {
	body, ioError := ioutil.ReadAll(request.Body)
	if ioError != nil || body == nil || len(body) <= 0 {
		s.logger.Printf("%s - Empty SM update received: %s\n", request.RemoteAddr, ioError)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	serverState := new(model.ServerState)
	if jsonError := json.Unmarshal(body, serverState); jsonError != nil {
		s.logger.Printf("%s - Could not de-serialize server state: %s\n", request.RemoteAddr, jsonError)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	serverInfo := serverState.ServerInfo

	playerInfos := serverState.PlayerInfo

	for _, player := range playerInfos {
		if player.AuthKey != "" {
			s.smStore.Put(&serverInfo, &player)
		}
	}

	writer.WriteHeader(http.StatusOK)
}

func (s *server) handleWebsocket(writer http.ResponseWriter, request *http.Request) {
	authToken := request.Header.Get("Sec-WebSocket-Protocol")
	if authToken == "" {
		s.logger.Printf("%s - Unauthorized GSI websocket read (no token)\n", request.RemoteAddr)
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}

	if !s.filter.Accept(authToken) {
		s.logger.Printf("%s - Unauthorized GSI read (rejected token)\n", request.RemoteAddr)
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}

	conn, upgradeError := s.upgrader.Upgrade(writer, request, http.Header{
		"Sec-Websocket-Protocol": []string{authToken},
	})
	if upgradeError != nil {
		s.logger.Printf("%s - Could not upgrade websocket connection on %s: %s\n", request.RemoteAddr, authToken, upgradeError)
		_ = conn.Close()
		return
	}

	channel := s.gsiStore.GetChannel(authToken)

	for {
		select {
		case gameState, more := <-channel:
			if ioError := conn.WriteJSON(gameState); ioError != nil || !more {
				if ioError != nil {
					s.logger.Printf("%s - Could not serialize game state %s: %s\n", request.RemoteAddr, authToken, ioError)
				}
				_ = conn.Close()
				s.gsiStore.ReleaseChannel(authToken)
				return
			}
		}
	}
}
