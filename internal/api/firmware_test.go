package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirmware_ListEmpty(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/firmware", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var body []interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Empty(t, body)
}

func TestFirmware_GetMissing(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/firmware/1.0.0", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestFirmware_ListOne(t *testing.T) {
	srv := newTestServer(t)
	database := getDatabase(t, srv)
	require.NoError(t, database.CreateFirmware(db.FirmwareRow{
		Version: "1.0.0", Boards: "esp32-c3", Notes: "test", IsLatest: true,
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/firmware", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var list []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&list))
	require.Len(t, list, 1)
	assert.Equal(t, "1.0.0", list[0]["version"])
}
