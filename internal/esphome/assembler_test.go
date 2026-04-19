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
		OTAPassword: "otapass", HubURL: "http://10.0.0.1:8080",
	}
	out, err := esphome.Assemble(cfg, testModules())
	require.NoError(t, err)
	assert.Contains(t, out, "esp32:")
	assert.Contains(t, out, "board: esp32-c3-devkitm-1")
	assert.Contains(t, out, `ssid: "MyNet"`)
	assert.Contains(t, out, "password: \"otapass\"")
	assert.Contains(t, out, "http://10.0.0.1:8080/api/devices/aabbccddeeff/heartbeat")
}

func TestAssemble_HAIntegration(t *testing.T) {
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "Sensor", DeviceID: "aabb",
		WiFiSSID: "net", WiFiPassword: "pass",
		OTAPassword: "otp", HubURL: "http://hub",
		HAIntegration: true, APIKey: "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleTA=",
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
		OTAPassword: "otp", HubURL: "http://hub",
		HAIntegration: false,
	}
	out, err := esphome.Assemble(cfg, testModules())
	require.NoError(t, err)
	assert.NotContains(t, out, "api:")
}

func TestAssemble_ComponentPinSubstitution(t *testing.T) {
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "k", DeviceID: "id",
		WiFiSSID: "n", WiFiPassword: "p", OTAPassword: "o", HubURL: "http://h",
		Components: []esphome.ComponentConfig{
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
		WiFiSSID: "n", WiFiPassword: "p", OTAPassword: "o", HubURL: "http://h",
		Components: []esphome.ComponentConfig{
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

func TestAssemble_UnknownModuleError(t *testing.T) {
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "x", DeviceID: "y",
		WiFiSSID: "n", WiFiPassword: "p", OTAPassword: "o", HubURL: "http://h",
		Components: []esphome.ComponentConfig{
			{Type: "nonexistent", Name: "X", Pins: map[string]string{}},
		},
	}
	_, err := esphome.Assemble(cfg, testModules())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}
