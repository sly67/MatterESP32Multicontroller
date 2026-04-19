package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModules_ListEmpty(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/modules", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var body []interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Empty(t, body)
}

func TestModules_CreateAndList(t *testing.T) {
	srv := newTestServer(t)

	payload := map[string]string{
		"id": "test-mod", "name": "Test", "category": "io",
		"yaml_body": "id: test-mod\nname: Test\nversion: \"1.0\"\ncategory: io\nio: []\nmatter:\n  endpoint_type: on_off_light\n  behaviors: []\n",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/modules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	req2 := httptest.NewRequest(http.MethodGet, "/api/modules", nil)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
	var list []map[string]interface{}
	require.NoError(t, json.NewDecoder(w2.Body).Decode(&list))
	assert.Len(t, list, 1)
	assert.Equal(t, "test-mod", list[0]["id"])
}

func TestModules_GetByID(t *testing.T) {
	srv := newTestServer(t)
	getDatabase(t, srv).CreateModule(db.ModuleRow{ID: "x", Name: "X", Category: "io", YAMLBody: "id: x\n"})

	req := httptest.NewRequest(http.MethodGet, "/api/modules/x", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestModules_GetMissing(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/modules/nope", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestModules_ESPHomeFilter(t *testing.T) {
	srv := newTestServer(t)
	database := getDatabase(t, srv)

	require.NoError(t, database.CreateModule(db.ModuleRow{
		ID: "dht22-test", Name: "DHT22", Category: "sensor",
		YAMLBody: `id: dht22-test
name: "DHT22"
version: "1.0"
category: sensor
io:
  - id: DATA
    type: digital_out
    label: "Data"
    constraints:
      digital: {active: high, initial_state: low}
matter:
  endpoint_type: temperature_sensor
  behaviors: [temperature_reporting]
esphome:
  components:
    - domain: sensor
      template: "platform: dht"
`,
	}))

	require.NoError(t, database.CreateModule(db.ModuleRow{
		ID: "no-esphome-test", Name: "No ESPHome", Category: "io",
		YAMLBody: `id: no-esphome-test
name: "No ESPHome"
version: "1.0"
category: io
io:
  - id: OUT
    type: digital_out
    label: "Out"
    constraints:
      digital: {active: high, initial_state: low}
matter:
  endpoint_type: on_off_light
  behaviors: [on_off]
`,
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/modules?esphome=true", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var mods []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&mods))
	require.Len(t, mods, 1, "only ESPHome-capable modules returned")
	assert.True(t, mods[0]["has_esphome"].(bool))
	assert.Equal(t, "dht22-test", mods[0]["id"])
}
