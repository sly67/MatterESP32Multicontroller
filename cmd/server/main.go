package main

import (
	"log"
	"os"

	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/tlsutil"
	"github.com/karthangar/matteresp32hub/internal/api"
)

func main() {
	dataDir := envOr("DATA_DIR", "./data")
	configDir := dataDir + "/config"
	dbPath := dataDir + "/db/matteresp32.db"
	certsDir := dataDir + "/certs"

	cfg, err := config.Load(configDir)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	if err := tlsutil.EnsureCerts(certsDir); err != nil {
		log.Fatalf("tls: %v", err)
	}

	srv := api.NewServer(cfg, database, certsDir)
	log.Printf("web UI:  https://0.0.0.0:%d", cfg.WebPort)
	log.Printf("OTA srv: https://0.0.0.0:%d", cfg.OTAPort)
	if err := srv.ListenAndServeTLS(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
