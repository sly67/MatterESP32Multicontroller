package nvs_test

import (
	"strings"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/nvs"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestTemplate() *yamldef.Template {
	return &yamldef.Template{
		ID:    "test-tpl",
		Board: "esp32-c3",
		Modules: []yamldef.TemplateModule{
			{
				Module:       "drv8833",
				Pins:         map[string]string{"AIN1": "GPIO4", "AIN2": "GPIO5", "BIN1": "GPIO6", "BIN2": "GPIO7"},
				EndpointName: "Firefly Lights",
				Effect:       "firefly-effect",
			},
		},
	}
}

func makeTestDevice() nvs.DeviceConfig {
	return nvs.DeviceConfig{
		Name:           "1/Bedroom",
		WiFiSSID:       "TestNet",
		WiFiPassword:   "secret",
		PSK:            make([]byte, 32),
		BoardID:        "esp32-c3",
		MatterDiscrim:  3840,
		MatterPasscode: 20202021,
	}
}

func TestCompile_ProducesCSV(t *testing.T) {
	tpl := makeTestTemplate()
	dev := makeTestDevice()
	csv, err := nvs.Compile(tpl, dev)
	require.NoError(t, err)
	assert.Contains(t, csv, "key,type,encoding,value")
}

func TestCompile_ContainsWiFi(t *testing.T) {
	csv, err := nvs.Compile(makeTestTemplate(), makeTestDevice())
	require.NoError(t, err)
	assert.Contains(t, csv, "TestNet")
	assert.Contains(t, csv, "secret")
}

func TestCompile_ContainsDeviceName(t *testing.T) {
	csv, err := nvs.Compile(makeTestTemplate(), makeTestDevice())
	require.NoError(t, err)
	assert.Contains(t, csv, "1/Bedroom")
}

func TestCompile_ContainsBoardID(t *testing.T) {
	csv, err := nvs.Compile(makeTestTemplate(), makeTestDevice())
	require.NoError(t, err)
	assert.Contains(t, csv, "esp32-c3")
}

func TestCompile_ContainsModuleType(t *testing.T) {
	csv, err := nvs.Compile(makeTestTemplate(), makeTestDevice())
	require.NoError(t, err)
	assert.Contains(t, csv, "drv8833")
}

func TestCompile_ContainsPinAssignment(t *testing.T) {
	csv, err := nvs.Compile(makeTestTemplate(), makeTestDevice())
	require.NoError(t, err)
	assert.Contains(t, csv, "GPIO4")
}

func TestCompile_HeaderOnFirstLine(t *testing.T) {
	csv, err := nvs.Compile(makeTestTemplate(), makeTestDevice())
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(csv), "\n")
	assert.Equal(t, "key,type,encoding,value", lines[0])
}

func TestCompile_EmptyPSK(t *testing.T) {
	dev := makeTestDevice()
	dev.PSK = []byte{}
	_, err := nvs.Compile(makeTestTemplate(), dev)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PSK")
}
