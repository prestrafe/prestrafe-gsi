package gsistore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"gitlab.com/prestrafe/prestrafe-gsi/model"
)

func TestStoring(t *testing.T) {
	store := newStore(15 * time.Millisecond)
	store.Put("token", &model.GameState{})

	gameState, present := store.Get("token")
	assert.True(t, present)
	assert.NotNil(t, gameState)

	time.Sleep(20 * time.Millisecond)

	gameState, present = store.Get("token")
	assert.False(t, present)
	assert.Nil(t, gameState)
}

func TestChannelStoreRemove(t *testing.T) {
	store := newStore(15 * time.Minute)
	store.Put("token", &model.GameState{})

	channel := store.GetChannel("token")
	assert.NotNil(t, channel)

	assertChannel(t, channel, true, true)
	store.Remove("token")
	assertChannel(t, channel, false, true)
	store.ReleaseChannel("token")
	assertChannel(t, channel, false, false)
}

func TestChannelStoreTimeout(t *testing.T) {
	store := newStore(15 * time.Millisecond)
	store.Put("token", &model.GameState{})

	channel := store.GetChannel("token")
	assert.NotNil(t, channel)

	assertChannel(t, channel, true, true)
	time.Sleep(20 * time.Millisecond)
	assertChannel(t, channel, false, true)
	store.ReleaseChannel("token")
	assertChannel(t, channel, false, false)
}

func TestChannelStoreClose(t *testing.T) {
	store := newStore(15 * time.Minute)
	store.Put("token", &model.GameState{})

	channel := store.GetChannel("token")
	assert.NotNil(t, channel)

	assertChannel(t, channel, true, true)
	store.Close()
	assertChannel(t, channel, false, false)
}

func assertChannel(t *testing.T, channel chan *model.GameState, hasElement, hasMore bool) {
	element, more := <-channel
	if hasElement {
		assert.NotNil(t, element)
	} else {
		assert.Nil(t, element)
	}

	if hasMore {
		assert.True(t, more)
	} else {
		assert.False(t, more)
	}
}
