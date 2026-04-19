package esphome_test

import (
	"context"
	"os"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/esphome"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder_New(t *testing.T) {
	b, err := esphome.NewBuilder("/tmp/esphome-test-cache")
	require.NoError(t, err)
	assert.NotNil(t, b)
	b.Close()
}

func TestBuilder_Compile_Integration(t *testing.T) {
	if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
		t.Skip("Docker socket not available — skipping integration test")
	}
	b, err := esphome.NewBuilder(t.TempDir())
	require.NoError(t, err)
	defer b.Close()

	yaml := `
esphome:
  name: test-device

esp32:
  board: esp32-c3-devkitm-1
  framework:
    type: esp-idf

wifi:
  ssid: "TestNet"
  password: "testpass"
  ap:
    ssid: "test-fallback"
    password: "changeme"

logger:

ota:
  - platform: esphome
    password: "otapass"
`
	bin, err := b.Compile(context.Background(), "test-device", yaml, os.Stdout)
	require.NoError(t, err)
	assert.Greater(t, len(bin), 1000, "firmware binary should be > 1kB")
}
