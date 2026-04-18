package ota_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/ota"
)

func newTestDB(t *testing.T) *db.Database {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Insert a seed template so devices can reference it.
	_, err = database.DB.Exec(
		`INSERT INTO templates (id, name, board, yaml_body) VALUES ('tpl-1', 'test-tpl', 'esp32-c3', '')`)
	if err != nil {
		t.Fatalf("seed template: %v", err)
	}
	t.Cleanup(func() { database.DB.Close() })
	return database
}

func signedRequest(t *testing.T, method, path string, psk []byte, deviceID string) *http.Request {
	t.Helper()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	msg := fmt.Sprintf("%s:%s:%s", method, path, ts)
	h := hmac.New(sha256.New, psk)
	h.Write([]byte(msg))
	sig := base64.StdEncoding.EncodeToString(h.Sum(nil))

	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("X-Device-ID", deviceID)
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-HMAC", sig)
	req.Header.Set("X-FW-Version", "1.0.0")
	return req
}

func TestOTA_MissingAuthHeaders(t *testing.T) {
	database := newTestDB(t)
	mux := ota.NewMux(database, t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/ota/check", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestOTA_Check_ValidAuth(t *testing.T) {
	database := newTestDB(t)
	psk := []byte("testpsk0123456789012345678901234")

	if err := database.CreateDevice(db.Device{
		ID: "esp-AABBCC", Name: "test", TemplateID: "tpl-1",
		FWVersion: "1.0.0", PSK: psk,
	}); err != nil {
		t.Fatalf("create device: %v", err)
	}
	if err := database.CreateFirmware(db.FirmwareRow{Version: "1.1.0", Boards: "esp32-c3", IsLatest: true}); err != nil {
		t.Fatalf("create firmware: %v", err)
	}
	if err := database.SetLatestFirmware("1.1.0"); err != nil {
		t.Fatalf("set latest: %v", err)
	}

	mux := ota.NewMux(database, t.TempDir())
	req := signedRequest(t, http.MethodGet, "/ota/check", psk, "esp-AABBCC")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOTA_Check_WrongHMAC(t *testing.T) {
	database := newTestDB(t)
	psk := []byte("testpsk0123456789012345678901234")
	wrongPSK := []byte("wrongpsk012345678901234567890123")

	err := database.CreateDevice(db.Device{
		ID: "esp-AABBCC", Name: "test", TemplateID: "tpl-1",
		FWVersion: "1.0.0", PSK: psk,
	})
	if err != nil {
		t.Fatalf("create device: %v", err)
	}

	mux := ota.NewMux(database, t.TempDir())
	req := signedRequest(t, http.MethodGet, "/ota/check", wrongPSK, "esp-AABBCC")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestOTA_ExpiredTimestamp(t *testing.T) {
	database := newTestDB(t)
	psk := []byte("testpsk0123456789012345678901234")
	err := database.CreateDevice(db.Device{
		ID: "esp-AABBCC", Name: "test", TemplateID: "tpl-1", FWVersion: "1.0.0", PSK: psk,
	})
	if err != nil {
		t.Fatalf("create device: %v", err)
	}

	// Build request with timestamp 10 minutes in the past
	staleTS := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	msg := fmt.Sprintf("GET:/ota/check:%s", staleTS)
	h := hmac.New(sha256.New, psk)
	h.Write([]byte(msg))
	sig := base64.StdEncoding.EncodeToString(h.Sum(nil))

	req := httptest.NewRequest(http.MethodGet, "/ota/check", nil)
	req.Header.Set("X-Device-ID", "esp-AABBCC")
	req.Header.Set("X-Timestamp", staleTS)
	req.Header.Set("X-HMAC", sig)

	mux := ota.NewMux(database, t.TempDir())
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401 for stale timestamp, got %d", w.Code)
	}
}

func TestOTA_UnknownDevice(t *testing.T) {
	database := newTestDB(t)
	psk := []byte("testpsk0123456789012345678901234")

	mux := ota.NewMux(database, t.TempDir())
	req := signedRequest(t, http.MethodGet, "/ota/check", psk, "esp-UNKNOWN")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401 for unknown device, got %d", w.Code)
	}
}

func TestOTA_Download_FileNotFound(t *testing.T) {
	database := newTestDB(t)
	psk := []byte("testpsk0123456789012345678901234")

	err := database.CreateDevice(db.Device{
		ID: "esp-AABBCC", Name: "test", TemplateID: "tpl-1", FWVersion: "1.0.0", PSK: psk,
	})
	if err != nil {
		t.Fatalf("create device: %v", err)
	}
	// Seed firmware record but no actual file
	if err := database.CreateFirmware(db.FirmwareRow{Version: "1.1.0", Boards: "esp32-c3", IsLatest: true}); err != nil {
		t.Fatalf("create firmware: %v", err)
	}
	if err := database.SetLatestFirmware("1.1.0"); err != nil {
		t.Fatalf("set latest firmware: %v", err)
	}

	mux := ota.NewMux(database, t.TempDir()) // empty dir — no bin file
	req := signedRequest(t, http.MethodGet, "/ota/download", psk, "esp-AABBCC")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404 for missing bin, got %d", w.Code)
	}
}

func TestOTA_Download_Success(t *testing.T) {
	database := newTestDB(t)
	psk := []byte("testpsk0123456789012345678901234")

	if err := database.CreateDevice(db.Device{
		ID: "esp-AABBCC", Name: "test", TemplateID: "tpl-1", FWVersion: "1.0.0", PSK: psk,
	}); err != nil {
		t.Fatalf("create device: %v", err)
	}
	if err := database.CreateFirmware(db.FirmwareRow{Version: "1.1.0", Boards: "esp32-c3", IsLatest: true}); err != nil {
		t.Fatalf("create firmware: %v", err)
	}
	if err := database.SetLatestFirmware("1.1.0"); err != nil {
		t.Fatalf("set latest: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1.1.0.bin"), []byte("fake-firmware"), 0600); err != nil {
		t.Fatalf("write bin: %v", err)
	}

	mux := ota.NewMux(database, dir)
	req := signedRequest(t, http.MethodGet, "/ota/download", psk, "esp-AABBCC")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("want octet-stream Content-Type, got %q", ct)
	}
	if w.Body.String() != "fake-firmware" {
		t.Errorf("want body 'fake-firmware', got %q", w.Body.String())
	}

	// Verify OTA log was written
	rows, err := database.ListOTALogForDevice("esp-AABBCC")
	if err != nil {
		t.Fatalf("list ota log: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 ota log row, got %d", len(rows))
	}
	if rows[0].Result != "ok" {
		t.Errorf("want result='ok', got %q", rows[0].Result)
	}
	if rows[0].FromVer != "1.0.0" || rows[0].ToVer != "1.1.0" {
		t.Errorf("want from=1.0.0 to=1.1.0, got from=%q to=%q", rows[0].FromVer, rows[0].ToVer)
	}
}
