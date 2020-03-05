package gsi

import (
	"reflect"
	"time"

	"github.com/patrickmn/go-cache"
)

// Defines the public API for the GSI store. The store is responsible for saving game states and evicting them once they
// go stale. Additional the store provides a channel object, that can be used to get notified, if a game state updates.
type Store interface {
	Channel(authToken string) chan *GameState
	// Returns a game state for the given auth token, if one is present.
	Get(authToken string) (gameState *GameState, present bool)
	// Puts a new game state for the given auth token, if none is already present. Otherwise the existing game state
	// will be updated with the passed one.
	Put(authToken string, gameState *GameState)
	// Removes a game state for the given auth token, if one is present.
	Remove(authToken string)
}

type store struct {
	channels      map[string]chan *GameState
	internalCache *cache.Cache
}

// Creates a new GSI store, with a given TTL. The TTL is the duration for game states, before they are considered stale.
func NewStore(ttl time.Duration) Store {
	internalCache := cache.New(ttl, ttl*10)
	channels := make(map[string]chan *GameState)
	store := &store{channels, internalCache}

	internalCache.OnEvicted(func(authToken string, item interface{}) {
		channels[authToken] <- new(GameState)
	})

	return store
}

func (s *store) Channel(authToken string) chan *GameState {
	return s.channels[authToken]
}

func (s *store) Get(authToken string) (gameState *GameState, present bool) {
	if cached, isCached := s.internalCache.Get(authToken); isCached {
		gameState = cached.(*GameState)
		present = isCached
	}
	return
}

func (s *store) Put(authToken string, gameState *GameState) {
	if _, present := s.channels[authToken]; !present {
		// TODO These channels need to be cleaned up after awhile or do they?
		s.channels[authToken] = make(chan *GameState)
	}

	previousGameState, _ := s.internalCache.Get(authToken)
	s.internalCache.Set(authToken, gameState, cache.DefaultExpiration)

	if !reflect.DeepEqual(previousGameState, gameState) {
		s.channels[authToken] <- gameState
	}
}

func (s *store) Remove(authToken string) {
	s.internalCache.Delete(authToken)
}
