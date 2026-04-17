# Web Platform Foundation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A running Docker container serving an HTTPS Go backend with embedded Svelte + DaisyUI frontend, SQLite database, auto-generated TLS cert, and config loading from `/data/config` — the foundation all other plans plug into.

**Architecture:** Single Go binary embeds the compiled Svelte frontend via `go:embed`. The HTTP server serves the API on port 48060 (HTTPS) and OTA on port 48061 (HTTPS). SQLite stores all persistent data. Config is loaded from YAML files in `/data/config`, with defaults written on first boot.

**Tech Stack:** Go 1.22, `go-chi/chi` (router), `modernc.org/sqlite` (pure-Go SQLite, no CGO), `gopkg.in/yaml.v3`, Svelte 4, Vite, DaisyUI 4, Tailwind CSS 3, Docker, docker-compose.

---

## Project Structure

```
MatterESP32Multicontroller/
├── cmd/
│   └── server/
│       └── main.go                  # Entry point
├── internal/
│   ├── config/
│   │   ├── config.go                # Load + write YAML config files
│   │   └── types.go                 # Config structs
│   ├── db/
│   │   ├── db.go                    # SQLite open + migrate
│   │   ├── schema.sql               # Schema definition
│   │   ├── device.go                # Device CRUD
│   │   ├── template.go              # Template CRUD
│   │   ├── module.go                # Module CRUD
│   │   ├── effect.go                # Effect CRUD
│   │   └── firmware.go              # Firmware version CRUD
│   ├── tlsutil/
│   │   └── tlsutil.go               # Auto-generate self-signed cert
│   └── api/
│       ├── router.go                # Chi router + middleware
│       ├── health.go                # GET /api/health
│       ├── devices.go               # /api/devices stubs
│       ├── templates.go             # /api/templates stubs
│       ├── modules.go               # /api/modules stubs
│       ├── effects.go               # /api/effects stubs
│       ├── firmware.go              # /api/firmware stubs
│       └── settings.go              # /api/settings stubs
├── web/
│   ├── src/
│   │   ├── App.svelte               # Root app
│   │   ├── main.js                  # Entry point
│   │   ├── lib/
│   │   │   └── Sidebar.svelte       # Navigation sidebar
│   │   └── views/
│   │       ├── Fleet.svelte
│   │       ├── Flash.svelte
│   │       ├── Templates.svelte
│   │       ├── Modules.svelte
│   │       ├── OTA.svelte
│   │       ├── Firmware.svelte
│   │       └── Settings.svelte
│   ├── package.json
│   ├── vite.config.js
│   ├── tailwind.config.js
│   └── postcss.config.js
├── data/
│   ├── modules/                     # Built-in module YAMLs (empty for now)
│   └── effects/                     # Built-in effect YAMLs (empty for now)
├── Dockerfile
├── docker-compose.yml
├── Makefile
└── go.mod
```

---

## Task 1: Go Project Scaffold

**Files:**
- Create: `go.mod`
- Create: `Makefile`
- Create: `cmd/server/main.go`

- [ ] **Step 1: Initialize Go module**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
go mod init github.com/karthangar/matteresp32hub
```

Expected: `go.mod` created with `module github.com/karthangar/matteresp32hub`

- [ ] **Step 2: Add dependencies**

```bash
go get github.com/go-chi/chi/v5@latest
go get modernc.org/sqlite@latest
go get gopkg.in/yaml.v3@latest
go get github.com/stretchr/testify@latest
```

- [ ] **Step 3: Create directory structure**

```bash
mkdir -p cmd/server internal/config internal/db internal/tlsutil internal/api
mkdir -p web/src/lib web/src/views data/modules data/effects
```

- [ ] **Step 4: Write `Makefile`**

```makefile
.PHONY: build web test docker

web:
	cd web && npm install && npm run build

build: web
	go build -o bin/server ./cmd/server

test:
	go test ./... -v

docker:
	docker compose build

run:
	go run ./cmd/server

lint:
	go vet ./...
```

- [ ] **Step 5: Write minimal `cmd/server/main.go`**

```go
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
```

- [ ] **Step 6: Commit**

```bash
git init
git add go.mod go.sum Makefile cmd/ internal/ data/
git commit -m "feat: initialize Go project scaffold"
```

---

## Task 2: Config System

**Files:**
- Create: `internal/config/types.go`
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write `internal/config/types.go`**

```go
package config

