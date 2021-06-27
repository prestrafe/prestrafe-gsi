package smstore

import (
	"reflect"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"gitlab.com/prestrafe/prestrafe-gsi/model"
)

const (
	channelBufferSize = 10
)

var (
	operationsCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "prestrafe",
		Subsystem: "sm",
		Name:      "operations",
		Help:      "Counts the number of operations on the SM backend per token",
	}, []string{"token", "operation"})
)

// Defines the public API for the SM store. The store is responsible for saving game states and evicting them once they
// go stale. Additional the store provides a channel object, that can be used to get notified, if a game state updates.
type Store interface {
	// Returns a channel that is filled with updates of the game state for the given auth token. Calling this method
	// also means that the caller needs to call ReleaseChannel(authKey), once he is done with using the channel.
	GetChannel(authKey string) chan *model.FullPlayerInfo

	// Releases a channel that was previously acquired by GetChannel(authKey).
	ReleaseChannel(authKey string)

	// Returns a game state for the given auth token, if one is present.
	Get(authKey string) (playerState *model.FullPlayerInfo, present bool)

	// Puts a newStore game state for the given auth token, if none is already present. Otherwise the existing game state
	// will be updated with the passed one.
	Put(serverInfo *model.ServerInfo, playerInfo *model.PlayerInfo)

	// Removes a game state for the given auth token, if one is present.
	Remove(authKey string)

	// Closes the store and releases all resources held by it.
	Close()
}

type store struct {
	channels      map[string]*channelContainer
	internalCache *cache.Cache
	locker        sync.Locker
}

type channelContainer struct {
	channel chan *model.FullPlayerInfo
	clients int
}

// Creates a newStore store, with a given TTL. The TTL is the duration for game states, before they are considered stale.
func New(ttl time.Duration) Store {
	return newStore(ttl)
}

func newStore(ttl time.Duration) *store {
	internalCache := cache.New(ttl, ttl*10)
	channels := make(map[string]*channelContainer)
	store := &store{channels, internalCache, &sync.Mutex{}}

	internalCache.OnEvicted(func(authKey string, item interface{}) {
		store.pushUpdate(authKey, nil)
	})

	return store
}

func (s *store) GetChannel(authKey string) chan *model.FullPlayerInfo {
	operationsCounter.WithLabelValues(authKey, "channel_get").Inc()

	s.locker.Lock()

	if _, present := s.channels[authKey]; !present {
		playerState, _ := s.Get(authKey)

		s.channels[authKey] = &channelContainer{make(chan *model.FullPlayerInfo, channelBufferSize), 0}
		s.channels[authKey].channel <- playerState
	}

	container := s.channels[authKey]
	container.clients++

	s.locker.Unlock()

	return container.channel
}

func (s *store) ReleaseChannel(authKey string) {
	operationsCounter.WithLabelValues(authKey, "channel_release").Inc()

	if _, present := s.channels[authKey]; present {
		s.locker.Lock()

		if container, present := s.channels[authKey]; present {
			container.clients--
			if container.clients < 1 {
				delete(s.channels, authKey)
				close(container.channel)
			}
		}

		s.locker.Unlock()
	}
}

func (s *store) Get(authKey string) (gameState *model.FullPlayerInfo, present bool) {
	operationsCounter.WithLabelValues(authKey, "get").Inc()

	if cached, isCached := s.internalCache.Get(authKey); isCached {
		gameState = cached.(*model.FullPlayerInfo)
		present = isCached
	}
	return
}

func (s *store) Put(serverInfo *model.ServerInfo, playerInfo *model.PlayerInfo) {
	operationsCounter.WithLabelValues(playerInfo.AuthKey, "put").Inc()

	previousFullPlayerInfo, _ := s.internalCache.Get(playerInfo.AuthKey)
	fullPlayerInfo := model.New(serverInfo, playerInfo)
	s.internalCache.Set(playerInfo.AuthKey, fullPlayerInfo, cache.DefaultExpiration)

	if !reflect.DeepEqual(previousFullPlayerInfo, fullPlayerInfo) {
		s.pushUpdate(playerInfo.AuthKey, fullPlayerInfo)
	}
}

// Unneeded, delete later
func (s *store) Remove(authKey string) {
	operationsCounter.WithLabelValues(authKey, "remove").Inc()

	s.internalCache.Delete(authKey)
}

func (s *store) Close() {
	for authKey, channelContainer := range s.channels {
		delete(s.channels, authKey)
		close(channelContainer.channel)
	}
}

func (s *store) pushUpdate(authKey string, gameState *model.FullPlayerInfo) {
	if _, present := s.channels[authKey]; present {
		s.locker.Lock()

		if channel, present := s.channels[authKey]; present {
			channel.channel <- gameState
		}

		s.locker.Unlock()
	}
}
