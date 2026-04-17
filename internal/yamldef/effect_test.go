package yamldef_test

import (
	"testing"

	"github.com/karthangar/matteresp32hub/internal/yamldef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var fireflyYAML = []byte(`
id: firefly-effect
name: "Firefly Blink"
version: "1.0"
compatible_with: [drv8833]
params:
  - id: speed
    type: speed
    label: "Blink speed"
    default: 1.0
    unit: hz
    min: 0.1
    max: 10.0
  - id: intensity
    type: percent
    label: "Max brightness"
    default: 0.8
  - id: randomize
    type: bool
    label: "Random timing"
    default: true
`)

func TestParseEffect_Valid(t *testing.T) {
	e, err := yamldef.ParseEffect(fireflyYAML)
	require.NoError(t, err)
	assert.Equal(t, "firefly-effect", e.ID)
	assert.Equal(t, []string{"drv8833"}, e.CompatibleWith)
	assert.Len(t, e.Params, 3)
	assert.Equal(t, "speed", e.Params[0].ID)
	assert.Equal(t, "speed", e.Params[0].Type)
}

func TestParseEffect_MissingID(t *testing.T) {
	_, err := yamldef.ParseEffect([]byte("name: test\ncompatible_with: [drv8833]\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "id")
}

func TestParseEffect_EmptyCompatible(t *testing.T) {
	_, err := yamldef.ParseEffect([]byte("id: x\nname: x\nversion: \"1.0\"\ncompatible_with: []\nparams: []\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compatible_with")
}

func TestParseEffect_InvalidParamType(t *testing.T) {
	yamlBytes := []byte(`
id: x
name: x
version: "1.0"
compatible_with: [drv8833]
params:
  - id: p1
    type: rainbow
    label: "bad"
`)
	_, err := yamldef.ParseEffect(yamlBytes)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "param type")
}
