package gsi

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
	logger     *log.Logger
	store      Store
	httpServer *http.Server
	upgrader   *websocket.Upgrader
}

// Creates a new GSI server, listening on a given address and port. The TTL controls for how long game states should be
// kept, until they are considered stale.
func NewServer(addr string, port, ttl int) Server {
	return &server{
		addr,
		port,
		log.New(os.Stdout, "GSI-Server", log.LstdFlags),
		NewStore(time.Duration(ttl) * time.Second),
		nil,
		nil,
	}
}

func (s *server) Start() error {
	router := mux.NewRouter()
	router.Path("/").Methods("GET").HandlerFunc(s.handleGet)
	router.Path("/").Methods("POST").HandlerFunc(s.handlePost)
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
	return s.httpServer.Shutdown(context.Background())
}

func (s *server) handleGet(writer http.ResponseWriter, request *http.Request) {
	if !strings.HasPrefix(request.Header.Get("Authorization"), "GSI ") {
		s.logger.Printf("%s - Unauthorized GSI read\n", request.RemoteAddr)
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}

	authToken := request.Header.Get("Authorization")[4:]
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

	gameState := new(GameState)
	if jsonError := json.Unmarshal(body, gameState); jsonError != nil {
		s.logger.Printf("%s - Could not de-serialize game state: %s\n", request.RemoteAddr, jsonError)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	authToken := gameState.Auth.Token
	gameState.Auth = nil

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
		s.logger.Printf("%s - Unauthorized GSI websocket read\n", request.RemoteAddr)
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}

	conn, upgradeError := s.upgrader.Upgrade(writer, request, nil)
	if upgradeError != nil {
		s.logger.Printf("%s - Could not upgrade websocket connection on %s: %s\n", request.RemoteAddr, authToken, upgradeError)
		_ = conn.Close()
		return
	}

	channel := s.store.Channel(authToken)

	for {
		select {
		case gameState, more := <-channel:
			if ioError := conn.WriteJSON(gameState); ioError != nil || !more {
				if ioError != nil {
					s.logger.Printf("%s - Could not serialize game state %s: %s\n", request.RemoteAddr, authToken, ioError)
				}
				_ = conn.Close()
				return
			}
		}
	}
}
