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
	cacheDir     string
	volumeRef    string // Docker volume source for /config bind mount
	pioVolumeRef string // Docker volume source for /root/.platformio (Python venv cache)
}

// NewBuilder creates a Builder that will use cacheDir for ESPHome build artifacts.
// volumeRef is the Docker volume source for the /config bind mount into the ESPHome container.
// pioDir/pioVolumeRef follow the same pattern for the PlatformIO home (/root/.platformio),
// which caches the Python venv and avoids re-downloading it on every compile.
// Pass empty strings for pioDir/pioVolumeRef to skip PIO home caching.
func NewBuilder(cacheDir, volumeRef, pioDir, pioVolumeRef string) (*Builder, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}
	if volumeRef == "" {
		volumeRef = cacheDir
	}
	if pioDir != "" {
		if err := os.MkdirAll(pioDir, 0755); err != nil {
			return nil, fmt.Errorf("create pio dir: %w", err)
		}
		if pioVolumeRef == "" {
			pioVolumeRef = pioDir
		}
	}
	return &Builder{cacheDir: cacheDir, volumeRef: volumeRef, pioVolumeRef: pioVolumeRef}, nil
}

// Close is a no-op (kept for API symmetry).
func (b *Builder) Close() {}

// Compile writes the ESPHome YAML to disk, compiles it in a Docker container, and
// returns the firmware.factory.bin bytes. Build logs are streamed to logWriter.
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

	// Write SDK header wrappers so modules can use esphome: includes: with SDK paths.
	// Including SDK headers inside a lambda body is invalid C++ (extern "C" at function scope);
	// ESPHome's includes: emits #include at file scope, which is correct.
	for relPath, content := range map[string]string{
		"driver/ledc.h": "#pragma once\n#include <driver/ledc.h>\n",
	} {
		wrapPath := filepath.Join(devDir, relPath)
		if err := os.MkdirAll(filepath.Dir(wrapPath), 0755); err != nil {
			return nil, fmt.Errorf("create wrapper dir: %w", err)
		}
		if err := os.WriteFile(wrapPath, []byte(content), 0644); err != nil {
			return nil, fmt.Errorf("write header wrapper %s: %w", relPath, err)
		}
	}

	deviceSlug := slug(deviceName)

	args := []string{
		"run", "--rm",
		"--network", "host",
		"-v", b.volumeRef + ":/config",
	}
	if b.pioVolumeRef != "" {
		args = append(args, "-v", b.pioVolumeRef+":/root/.platformio")
	}
	args = append(args,
		"ghcr.io/esphome/esphome:latest",
		"compile",
		"/config/"+deviceSlug+"/config.yaml",
	)
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("esphome compile: %w", err)
	}

	binPath := filepath.Join(devDir, ".esphome", "build", deviceSlug, ".pioenvs", deviceSlug, "firmware.factory.bin")
	bin, err := os.ReadFile(binPath)
	if err != nil {
		return nil, fmt.Errorf("read firmware binary (%s): %w", binPath, err)
	}
	return bin, nil
}
