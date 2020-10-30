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

	"gitlab.com/prestrafe/prestrafe-gsi/model"
	"gitlab.com/prestrafe/prestrafe-gsi/store"
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
	store      store.Store
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
		store.New(time.Duration(ttl) * time.Second),
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

	router.Path("/get").Methods("GET").HandlerFunc(s.handleGet)
	router.Path("/update").Methods("POST").HandlerFunc(s.handlePost)
	router.Path("/websocket").Methods("GET").HandlerFunc(s.handleWebsocket)
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

	s.store.Close()
	return s.httpServer.Shutdown(context.Background())
}

func (s *server) handleGet(writer http.ResponseWriter, request *http.Request) {
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

	gameState, hasGameState := s.store.Get(authToken)
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

func (s *server) handlePost(writer http.ResponseWriter, request *http.Request) {
	body, ioError := ioutil.ReadAll(request.Body)
	if ioError != nil || body == nil || len(body) <= 0 {
		s.logger.Printf("%s - Empty GSI update received: %s\n", request.RemoteAddr, ioError)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	gameState := new(model.GameState)
	if jsonError := json.Unmarshal(body, gameState); jsonError != nil {
		s.logger.Printf("%s - Could not de-serialize game state: %s\n", request.RemoteAddr, jsonError)
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
		s.store.Put(authToken, gameState)
	} else {
		s.store.Remove(authToken)
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

	channel := s.store.GetChannel(authToken)

	for {
		select {
		case gameState, more := <-channel:
			if ioError := conn.WriteJSON(gameState); ioError != nil || !more {
				if ioError != nil {
					s.logger.Printf("%s - Could not serialize game state %s: %s\n", request.RemoteAddr, authToken, ioError)
				}
				_ = conn.Close()
				s.store.ReleaseChannel(authToken)
				return
			}
		}
	}
}
