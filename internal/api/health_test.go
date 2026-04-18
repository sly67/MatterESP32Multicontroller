package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/api"
	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testDBs = map[*testing.T]*db.Database{}

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() {
		delete(testDBs, t)
		database.Close()
	})
	testDBs[t] = database
	cfg := &config.Config{WebPort: 48060, OTAPort: 48061}
	return api.NewRouter(cfg, database)
}

func newTestServerWithDataDir(t *testing.T, dataDir string) http.Handler {
	t.Helper()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() {
		delete(testDBs, t)
		database.Close()
	})
	testDBs[t] = database
	cfg := &config.Config{WebPort: 48060, OTAPort: 48061, DataDir: dataDir}
	return api.NewRouter(cfg, database)
}

func getDatabase(t *testing.T, _ http.Handler) *db.Database {
	t.Helper()
	d, ok := testDBs[t]
	require.True(t, ok, "no database registered for this test")
	return d
}

func TestHealth(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "ok", body["status"])
}
