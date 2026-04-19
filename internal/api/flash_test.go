package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlash_ESPHomeEndpointExists(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]interface{}{
		"port":           "/dev/ttyUSB0",
		"device_name":    "Test",
		"wifi_ssid":      "net",
		"wifi_password":  "pass",
		"hub_url":        "http://hub",
		"board":          "esp32-c3",
		"ha_integration": false,
		"components":     []interface{}{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/flash/esphome", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	// Port /dev/ttyUSB0 won't exist in test — expect 500 or 400, NOT 404
	assert.NotEqual(t, http.StatusNotFound, w.Code, "route must be registered")
}

func TestFlash_ListPorts(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/flash/ports", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var body []interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	// may be empty — verifies endpoint exists and returns JSON array
}
