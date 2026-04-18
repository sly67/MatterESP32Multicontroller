package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebFlash_Manifest_NoFirmware(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/webflash/manifest.json", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestWebFlash_Manifest_WithFirmware(t *testing.T) {
	srv := newTestServer(t)
	database := getDatabase(t, srv)
	require.NoError(t, database.CreateFirmware(db.FirmwareRow{Version: "2.0.0", Boards: "esp32c3", IsLatest: true}))

	req := httptest.NewRequest(http.MethodGet, "/api/webflash/manifest.json", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var manifest map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&manifest))
	assert.Equal(t, "2.0.0", manifest["version"])
	builds := manifest["builds"].([]interface{})
	require.Len(t, builds, 1)
	build := builds[0].(map[string]interface{})
	assert.Equal(t, "ESP32-C3", build["chipFamily"])
	parts := build["parts"].([]interface{})
	require.Len(t, parts, 4)
	expectedOffsets := []float64{0, 32768, 61440, 131072}
	for i, p := range parts {
		part := p.(map[string]interface{})
		assert.Equal(t, expectedOffsets[i], part["offset"].(float64))
	}
}

func TestWebFlash_Bootloader_Served(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/webflash/bootloader.bin", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/octet-stream", w.Header().Get("Content-Type"))
	assert.Greater(t, w.Body.Len(), 0)
}

func TestWebFlash_Firmware_NoLatest(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/webflash/firmware.bin", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestWebFlash_Firmware_ServesLatest(t *testing.T) {
	dir := t.TempDir()
	// Write fake firmware file
	content := []byte("fake-firmware-binary")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "1.5.0.bin"), content, 0644))

	// Use a server with DataDir pointing to dir (need to store firmware under dir/firmware/)
	fwDir := filepath.Join(dir, "firmware")
	require.NoError(t, os.MkdirAll(fwDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(fwDir, "1.5.0.bin"), content, 0644))

	srv := newTestServerWithDataDir(t, dir)
	database := getDatabase(t, srv)
	require.NoError(t, database.CreateFirmware(db.FirmwareRow{Version: "1.5.0", Boards: "esp32c3", IsLatest: true}))

	req := httptest.NewRequest(http.MethodGet, "/api/webflash/firmware.bin", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/octet-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, string(content), w.Body.String())
}
