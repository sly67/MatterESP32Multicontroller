package nvs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/nvs"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
	"github.com/stretchr/testify/require"
)

func TestGenerateBinary_ProducesBin(t *testing.T) {
	if _, err := exec.LookPath("nvs_partition_gen.py"); err != nil {
		t.Skip("nvs_partition_gen.py not in PATH — skipping (runs in Docker)")
	}

	tpl := &yamldef.Template{
		ID:    "test",
		Board: "esp32-c3",
		Modules: []yamldef.TemplateModule{{
			Module:       "gpio-switch",
			Pins:         map[string]string{"OUT": "GPIO4"},
			EndpointName: "Light",
		}},
	}
	dev := nvs.DeviceConfig{
		Name: "1/Test", WiFiSSID: "Net", WiFiPassword: "pass",
		PSK: make([]byte, 32), BoardID: "esp32-c3",
		MatterDiscrim: 1234, MatterPasscode: 20202021,
	}
	csv, err := nvs.Compile(tpl, dev)
	require.NoError(t, err)

	binPath, err := nvs.GenerateBinary(csv)
	require.NoError(t, err)
	defer os.RemoveAll(filepath.Dir(binPath))

	info, err := os.Stat(binPath)
	require.NoError(t, err)
	require.Greater(t, info.Size(), int64(0))
}
