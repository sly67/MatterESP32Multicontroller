package nvs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// NVSPartitionSize is the standard ESP-IDF NVS partition size (24 KB).
const NVSPartitionSize = "0x6000"

// GenerateBinary writes csvContent to a temp CSV file, calls
// nvs_partition_gen.py to produce a .bin, and returns the path to that .bin.
// The caller is responsible for removing the temp directory when done.
// Use filepath.Dir(returned path) to get the temp dir for cleanup.
func GenerateBinary(csvContent string) (string, error) {
	dir, err := os.MkdirTemp("", "nvs-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	csvPath := filepath.Join(dir, "nvs.csv")
	if err := os.WriteFile(csvPath, []byte(csvContent), 0600); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("write csv: %w", err)
	}

	binPath := filepath.Join(dir, "nvs.bin")
	out, err := exec.Command(
		"python3", "-m", "esp_idf_nvs_partition_gen.nvs_partition_gen",
		"generate", csvPath, binPath, NVSPartitionSize,
	).CombinedOutput()
	if err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("nvs_partition_gen: %w\n%s", err, out)
	}

	return binPath, nil
}
