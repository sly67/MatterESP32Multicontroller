package api

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/esphome"
	"github.com/karthangar/matteresp32hub/internal/ota"
)

type Server struct {
	cfg      *config.Config
	database *db.Database
	queue    *esphome.Queue
	certsDir string
}

func NewServer(cfg *config.Config, database *db.Database, queue *esphome.Queue, certsDir string) *Server {
	return &Server{cfg: cfg, database: database, queue: queue, certsDir: certsDir}
}

func (s *Server) ListenAndServeTLS() error {
	handler := NewRouter(s.cfg, s.database, s.queue)
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.cfg.WebPort),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // SSE streams need no write timeout
		IdleTimeout:       120 * time.Second,
	}
	return srv.ListenAndServeTLS(
		filepath.Join(s.certsDir, "server.crt"),
		filepath.Join(s.certsDir, "server.key"),
	)
}

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