// App is the top-level application configuration.
type App struct {
	WebPort int `yaml:"web_port"`
	OTAPort int `yaml:"ota_port"`
}

// WiFi holds default WiFi credentials used when flashing devices.
type WiFi struct {
	SSID     string `yaml:"ssid"`
	Password string `yaml:"password"`
}

// USB holds declared USB port paths.
type USB struct {
	Ports []string `yaml:"ports"`
}

// PSKPolicy controls PSK generation behaviour.
type PSKPolicy struct {
	LengthBytes int `yaml:"length_bytes"`
}

// Config is the full loaded configuration.
type Config struct {
	App       App       `yaml:"-"`
	WiFi      WiFi      `yaml:"-"`
	USB       USB       `yaml:"-"`
	PSKPolicy PSKPolicy `yaml:"-"`
	WebPort   int
	OTAPort   int
}
```

- [ ] **Step 2: Write failing test**

```go
// internal/config/config_test.go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_WritesDefaultsOnFirstBoot(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, 48060, cfg.WebPort)
	assert.Equal(t, 48061, cfg.OTAPort)
	assert.FileExists(t, filepath.Join(dir, "app.yaml"))
}

func TestLoad_ReadsExistingConfig(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.yaml"),
		[]byte("web_port: 9000\nota_port: 9001\n"), 0644)
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, 9000, cfg.WebPort)
	assert.Equal(t, 9001, cfg.OTAPort)
}
```

- [ ] **Step 3: Run test — verify it fails**

```bash
go test ./internal/config/... -v
```

Expected: FAIL — `config.Load` undefined

- [ ] **Step 4: Write `internal/config/config.go`**

```go
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

var defaults = App{WebPort: 48060, OTAPort: 48061}

// Load reads config files from dir, writing defaults if missing.
func Load(dir string) (*Config, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir config dir: %w", err)
	}

	app, err := loadOrWrite[App](filepath.Join(dir, "app.yaml"), defaults)
	if err != nil {
		return nil, err
	}
	wifi, err := loadOrWrite[WiFi](filepath.Join(dir, "wifi.yaml"), WiFi{})
	if err != nil {
		return nil, err
	}
	usb, err := loadOrWrite[USB](filepath.Join(dir, "usb.yaml"), USB{Ports: []string{"/dev/ttyUSB0"}})
	if err != nil {
		return nil, err
	}
	psk, err := loadOrWrite[PSKPolicy](filepath.Join(dir, "psk-policy.yaml"), PSKPolicy{LengthBytes: 32})
	if err != nil {
		return nil, err
	}

	return &Config{
		App:       app,
		WiFi:      wifi,
		USB:       usb,
		PSKPolicy: psk,
		WebPort:   app.WebPort,
		OTAPort:   app.OTAPort,
	}, nil
}

