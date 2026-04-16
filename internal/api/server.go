package api

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
)

// Server holds the HTTP server configuration.
type Server struct {
	cfg      *config.Config
	database *db.Database
	certsDir string
}

// NewServer creates a new Server.
func NewServer(cfg *config.Config, database *db.Database, certsDir string) *Server {
	return &Server{cfg: cfg, database: database, certsDir: certsDir}
}

// ListenAndServeTLS starts the HTTPS server on the configured port.
func (s *Server) ListenAndServeTLS() error {
	cert, err := tls.LoadX509KeyPair(
		filepath.Join(s.certsDir, "server.crt"),
		filepath.Join(s.certsDir, "server.key"))
	if err != nil {
		return fmt.Errorf("load TLS cert: %w", err)
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	handler := NewRouter(s.cfg, s.database)
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.cfg.WebPort),
		Handler:           handler,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return srv.ListenAndServeTLS("", "")
}

// ListenAndServeOTA starts a placeholder HTTPS server on OTAPort.
// Full OTA handler is implemented in Plan 4.
func (s *Server) ListenAndServeOTA() error {
	cert, err := tls.LoadX509KeyPair(
		filepath.Join(s.certsDir, "server.crt"),
		filepath.Join(s.certsDir, "server.key"))
	if err != nil {
		return fmt.Errorf("load TLS cert: %w", err)
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "OTA server not yet implemented", http.StatusServiceUnavailable)
	})
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.cfg.OTAPort),
		Handler:           mux,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return srv.ListenAndServeTLS("", "")
}
