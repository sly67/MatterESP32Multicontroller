package yamldef_test

import (
	"testing"

	"github.com/karthangar/matteresp32hub/internal/yamldef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var drv8833YAML = []byte(`
id: drv8833
name: "DRV8833 Dual H-Bridge Motor Driver"
version: "1.0"
category: driver
io:
  - id: AIN1
    type: digital_pwm_out
    label: "Bridge A Input 1"
    constraints:
      pwm:
        frequency_hz: 20000
        duty_min: 0.0
        duty_max: 1.0
        resolution_bits: 10
  - id: AIN2
    type: digital_pwm_out
    label: "Bridge A Input 2"
    constraints:
      pwm:
        frequency_hz: 20000
        duty_min: 0.0
        duty_max: 1.0
        resolution_bits: 10
truth_table:
  coast:   {in1: 0, in2: 0}
  forward: {in1: 1, in2: 0}
capabilities:
  - id: channel_mode
    type: select
    label: "Active channels"
    options:
      - {value: A_only, label: "Channel A only"}
matter:
  endpoint_type: extended_color_light
  behaviors: [firefly_effect]
`)

func TestParseModule_Valid(t *testing.T) {
	m, err := yamldef.ParseModule(drv8833YAML)
	require.NoError(t, err)
	assert.Equal(t, "drv8833", m.ID)
	assert.Equal(t, "driver", m.Category)
	assert.Len(t, m.IO, 2)
	assert.Equal(t, "AIN1", m.IO[0].ID)
	assert.Equal(t, "digital_pwm_out", m.IO[0].Type)
	assert.NotNil(t, m.IO[0].Constraints.PWM)
	assert.Equal(t, 20000, m.IO[0].Constraints.PWM.FrequencyHz)
	assert.Equal(t, "extended_color_light", m.Matter.EndpointType)
}

func TestParseModule_MissingID(t *testing.T) {
	_, err := yamldef.ParseModule([]byte("name: test\ncategory: driver\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "id")
}

func TestParseModule_InvalidCategory(t *testing.T) {
	_, err := yamldef.ParseModule([]byte("id: x\nname: x\nversion: \"1.0\"\ncategory: robot\nio: []\nmatter:\n  endpoint_type: on_off_light\n  behaviors: []\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "category")
}

func TestParseModule_InvalidIOType(t *testing.T) {
	yamlBytes := []byte(`
id: x
name: x
version: "1.0"
category: driver
io:
  - id: P1
    type: laser_out
    label: "bad"
matter:
  endpoint_type: on_off_light
  behaviors: []
`)
	_, err := yamldef.ParseModule(yamlBytes)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "io type")
}

func TestParseModule_ESPHomeBlock(t *testing.T) {
	yaml := []byte(`
id: test-gpio
name: "Test GPIO"
version: "1.0"
category: io
io:
  - id: OUT
    type: digital_out
    label: "Output"
    constraints:
      digital: {active: high, initial_state: low}
matter:
  endpoint_type: on_off_light
  behaviors: [on_off]
esphome:
  components:
    - domain: switch
      template: "platform: gpio\npin: \"{OUT}\"\nname: \"{NAME}\""
`)
	mod, err := yamldef.ParseModule(yaml)
	require.NoError(t, err)
	require.NotNil(t, mod.ESPHome)
	require.Len(t, mod.ESPHome.Components, 1)
	assert.Equal(t, "switch", mod.ESPHome.Components[0].Domain)
	assert.Contains(t, mod.ESPHome.Components[0].Template, "{OUT}")
}

func TestParseModule_NoESPHomeIsValid(t *testing.T) {
	yaml := []byte(`
id: gpio-switch
name: "GPIO Switch"
version: "1.0"
category: io
io:
  - id: OUT
    type: digital_out
    label: "Output"
    constraints:
      digital: {active: high, initial_state: low}
matter:
  endpoint_type: on_off_light
  behaviors: [on_off]
`)
	mod, err := yamldef.ParseModule(yaml)
	require.NoError(t, err)
	assert.Nil(t, mod.ESPHome)
}