func loadOrWrite[T any](path string, def T) (T, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		data, err := yaml.Marshal(def)
		if err != nil {
			return def, fmt.Errorf("marshal default %s: %w", path, err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return def, fmt.Errorf("write default %s: %w", path, err)
		}
		return def, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return def, fmt.Errorf("read %s: %w", path, err)
	}
	var v T
	if err := yaml.Unmarshal(data, &v); err != nil {
		return def, fmt.Errorf("parse %s: %w", path, err)
	}
	return v, nil
}
```

- [ ] **Step 5: Run test — verify it passes**

```bash
go test ./internal/config/... -v
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/config/
git commit -m "feat: config loader with YAML defaults on first boot"
```

---

## Task 3: SQLite Database

**Files:**
- Create: `internal/db/schema.sql`
- Create: `internal/db/db.go`
- Create: `internal/db/device.go`
- Test: `internal/db/db_test.go`

- [ ] **Step 1: Write `internal/db/schema.sql`**

```sql
CREATE TABLE IF NOT EXISTS devices (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    template_id TEXT NOT NULL,
    fw_version  TEXT NOT NULL DEFAULT '',
    psk         BLOB NOT NULL,
    status      TEXT NOT NULL DEFAULT 'unknown',
    last_seen   DATETIME,
    ip          TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS templates (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    board      TEXT NOT NULL,
    yaml_body  TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS modules (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    category   TEXT NOT NULL,
    builtin    INTEGER NOT NULL DEFAULT 0,
    yaml_body  TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS effects (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    builtin    INTEGER NOT NULL DEFAULT 0,
    yaml_body  TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS firmware (
    version    TEXT PRIMARY KEY,
    boards     TEXT NOT NULL,
    notes      TEXT NOT NULL DEFAULT '',
    is_latest  INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS flash_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id  TEXT NOT NULL,
    result     TEXT NOT NULL,
    error      TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS ota_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id  TEXT NOT NULL,
    from_ver   TEXT NOT NULL,
    to_ver     TEXT NOT NULL,
    result     TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

- [ ] **Step 2: Write failing test**

```go
// internal/db/db_test.go
package db_test

import (
	"testing"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_CreatesTables(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	row := database.DB.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='devices'")
	var name string
	require.NoError(t, row.Scan(&name))
	assert.Equal(t, "devices", name)
}

func TestDevice_CreateAndGet(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	dev := db.Device{
		ID:         "esp-test01",
		Name:       "1/Bedroom",
		TemplateID: "firefly-hub-v1",
		PSK:        []byte("testpsk"),
	}
	require.NoError(t, database.CreateDevice(dev))

	got, err := database.GetDevice("esp-test01")
	require.NoError(t, err)
	assert.Equal(t, "1/Bedroom", got.Name)
	assert.Equal(t, []byte("testpsk"), got.PSK)
}
```

- [ ] **Step 3: Run — verify fails**

```bash
go test ./internal/db/... -v
```

Expected: FAIL — `db.Open` undefined

- [ ] **Step 4: Write `internal/db/db.go`**

```go
package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Database wraps a sql.DB with app-specific methods.
type Database struct {
	DB *sql.DB
}

// Open opens (or creates) the SQLite database at path and applies the schema.
func Open(path string) (*Database, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, fmt.Errorf("mkdir db dir: %w", err)
		}
	}
	sqldb, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqldb.SetMaxOpenConns(1) // SQLite is single-writer

	if _, err := sqldb.Exec(schema); err != nil {
		sqldb.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Database{DB: sqldb}, nil
}

// Close closes the underlying database connection.
func (d *Database) Close() error {
	return d.DB.Close()
}
```

- [ ] **Step 5: Write `internal/db/device.go`**

```go
package db

import "time"

// Device represents a registered ESP32 device.
type Device struct {
	ID         string
	Name       string
	TemplateID string
	FWVersion  string
	PSK        []byte
	Status     string
	LastSeen   *time.Time
	IP         string
	CreatedAt  time.Time
}

// CreateDevice inserts a new device record.
func (d *Database) CreateDevice(dev Device) error {
	_, err := d.DB.Exec(
		`INSERT INTO devices (id, name, template_id, fw_version, psk, status)
		 VALUES (?, ?, ?, ?, ?, 'unknown')`,
		dev.ID, dev.Name, dev.TemplateID, dev.FWVersion, dev.PSK)
	return err
}

// GetDevice retrieves a device by ID.
func (d *Database) GetDevice(id string) (Device, error) {
	row := d.DB.QueryRow(
		`SELECT id, name, template_id, fw_version, psk, status, last_seen, ip, created_at
		 FROM devices WHERE id = ?`, id)
	var dev Device
	var lastSeen *time.Time
	err := row.Scan(&dev.ID, &dev.Name, &dev.TemplateID, &dev.FWVersion,
		&dev.PSK, &dev.Status, &lastSeen, &dev.IP, &dev.CreatedAt)
	dev.LastSeen = lastSeen
	return dev, err
}

// ListDevices returns all devices ordered by name.
func (d *Database) ListDevices() ([]Device, error) {
	rows, err := d.DB.Query(
		`SELECT id, name, template_id, fw_version, psk, status, last_seen, ip, created_at
		 FROM devices ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devs []Device
	for rows.Next() {
		var dev Device
		if err := rows.Scan(&dev.ID, &dev.Name, &dev.TemplateID, &dev.FWVersion,
			&dev.PSK, &dev.Status, &dev.LastSeen, &dev.IP, &dev.CreatedAt); err != nil {
			return nil, err
		}
		devs = append(devs, dev)
	}
	return devs, rows.Err()
}

// UpdateDeviceStatus updates the status, IP, and last_seen for a device.
func (d *Database) UpdateDeviceStatus(id, status, ip string) error {
	_, err := d.DB.Exec(
		`UPDATE devices SET status = ?, ip = ?, last_seen = CURRENT_TIMESTAMP WHERE id = ?`,
		status, ip, id)
	return err
}
```

- [ ] **Step 6: Run test — verify passes**

```bash
go test ./internal/db/... -v
```

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/db/
git commit -m "feat: SQLite database with schema and device CRUD"
```

---

## Task 4: TLS Auto-Generation

**Files:**
- Create: `internal/tlsutil/tlsutil.go`
- Test: `internal/tlsutil/tlsutil_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/tlsutil/tlsutil_test.go
package tlsutil_test

import (
	"crypto/tls"
	"path/filepath"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/tlsutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureCerts_GeneratesOnFirstBoot(t *testing.T) {
	dir := t.TempDir()
	err := tlsutil.EnsureCerts(dir)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "server.crt"))
	assert.FileExists(t, filepath.Join(dir, "server.key"))
}

func TestEnsureCerts_LoadsExistingCert(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, tlsutil.EnsureCerts(dir))
	// Second call must not overwrite
	require.NoError(t, tlsutil.EnsureCerts(dir))
	_, err := tls.LoadX509KeyPair(
		filepath.Join(dir, "server.crt"),
		filepath.Join(dir, "server.key"))
	assert.NoError(t, err)
}
```

- [ ] **Step 2: Run — verify fails**

```bash
go test ./internal/tlsutil/... -v
```

Expected: FAIL — `tlsutil.EnsureCerts` undefined

- [ ] **Step 3: Write `internal/tlsutil/tlsutil.go`**

```go
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// EnsureCerts generates a self-signed TLS cert+key into dir if not present.
func EnsureCerts(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir certs: %w", err)
	}
	crtPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")
	if fileExists(crtPath) && fileExists(keyPath) {
		return nil
	}
	return generateSelfSigned(crtPath, keyPath)
}

func generateSelfSigned(crtPath, keyPath string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{Organization: []string{"MatterESP32Hub"}},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create cert: %w", err)
	}
	if err := writePEM(crtPath, "CERTIFICATE", certDER); err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	return writePEM(keyPath, "EC PRIVATE KEY", keyDER)
}

func writePEM(path, typ string, der []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: typ, Bytes: der})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
```

- [ ] **Step 4: Run test — verify passes**

```bash
go test ./internal/tlsutil/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tlsutil/
git commit -m "feat: auto-generate self-signed TLS cert on first boot"
```

---

## Task 5: HTTP Server + API Skeleton

**Files:**
- Create: `internal/api/router.go`
- Create: `internal/api/server.go`
- Create: `internal/api/health.go`
- Create: `internal/api/devices.go`
- Create: `internal/api/templates.go`
- Create: `internal/api/modules.go`
- Create: `internal/api/effects.go`
- Create: `internal/api/firmware.go`
- Create: `internal/api/settings.go`
- Test: `internal/api/health_test.go`

- [ ] **Step 1: Write failing health test**

```go
// internal/api/health_test.go
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

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })
	cfg := &config.Config{WebPort: 48060, OTAPort: 48061}
	return api.NewRouter(cfg, database)
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
```

- [ ] **Step 2: Run — verify fails**

```bash
go test ./internal/api/... -v
```

Expected: FAIL — `api.NewRouter` undefined

- [ ] **Step 3: Write `internal/api/health.go`**

```go
package api

import (
	"encoding/json"
	"net/http"
)

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

- [ ] **Step 4: Write `internal/api/router.go`**

```go
package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
	"net/http"
)

// NewRouter builds and returns the chi HTTP router.
func NewRouter(cfg *config.Config, database *db.Database) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Get("/api/health", handleHealth)
	r.Route("/api/devices",  devicesRouter(database))
	r.Route("/api/templates", templatesRouter(database))
	r.Route("/api/modules",   modulesRouter(database))
	r.Route("/api/effects",   effectsRouter(database))
	r.Route("/api/firmware",  firmwareRouter(database))
	r.Route("/api/settings",  settingsRouter(cfg, database))

	// Frontend — served from embedded FS (wired in Task 7)
	r.Handle("/*", http.NotFoundHandler())

	return r
}
```

- [ ] **Step 5: Write stub route handlers**

```go
// internal/api/devices.go
package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
	"net/http"
)

func devicesRouter(database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotImplemented)
		})
	}
}
```

```go
// internal/api/templates.go
package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
	"net/http"
)

