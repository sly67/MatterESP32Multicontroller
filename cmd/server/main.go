package main

import (
	"log"
	"os"

	"github.com/karthangar/matteresp32hub/internal/api"
	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/esphome"
	"github.com/karthangar/matteresp32hub/internal/fwwatch"
	"github.com/karthangar/matteresp32hub/internal/seed"
	"github.com/karthangar/matteresp32hub/internal/tlsutil"
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

	if err := seed.SeedBuiltins(database); err != nil {
		log.Fatalf("seed: %v", err)
	}
	if err := tlsutil.EnsureCerts(certsDir); err != nil {
		log.Fatalf("tls: %v", err)
	}

	fwwatch.Start(database, dataDir)

	svcURL := envOr("ESPHOME_SVC_URL", "http://localhost:6052")
	sidecar := esphome.NewClient(svcURL)
	queue := esphome.NewQueue(database, sidecar, dataDir)

	go func() {
		otaSrv := api.NewServer(cfg, database, queue, certsDir)
		if err := otaSrv.ListenAndServeOTA(); err != nil {
			log.Printf("OTA server: %v", err)
		}
	}()

	srv := api.NewServer(cfg, database, queue, certsDir)
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
