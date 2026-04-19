package esphome

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const compileTimeout = 15 * time.Minute

// Builder compiles ESPHome YAML into firmware binaries using a one-shot Docker container.
type Builder struct {
	cacheDir string
}

// NewBuilder creates a Builder that will use cacheDir for ESPHome build artifacts.
func NewBuilder(cacheDir string) (*Builder, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}
	return &Builder{cacheDir: cacheDir}, nil
}

// Close is a no-op (kept for API symmetry).
func (b *Builder) Close() {}

// Compile writes the ESPHome YAML to disk, compiles it in a Docker container, and
// returns the firmware-factory.bin bytes. Build logs are streamed to logWriter.
// deviceName is used as both the config directory name and the YAML device slug.
func (b *Builder) Compile(ctx context.Context, deviceName string, yaml string, logWriter io.Writer) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, compileTimeout)
	defer cancel()

	devDir := filepath.Join(b.cacheDir, deviceName)
	if err := os.MkdirAll(devDir, 0755); err != nil {
		return nil, fmt.Errorf("create device dir: %w", err)
	}

	cfgPath := filepath.Join(devDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yaml), 0644); err != nil {
		return nil, fmt.Errorf("write config: %w", err)
	}

	cmd := exec.CommandContext(ctx,
		"docker", "run", "--rm",
		"-v", b.cacheDir+":/config",
		"ghcr.io/esphome/esphome:latest",
		"compile",
		"/config/"+deviceName+"/config.yaml",
	)
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("esphome compile: %w", err)
	}

	binPath := filepath.Join(devDir, ".esphome", "build", deviceName, ".pioenvs", deviceName, "firmware-factory.bin")
	bin, err := os.ReadFile(binPath)
	if err != nil {
		return nil, fmt.Errorf("read firmware binary (%s): %w", binPath, err)
	}
	return bin, nil
}
