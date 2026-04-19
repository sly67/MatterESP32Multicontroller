package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDevices_ListEmpty(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var body []interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Empty(t, body)
}

func TestDevices_GetMissing(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/devices/nope", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDevices_ListOne(t *testing.T) {
	srv := newTestServer(t)
	database := getDatabase(t, srv)

	require.NoError(t, database.CreateTemplate(db.TemplateRow{
		ID: "tpl-1", Name: "T1", Board: "esp32-c3", YAMLBody: "id: tpl-1\n",
	}))
	require.NoError(t, database.CreateDevice(db.Device{
		ID: "dev-1", Name: "1/Bedroom", TemplateID: "tpl-1",
		FWVersion: "1.0.0", PSK: make([]byte, 32),
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var list []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&list))
	require.Len(t, list, 1)
	assert.Equal(t, "1/Bedroom", list[0]["name"])
	assert.Nil(t, list[0]["psk"], "PSK must not appear in API response")
}

func TestDevices_GetPairing(t *testing.T) {
	srv := newTestServer(t)
	database := getDatabase(t, srv)

	require.NoError(t, database.CreateTemplate(db.TemplateRow{
		ID: "tpl-1", Name: "T1", Board: "esp32-c3", YAMLBody: "id: tpl-1\n",
	}))
	require.NoError(t, database.CreateDevice(db.Device{
		ID: "dev-1", Name: "1/Bedroom", TemplateID: "tpl-1", PSK: make([]byte, 32),
	}))
	require.NoError(t, database.UpdateDeviceMatterCreds("dev-1", 3840, 20202021))

	req := httptest.NewRequest(http.MethodGet, "/api/devices/dev-1/pairing", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, float64(3840), body["discriminator"])
	assert.Equal(t, float64(20202021), body["passcode"])
	qr, _ := body["qr_payload"].(string)
	assert.True(t, strings.HasPrefix(qr, "MT:"), "qr_payload must start with MT:")
}

func TestDevices_GetPairing_NotFound(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/devices/missing/pairing", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
