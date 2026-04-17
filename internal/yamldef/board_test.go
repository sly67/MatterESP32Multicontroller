package yamldef_test

import (
	"os"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/yamldef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBoard_Valid(t *testing.T) {
	data, err := os.ReadFile("../../data/boards/esp32-c3.yaml")
	require.NoError(t, err)
	b, err := yamldef.ParseBoard(data)
	require.NoError(t, err)
	assert.Equal(t, "esp32-c3", b.ID)
	assert.Equal(t, "ESP32-C3", b.Chip)
	assert.Greater(t, len(b.GPIOPins), 10)
	assert.Equal(t, 6, b.ADCChannels)
}

func TestParseBoard_MissingID(t *testing.T) {
	_, err := yamldef.ParseBoard([]byte("name: test\nchip: ESP32-C3\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "id")
}
