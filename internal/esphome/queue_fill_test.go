package esphome

import (
	"testing"

	"github.com/karthangar/matteresp32hub/internal/library"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFillIODefaults_FillsMissingDefaults(t *testing.T) {
	mods, err := library.LoadModules()
	require.NoError(t, err)
	modMap := make(map[string]*yamldef.Module, len(mods))
	for _, m := range mods {
		modMap[m.ID] = m
	}

	components := []ComponentConfig{
		{
			Type: "drv8833-led-mono",
			Name: "Test",
			Pins: map[string]string{"AIN1": "GPIO0", "AIN2": "GPIO1"},
		},
	}

	fillIODefaults(components, modMap)

	pins := components[0].Pins
	assert.Equal(t, "2.2", pins["GAMMA"], "GAMMA should be filled with default")
	assert.Equal(t, "0.0", pins["CUTOFF_PCT"], "CUTOFF_PCT should be filled with default")
	assert.Equal(t, "0", pins["CUTOFF_AFTER_GAMMA"], "CUTOFF_AFTER_GAMMA should be filled with default")
	assert.Equal(t, "2", pins["LEDC_CHAN_A"], "LEDC_CHAN_A should be filled with default")
	assert.Equal(t, "GPIO0", pins["AIN1"], "AIN1 should be preserved as-is (not overwritten by fillIODefaults)")
}

func TestFillIODefaults_DoesNotOverwriteProvided(t *testing.T) {
	mods, err := library.LoadModules()
	require.NoError(t, err)
	modMap := make(map[string]*yamldef.Module, len(mods))
	for _, m := range mods {
		modMap[m.ID] = m
	}

	components := []ComponentConfig{
		{
			Type: "drv8833-led-mono",
			Name: "Test",
			Pins: map[string]string{
				"AIN1":  "GPIO0",
				"AIN2":  "GPIO1",
				"GAMMA": "1.8",
			},
		},
	}

	fillIODefaults(components, modMap)

	assert.Equal(t, "1.8", components[0].Pins["GAMMA"], "caller-provided GAMMA must not be overwritten")
}

func TestFillIODefaults_UnknownModuleSkipped(t *testing.T) {
	components := []ComponentConfig{
		{Type: "unknown-module", Name: "X", Pins: nil},
	}
	fillIODefaults(components, map[string]*yamldef.Module{})
	assert.Nil(t, components[0].Pins, "unknown module should not modify pins")
}
