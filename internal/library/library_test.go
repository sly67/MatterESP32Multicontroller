package library_test

import (
	"testing"

	"github.com/karthangar/matteresp32hub/internal/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadModules_ReturnsAll(t *testing.T) {
	mods, err := library.LoadModules()
	require.NoError(t, err)
	assert.Len(t, mods, 9)
	ids := make(map[string]bool)
	for _, m := range mods {
		ids[m.ID] = true
	}
	assert.True(t, ids["drv8833"])
	assert.True(t, ids["wrgb-led"])
	assert.True(t, ids["bh1750"])
	assert.True(t, ids["analog-in"])
	assert.True(t, ids["gpio-switch"])
	assert.True(t, ids["dht22"])
	assert.True(t, ids["bme280"])
	assert.True(t, ids["neopixel"])
	assert.True(t, ids["binary-input"])
}

func TestLoadEffects_ReturnsTwo(t *testing.T) {
	effs, err := library.LoadEffects()
	require.NoError(t, err)
	assert.Len(t, effs, 2)
	ids := make(map[string]bool)
	for _, e := range effs {
		ids[e.ID] = true
	}
	assert.True(t, ids["firefly-effect"])
	assert.True(t, ids["breathing-effect"])
}

func TestLoadBoards_ReturnsThree(t *testing.T) {
	boards, err := library.LoadBoards()
	require.NoError(t, err)
	assert.Len(t, boards, 3)
	ids := make(map[string]bool)
	for _, b := range boards {
		ids[b.ID] = true
	}
	assert.True(t, ids["esp32-c3"])
	assert.True(t, ids["esp32-h2"])
	assert.True(t, ids["esp32-s3"])
}
