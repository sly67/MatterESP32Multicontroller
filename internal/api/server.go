package api

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/ota"
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

// ListenAndServeTLS starts the web UI HTTPS server on the configured port.
// Uses the same self-signed cert as the OTA server so Web Serial API works
// (browsers require a secure context for navigator.serial).
func (s *Server) ListenAndServeTLS() error {
	handler := NewRouter(s.cfg, s.database)
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.cfg.WebPort),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return srv.ListenAndServeTLS(
		filepath.Join(s.certsDir, "server.crt"),
		filepath.Join(s.certsDir, "server.key"),
	)
}

// ListenAndServeOTA starts the PSK-authenticated HTTPS OTA server on OTAPort.
// ESP32 devices connect directly (no Traefik) so this server handles its own TLS.
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

	firmwareDir := filepath.Join(s.cfg.DataDir, "firmware")
	handler := ota.NewMux(s.database, firmwareDir)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.cfg.OTAPort),
		Handler:           handler,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return srv.ListenAndServeTLS("", "")
}
