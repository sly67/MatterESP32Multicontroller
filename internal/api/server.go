package api

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"path/filepath"

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
	tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}}
	handler := NewRouter(s.cfg, s.database)
	srv := &http.Server{
		Addr:      fmt.Sprintf(":%d", s.cfg.WebPort),
		Handler:   handler,
		TLSConfig: tlsCfg,
	}
	return srv.ListenAndServeTLS("", "")
}
