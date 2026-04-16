package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_WritesDefaultsOnFirstBoot(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, 48060, cfg.WebPort)
	assert.Equal(t, 48061, cfg.OTAPort)
	assert.FileExists(t, filepath.Join(dir, "app.yaml"))
}

func TestLoad_ReadsExistingConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.yaml"),
		[]byte("web_port: 9000\nota_port: 9001\n"), 0644))
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, 9000, cfg.WebPort)
	assert.Equal(t, 9001, cfg.OTAPort)
}
