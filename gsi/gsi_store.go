package gsi

import (
	"reflect"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
)

// Defines the public API for the GSI store. The store is responsible for saving game states and evicting them once they
// go stale. Additional the store provides a channel object, that can be used to get notified, if a game state updates.
type Store interface {
	// Returns a channel that is filled with updates of the game state for the given auth token. Calling this method
	// also means that the caller needs to call ReleaseChannel(authToken), once he is done with using the channel.
	GetChannel(authToken string) chan *GameState
	// Releases a channel that was previously acquired by GetChannel(authToken).
	ReleaseChannel(authToken string)
	// Returns a game state for the given auth token, if one is present.
	Get(authToken string) (gameState *GameState, present bool)
	// Puts a new game state for the given auth token, if none is already present. Otherwise the existing game state
	// will be updated with the passed one.
	Put(authToken string, gameState *GameState)
	// Removes a game state for the given auth token, if one is present.
	Remove(authToken string)
	// Closes the store and releases all resources held by it.
	Close()
}

type store struct {
	channels      map[string]*channelContainer
	internalCache *cache.Cache
	locker        sync.Locker
}

type channelContainer struct {
	channel chan *GameState
	clients int
}

// Creates a new GSI store, with a given TTL. The TTL is the duration for game states, before they are considered stale.
func NewStore(ttl time.Duration) Store {
	internalCache := cache.New(ttl, ttl*10)
	channels := make(map[string]*channelContainer)
	store := &store{channels, internalCache, &sync.Mutex{}}

	internalCache.OnEvicted(func(authToken string, item interface{}) {
		store.pushUpdate(authToken, nil)
	})

	return store
}

func (s *store) GetChannel(authToken string) chan *GameState {
	s.locker.Lock()

	if _, present := s.channels[authToken]; !present {
		gameState, _ := s.Get(authToken)

		s.channels[authToken] = &channelContainer{make(chan *GameState), 0}
		s.channels[authToken].channel <- gameState
	}

	container := s.channels[authToken]
	container.clients++

	s.locker.Unlock()

	return container.channel
}

func (s *store) ReleaseChannel(authToken string) {
	if _, present := s.channels[authToken]; present {
		s.locker.Lock()

		if container, present := s.channels[authToken]; present {
			container.clients--
			if container.clients < 1 {
				delete(s.channels, authToken)
				close(container.channel)
			}
		}

		s.locker.Unlock()
	}
}

func (s *store) Get(authToken string) (gameState *GameState, present bool) {
	if cached, isCached := s.internalCache.Get(authToken); isCached {
		gameState = cached.(*GameState)
		present = isCached
	}
	return
}

func (s *store) Put(authToken string, gameState *GameState) {
	previousGameState, _ := s.internalCache.Get(authToken)
	s.internalCache.Set(authToken, gameState, cache.DefaultExpiration)

	if !reflect.DeepEqual(previousGameState, gameState) {
		s.pushUpdate(authToken, gameState)
	}
}

func (s *store) Remove(authToken string) {
	s.internalCache.Delete(authToken)
}

func (s *store) Close() {
	for authToken, channelContainer := range s.channels {
		delete(s.channels, authToken)
		close(channelContainer.channel)
	}
}

func (s *store) pushUpdate(authToken string, gameState *GameState) {
	if _, present := s.channels[authToken]; present {
		s.locker.Lock()

		if channel, present := s.channels[authToken]; present {
			channel.channel <- gameState
		}

		s.locker.Unlock()
	}
}
