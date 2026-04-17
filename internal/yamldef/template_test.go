package yamldef_test

import (
	"testing"

	"github.com/karthangar/matteresp32hub/internal/yamldef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var fireflyTemplateYAML = []byte(`
id: firefly-hub-v1
board: esp32-c3
modules:
  - module: drv8833
    pins:
      AIN1: GPIO4
      AIN2: GPIO5
      BIN1: GPIO6
      BIN2: GPIO7
    endpoint_name: "Firefly Lights"
    effect: firefly-effect
    effect_params:
      speed: 1.2
      channel_mode: alternating
  - module: bh1750
    pins:
      SDA: GPIO18
      SCL: GPIO19
    endpoint_name: "Light Sensor"
    params:
      poll_interval_s: 5
`)

func TestParseTemplate_Valid(t *testing.T) {
	tpl, err := yamldef.ParseTemplate(fireflyTemplateYAML)
	require.NoError(t, err)
	assert.Equal(t, "firefly-hub-v1", tpl.ID)
	assert.Equal(t, "esp32-c3", tpl.Board)
	assert.Len(t, tpl.Modules, 2)
	assert.Equal(t, "drv8833", tpl.Modules[0].Module)
	assert.Equal(t, "GPIO4", tpl.Modules[0].Pins["AIN1"])
	assert.Equal(t, "firefly-effect", tpl.Modules[0].Effect)
}

func TestParseTemplate_MissingID(t *testing.T) {
	_, err := yamldef.ParseTemplate([]byte("board: esp32-c3\nmodules: []\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "id")
}

func TestParseTemplate_MissingBoard(t *testing.T) {
	_, err := yamldef.ParseTemplate([]byte("id: x\nmodules: []\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "board")
}

func TestParseTemplate_EmptyModules(t *testing.T) {
	_, err := yamldef.ParseTemplate([]byte("id: x\nboard: esp32-c3\nmodules: []\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one module")
}

func TestParseTemplate_ModuleMissingPins(t *testing.T) {
	yamlBytes := []byte(`
id: x
board: esp32-c3
modules:
  - module: drv8833
    pins: {}
    endpoint_name: "Test"
`)
	_, err := yamldef.ParseTemplate(yamlBytes)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pins")
}
