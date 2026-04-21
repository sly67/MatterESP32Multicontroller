package api_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/karthangar/matteresp32hub/internal/api"
	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/esphome"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newJobsTestServer creates a test server wired with a real Queue pointing at a
// mock sidecar that immediately returns a successful compile.
func newJobsTestServer(t *testing.T) (http.Handler, *db.Database) {
	t.Helper()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	// Mock sidecar: GET /health → 200, POST /compile/* → ndjson stream then ok
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/health" {
			w.WriteHeader(200)
			return
		}
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/compile/") {
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(200)
			w.Write([]byte(`{"log":"compiling..."}` + "\n"))
			w.Write([]byte(`{"result":"ok"}` + "\n"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(mock.Close)

	client := esphome.NewClient(mock.URL)
	dataDir := t.TempDir()
	queue := esphome.NewQueue(database, client, dataDir)

	cfg := &config.Config{WebPort: 48060, OTAPort: 48061, DataDir: dataDir}
	return api.NewRouter(cfg, database, queue), database
}

func TestJobs_CreateAndGet(t *testing.T) {
	srv, _ := newJobsTestServer(t)

	body := `{"board":"esp32-c3","device_name":"TestDevice","components":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)

	var created map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	id := created["id"]
	require.NotEmpty(t, id)

	// GET /api/jobs/{id}
	req2 := httptest.NewRequest(http.MethodGet, "/api/jobs/"+id, nil)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
	var got map[string]interface{}
	require.NoError(t, json.NewDecoder(w2.Body).Decode(&got))
	assert.Equal(t, id, got["ID"])
	assert.Equal(t, "TestDevice", got["DeviceName"])
}

func TestJobs_List(t *testing.T) {
	srv, _ := newJobsTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var list []interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&list))
	assert.NotNil(t, list)
}

func TestJobs_GetNotFound(t *testing.T) {
	srv, _ := newJobsTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/missing", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestJobs_Cancel(t *testing.T) {
	srv, _ := newJobsTestServer(t)

	// Create a job first
	body := `{"board":"esp32-c3","device_name":"CancelTest","components":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)

	var created map[string]string
	json.NewDecoder(w.Body).Decode(&created)
	id := created["id"]

	req2 := httptest.NewRequest(http.MethodDelete, "/api/jobs/"+id, nil)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)
	// 204 (cancelled) or 409 (already running/done — also acceptable since mock is fast)
	assert.True(t, w2.Code == http.StatusNoContent || w2.Code == http.StatusConflict)
}

func TestJobs_StreamReturnsEvents(t *testing.T) {
	srv, _ := newJobsTestServer(t)

	// Create job
	body := `{"board":"esp32-c3","device_name":"StreamTest","components":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code)
	var created map[string]string
	json.NewDecoder(w.Body).Decode(&created)
	id := created["id"]

	// Give queue a moment to process (mock sidecar is fast)
	time.Sleep(200 * time.Millisecond)

	// Stream
	req2 := httptest.NewRequest(http.MethodGet, "/api/jobs/"+id+"/stream", nil)
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Contains(t, w2.Header().Get("Content-Type"), "text/event-stream")

	// Should contain at least one "data:" line
	scanner := bufio.NewScanner(strings.NewReader(w2.Body.String()))
	found := false
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "data:") {
			found = true
			break
		}
	}
	assert.True(t, found, "SSE stream should contain at least one data line")
}
