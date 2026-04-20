package esphome_test

import (
	"testing"

	"github.com/karthangar/matteresp32hub/internal/esphome"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testModules() map[string]*yamldef.Module {
	return map[string]*yamldef.Module{
		"dht22": {
			ESPHome: &yamldef.ESPHomeDef{
				Components: []yamldef.ESPHomeComponent{
					{Domain: "sensor", Template: "platform: dht\n  model: DHT22\n  pin: \"{DATA}\"\n  temperature:\n    name: \"{NAME} Temperature\"\n  humidity:\n    name: \"{NAME} Humidity\"\n  update_interval: 30s"},
				},
			},
		},
		"relay": {
			ESPHome: &yamldef.ESPHomeDef{
				Components: []yamldef.ESPHomeComponent{
					{Domain: "switch", Template: "platform: gpio\n  pin: \"{OUT}\"\n  name: \"{NAME}\""},
				},
			},
		},
	}
}

func TestAssemble_ContainsHeader(t *testing.T) {
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "Kitchen Sensor", DeviceID: "aabbccddeeff",
		WiFiSSID: "MyNet", WiFiPassword: "s3cret",
		OTAPassword: "otapass", 	}
	out, err := esphome.Assemble(cfg, testModules())
	require.NoError(t, err)
	assert.Contains(t, out, "esp32:")
	assert.Contains(t, out, "board: esp32-c3-devkitm-1")
	assert.Contains(t, out, `ssid: "MyNet"`)
	assert.Contains(t, out, "password: \"otapass\"")
	assert.Contains(t, out, "api:")
}

func TestAssemble_HAIntegration(t *testing.T) {
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "Sensor", DeviceID: "aabb",
		WiFiSSID: "net", WiFiPassword: "pass",
		OTAPassword: "otp",		HAIntegration: true, APIKey: "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleTA=",
	}
	out, err := esphome.Assemble(cfg, testModules())
	require.NoError(t, err)
	assert.Contains(t, out, "api:")
	assert.Contains(t, out, "key: \"dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleTA=\"")
}

func TestAssemble_NoHAIntegration(t *testing.T) {
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "Sensor", DeviceID: "aabb",
		WiFiSSID: "net", WiFiPassword: "pass",
		OTAPassword: "otp",		HAIntegration: false,
	}
	out, err := esphome.Assemble(cfg, testModules())
	require.NoError(t, err)
	assert.NotContains(t, out, "key:") // no API encryption key without HA integration
}

func TestAssemble_ComponentPinSubstitution(t *testing.T) {
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "k", DeviceID: "id",
		WiFiSSID: "n", WiFiPassword: "p", OTAPassword: "o",		Components: []esphome.ComponentConfig{
			{Type: "dht22", Name: "Room Temp", Pins: map[string]string{"DATA": "GPIO4"}},
		},
	}
	out, err := esphome.Assemble(cfg, testModules())
	require.NoError(t, err)
	assert.Contains(t, out, "sensor:")
	assert.Contains(t, out, "GPIO4")
	assert.Contains(t, out, "Room Temp Temperature")
	assert.NotContains(t, out, "{DATA}")
	assert.NotContains(t, out, "{NAME}")
}

func TestAssemble_MultipleComponentsSameDomain(t *testing.T) {
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "multi", DeviceID: "id",
		WiFiSSID: "n", WiFiPassword: "p", OTAPassword: "o",		Components: []esphome.ComponentConfig{
			{Type: "dht22", Name: "Sensor 1", Pins: map[string]string{"DATA": "GPIO4"}},
			{Type: "dht22", Name: "Sensor 2", Pins: map[string]string{"DATA": "GPIO5"}},
		},
	}
	out, err := esphome.Assemble(cfg, testModules())
	require.NoError(t, err)
	assert.Contains(t, out, "GPIO4")
	assert.Contains(t, out, "GPIO5")
	assert.Contains(t, out, "Sensor 1 Temperature")
	assert.Contains(t, out, "Sensor 2 Temperature")
}

func TestAssemble_IDSubstitution(t *testing.T) {
	mods := map[string]*yamldef.Module{
		"led-strip": {
			ESPHome: &yamldef.ESPHomeDef{
				Components: []yamldef.ESPHomeComponent{
					{Domain: "output", Template: "platform: ledc\n  pin: \"{AIN1}\"\n  id: {ID}_ain1"},
					{Domain: "light", Template: "platform: monochromatic\n  name: \"{NAME}\"\n  output: {ID}_ain1"},
				},
			},
		},
	}
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "x", DeviceID: "y",
		WiFiSSID: "n", WiFiPassword: "p", OTAPassword: "o",		Components: []esphome.ComponentConfig{
			{Type: "led-strip", Name: "Room Strip", Pins: map[string]string{"AIN1": "GPIO4"}},
		},
	}
	out, err := esphome.Assemble(cfg, mods)
	require.NoError(t, err)
	assert.Contains(t, out, "room_strip_ain1", "ID placeholder must be replaced with slugified component name")
	assert.NotContains(t, out, "{ID}", "raw {ID} placeholder must not appear in output")
}

func TestAssemble_MonoConfigPinSubstitution(t *testing.T) {
	mods := map[string]*yamldef.Module{
		"drv8833-mono": {
			ESPHome: &yamldef.ESPHomeDef{
				Components: []yamldef.ESPHomeComponent{
					{Domain: "output", Template: "platform: custom\n  lambda: |-\n    auto c = new Mono(parseG(\"{AIN1}\"), {LEDC_TIMER}, {LEDC_CHAN_A}, {LEDC_CHAN_B});\n    return {c};\n  outputs:\n    - id: {ID}_mono_out"},
				},
			},
		},
	}
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "Strip", DeviceID: "dev1",
		WiFiSSID: "n", WiFiPassword: "p", OTAPassword: "o",		Components: []esphome.ComponentConfig{
			{Type: "drv8833-mono", Name: "Mono LED", Pins: map[string]string{
				"AIN1":       "GPIO0",
				"LEDC_TIMER": "1",
				"LEDC_CHAN_A": "2",
				"LEDC_CHAN_B": "3",
			}},
		},
	}
	out, err := esphome.Assemble(cfg, mods)
	require.NoError(t, err)
	assert.Contains(t, out, "GPIO0", "GPIO pin must be substituted")
	assert.Contains(t, out, ", 1, 2, 3)", "config pin integers must be substituted unquoted")
	assert.NotContains(t, out, "{AIN1}", "raw {AIN1} must not appear")
	assert.NotContains(t, out, "{LEDC_TIMER}", "raw {LEDC_TIMER} must not appear")
	assert.NotContains(t, out, "{LEDC_CHAN_A}", "raw {LEDC_CHAN_A} must not appear")
	assert.NotContains(t, out, "{LEDC_CHAN_B}", "raw {LEDC_CHAN_B} must not appear")
}

func TestAssemble_UnknownModuleError(t *testing.T) {
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "x", DeviceID: "y",
		WiFiSSID: "n", WiFiPassword: "p", OTAPassword: "o",		Components: []esphome.ComponentConfig{
			{Type: "nonexistent", Name: "X", Pins: map[string]string{}},
		},
	}
	_, err := esphome.Assemble(cfg, testModules())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}