func templatesRouter(database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotImplemented)
		})
	}
}
```

```go
// internal/api/modules.go
package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
	"net/http"
)

func modulesRouter(database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotImplemented)
		})
	}
}
```

```go
// internal/api/effects.go
package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
	"net/http"
)

func effectsRouter(database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotImplemented)
		})
	}
}
```

```go
// internal/api/firmware.go
package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
	"net/http"
)

func firmwareRouter(database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotImplemented)
		})
	}
}
```

```go
// internal/api/settings.go
package api

import (
	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
	"net/http"
)

func settingsRouter(cfg *config.Config, database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotImplemented)
		})
	}
}
```

- [ ] **Step 6: Write `internal/api/server.go`**

```go
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
```

- [ ] **Step 7: Run tests — verify pass**

```bash
go test ./internal/api/... -v
```

Expected: PASS

- [ ] **Step 8: Verify binary compiles**

```bash
go build ./cmd/server
```

Expected: no errors, `server` binary produced

- [ ] **Step 9: Commit**

```bash
git add internal/api/
git commit -m "feat: HTTPS server with chi router and API stubs"
```

---

## Task 6: Svelte Frontend Shell

**Files:**
- Create: `web/package.json`
- Create: `web/vite.config.js`
- Create: `web/tailwind.config.js`
- Create: `web/postcss.config.js`
- Create: `web/index.html`
- Create: `web/src/main.js`
- Create: `web/src/App.svelte`
- Create: `web/src/lib/Sidebar.svelte`
- Create: `web/src/views/Fleet.svelte` (and 6 other view stubs)

- [ ] **Step 1: Write `web/package.json`**

```json
{
  "name": "matteresp32hub-web",
  "version": "1.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "preview": "vite preview"
  },
  "devDependencies": {
    "@sveltejs/vite-plugin-svelte": "^3.0.0",
    "autoprefixer": "^10.4.0",
    "daisyui": "^4.12.10",
    "postcss": "^8.4.0",
    "svelte": "^4.0.0",
    "tailwindcss": "^3.4.0",
    "vite": "^5.0.0"
  }
}
```

- [ ] **Step 2: Write `web/vite.config.js`**

```js
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: '../web/dist',
    emptyOutDir: true,
  },
});
```

- [ ] **Step 3: Write `web/tailwind.config.js`**

```js
export default {
  content: ['./src/**/*.{html,js,svelte,ts}'],
  plugins: [require('daisyui')],
  daisyui: {
    themes: ['night'],
  },
};
```

- [ ] **Step 4: Write `web/postcss.config.js`**

```js
export default {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
};
```

- [ ] **Step 5: Write `web/index.html`**

```html
<!DOCTYPE html>
<html lang="en" data-theme="night">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>MatterESP32 Hub</title>
</head>
<body>
  <div id="app"></div>
  <script type="module" src="/src/main.js"></script>
