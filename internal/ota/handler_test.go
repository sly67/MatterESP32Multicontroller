package ota_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	err := database.CreateDevice(db.Device{
		ID: "esp-AABBCC", Name: "test", TemplateID: "tpl-1",
		FWVersion: "1.0.0", PSK: psk,
	})
	if err != nil {
		t.Fatalf("create device: %v", err)
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
