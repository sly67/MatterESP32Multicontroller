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
	cacheDir  string
	volumeRef string // Docker volume name or host path for the -v bind; equals cacheDir when empty
}

// NewBuilder creates a Builder that will use cacheDir for ESPHome build artifacts.
// volumeRef is the Docker volume source for the bind mount into the ESPHome container.
// Pass a named volume (e.g. "esphome-cache") when the server itself runs inside Docker so
// that both containers share the same volume rather than a container-internal path.
// If volumeRef is empty, cacheDir is used as the bind mount source (suitable for local dev).
func NewBuilder(cacheDir, volumeRef string) (*Builder, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}
	if volumeRef == "" {
		volumeRef = cacheDir
	}
	return &Builder{cacheDir: cacheDir, volumeRef: volumeRef}, nil
}

// Close is a no-op (kept for API symmetry).
func (b *Builder) Close() {}

// Compile writes the ESPHome YAML to disk, compiles it in a Docker container, and
// returns the firmware-factory.bin bytes. Build logs are streamed to logWriter.
// deviceName is used as both the config directory name and the YAML device slug.
func (b *Builder) Compile(ctx context.Context, deviceName string, yaml string, logWriter io.Writer) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, compileTimeout)
	defer cancel()

	devDir := filepath.Join(b.cacheDir, slug(deviceName))
	if err := os.MkdirAll(devDir, 0755); err != nil {
		return nil, fmt.Errorf("create device dir: %w", err)
	}

	cfgPath := filepath.Join(devDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yaml), 0644); err != nil {
		return nil, fmt.Errorf("write config: %w", err)
	}

	deviceSlug := slug(deviceName)

	cmd := exec.CommandContext(ctx,
		"docker", "run", "--rm",
		"-v", b.volumeRef+":/config",
		"ghcr.io/esphome/esphome:latest",
		"compile",
		"/config/"+deviceSlug+"/config.yaml",
	)
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("esphome compile: %w", err)
	}

	binPath := filepath.Join(devDir, ".esphome", "build", deviceSlug, ".pioenvs", deviceSlug, "firmware-factory.bin")
	bin, err := os.ReadFile(binPath)
	if err != nil {
		return nil, fmt.Errorf("read firmware binary (%s): %w", binPath, err)
	}
	return bin, nil
}
