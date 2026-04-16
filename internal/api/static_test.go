package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStaticHandler_RootReturnsHTML(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
}

func TestStaticHandler_SPAFallback(t *testing.T) {
	srv := newTestServer(t)
	// /fleet is a SPA route — no real file, should return index.html
	req := httptest.NewRequest(http.MethodGet, "/fleet", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	body := w.Body.String()
	assert.True(t, strings.Contains(body, "MatterESP32"), "expected index.html content in SPA fallback")
}

func TestStaticHandler_AssetServedDirectly(t *testing.T) {
	srv := newTestServer(t)
	// /assets/ path exists in dist — should serve the file, not fall back
	req := httptest.NewRequest(http.MethodGet, "/assets/", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	// /assets/ is a directory — should fall back to index.html, NOT serve a listing
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
}