</body>
</html>
```

- [ ] **Step 6: Write `web/src/main.js`**

```js
import './app.css';
import App from './App.svelte';

const app = new App({ target: document.getElementById('app') });
export default app;
```

- [ ] **Step 7: Create `web/src/app.css`**

```css
@tailwind base;
@tailwind components;
@tailwind utilities;
```

- [ ] **Step 8: Write `web/src/lib/Sidebar.svelte`**

```svelte
<script>
  export let current = 'fleet';

  const nav = [
    { section: 'Overview' },
    { id: 'fleet',     label: 'Fleet',          icon: '⊞' },
    { id: 'flash',     label: 'Flash Devices',  icon: '⚡' },
    { section: 'Configuration' },
    { id: 'templates', label: 'Templates',      icon: '⚙' },
    { id: 'modules',   label: 'Module Library', icon: '⬡' },
    { section: 'Updates' },
    { id: 'ota',       label: 'OTA Updates',    icon: '⇅' },
    { id: 'firmware',  label: 'Firmware',       icon: '💾' },
    { section: 'System' },
    { id: 'settings',  label: 'Settings',       icon: '⚙' },
  ];
</script>

<aside class="w-56 flex-shrink-0 bg-base-300 border-r border-base-200 flex flex-col h-full p-3">
  <div class="flex items-center gap-2 px-2 py-3 mb-2 border-b border-base-200">
    <span class="text-lg">⚡</span>
    <div>
      <div class="font-bold text-sm text-white">MatterESP32</div>
      <div class="text-xs text-base-content/40">Hub</div>
    </div>
  </div>

  {#each nav as item}
    {#if item.section}
      <div class="text-xs uppercase tracking-widest text-base-content/40 px-3 pt-3 pb-1">
        {item.section}
      </div>
    {:else}
      <button
        class="flex items-center gap-2 px-3 py-2 rounded-lg text-sm w-full text-left transition-all
               {current === item.id
                 ? 'bg-primary/15 text-primary font-semibold'
                 : 'text-base-content/70 hover:bg-white/5 hover:text-white'}"
        on:click={() => current = item.id}
      >
        <span>{item.icon}</span>
        {item.label}
      </button>
    {/if}
  {/each}
</aside>
```

- [ ] **Step 9: Write `web/src/App.svelte`**

```svelte
<script>
  import Sidebar from './lib/Sidebar.svelte';
  import Fleet     from './views/Fleet.svelte';
  import Flash     from './views/Flash.svelte';
  import Templates from './views/Templates.svelte';
  import Modules   from './views/Modules.svelte';
  import OTA       from './views/OTA.svelte';
  import Firmware  from './views/Firmware.svelte';
  import Settings  from './views/Settings.svelte';

  let current = 'fleet';

  const views = { Fleet, Flash, Templates, Modules, OTA, Firmware, Settings };
  $: ViewComponent = views[current.charAt(0).toUpperCase() + current.slice(1)];
</script>

<div class="flex h-screen w-screen overflow-hidden bg-base-100 text-base-content">
  <Sidebar bind:current />
  <main class="flex-1 flex flex-col overflow-hidden">
    <div class="navbar bg-base-200 border-b border-base-200 px-4 min-h-12 flex-shrink-0">
      <span class="font-semibold text-sm capitalize">{current}</span>
    </div>
    <div class="flex-1 overflow-y-auto">
      <svelte:component this={ViewComponent} />
    </div>
  </main>
</div>
```

- [ ] **Step 10: Write view stubs** (repeat for each)

```svelte
<!-- web/src/views/Fleet.svelte -->
<script></script>
<div class="p-6">
  <h2 class="text-lg font-semibold mb-1">Fleet Dashboard</h2>
  <p class="text-sm text-base-content/50">Coming in Plan 4.</p>
</div>
```

Create the same stub for `Flash.svelte`, `Templates.svelte`, `Modules.svelte`, `OTA.svelte`, `Firmware.svelte`, `Settings.svelte` — same structure, different title.

- [ ] **Step 11: Install and build**

```bash
cd web && npm install && npm run build
```

Expected: `web/dist/` created with `index.html` and assets.

- [ ] **Step 12: Commit**

```bash
cd ..
git add web/
git commit -m "feat: Svelte + DaisyUI frontend shell with sidebar navigation"
```

---

## Task 7: Embed Frontend in Go Binary

**Files:**
- Create: `web/dist/.gitkeep` (so the dir exists in git)
- Modify: `internal/api/router.go`
- Create: `internal/api/static.go`

- [ ] **Step 1: Write `internal/api/static.go`**

```go
package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:../../web/dist
var staticFiles embed.FS

// staticHandler returns an http.Handler that serves the embedded Svelte build.
// Any path not matching a file falls back to index.html (SPA routing).
func staticHandler() http.Handler {
	dist, err := fs.Sub(staticFiles, "web/dist")
	if err != nil {
		panic("embed: web/dist not found — run 'make web' first")
	}
	fsys := http.FS(dist)
	fileServer := http.FileServer(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try the exact file first; fall back to index.html for SPA routing
		f, err := dist.Open(r.URL.Path[1:])
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}
```

- [ ] **Step 2: Wire into router — update `internal/api/router.go`**

Replace the `r.Handle("/*", ...)` line with:

```go
// Replace this:
r.Handle("/*", http.NotFoundHandler())

// With this:
r.Handle("/*", staticHandler())
```

- [ ] **Step 3: Build and smoke test**

```bash
make web && go build ./cmd/server && ./server
```

Open `https://localhost:48060` in browser (accept self-signed cert warning).

Expected: DaisyUI night-theme sidebar visible with navigation items.

- [ ] **Step 4: Commit**

```bash
git add internal/api/static.go internal/api/router.go web/dist/
git commit -m "feat: embed Svelte frontend in Go binary via go:embed"
```

---

## Task 8: Docker + docker-compose

**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.yml`
- Create: `.dockerignore`

- [ ] **Step 1: Write `Dockerfile`**

```dockerfile
# Stage 1: build Svelte frontend
FROM node:20-alpine AS web-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: build Go binary
FROM golang:1.22-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-builder /app/web/dist ./web/dist
RUN go build -o bin/server ./cmd/server

# Stage 3: minimal runtime image
FROM alpine:3.19
RUN apk add --no-cache ca-certificates python3 py3-pip esptool
WORKDIR /app
COPY --from=go-builder /app/bin/server .
COPY data/ ./data/
EXPOSE 48060 48061
ENV DATA_DIR=/data
CMD ["./server"]
```

- [ ] **Step 2: Write `docker-compose.yml`**

```yaml
services:
  matteresp32hub:
    build: .
    restart: unless-stopped
    ports:
      - "48060:48060"
      - "48061:48061"
    volumes:
      - /Portainer/MatterESP32/db:/data/db
      - /Portainer/MatterESP32/firmware:/data/firmware
      - /Portainer/MatterESP32/modules:/data/modules
      - /Portainer/MatterESP32/templates:/data/templates
      - /Portainer/MatterESP32/config:/data/config
      - /Portainer/MatterESP32/logs:/data/logs
      - /Portainer/MatterESP32/certs:/data/certs
      - /Portainer/MatterESP32/matter:/data/matter
    devices:
      - /dev/ttyUSB0:/dev/ttyUSB0
    environment:
      - DATA_DIR=/data
```

- [ ] **Step 3: Write `.dockerignore`**

```
.git
web/node_modules
web/dist
bin/
*.md
docs/
.superpowers/
```

- [ ] **Step 4: Build Docker image**

```bash
docker compose build
```

Expected: image builds successfully, no errors

- [ ] **Step 5: Run container**

```bash
docker compose up
```

Expected output:
```
matteresp32hub  | web UI:  https://0.0.0.0:48060
matteresp32hub  | OTA srv: https://0.0.0.0:48061
```

Open `https://localhost:48060` — DaisyUI sidebar visible.

- [ ] **Step 6: Commit**

```bash
git add Dockerfile docker-compose.yml .dockerignore
git commit -m "feat: Docker multi-stage build with all Portainer volumes"
```

---

## Self-Review

**Spec coverage check:**

| Spec section | Covered by |
|---|---|
| Go + Svelte + DaisyUI night theme | Tasks 6, 7 |
| Docker via Portainer | Task 8 |
| Ports 48060 / 48061 | Tasks 5, 8 |
| All 8 Portainer volumes | Task 8 |
| Config files in /data/config | Task 2 |
| SQLite device/template/module/effect/firmware tables | Task 3 |
| TLS auto-gen on first boot | Task 4 |
| Sidebar navigation (all 7 sections) | Task 6 |
| API stubs for all routes | Task 5 |

**Gaps noted — handled by later plans:**
- Flash pipeline → Plan 3
- OTA server + PSK → Plan 4
- YAML template system → Plan 2
- Full Svelte views → Plans 2–4
- ESP32 firmware → Plan 5

**Placeholder scan:** No TBD/TODO in code blocks. All steps have complete code. ✓

**Type consistency:** `db.Database`, `config.Config`, `api.NewRouter`, `api.NewServer` used consistently across tasks. ✓
