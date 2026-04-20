# OTA Server — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a PSK-authenticated HTTPS OTA server on port 48061 that ESP32 devices poll to check for firmware updates and download new binaries, with a fleet-wide OTA management UI.

**Architecture:** New `internal/ota` package handles HMAC-SHA256 PSK authentication and the two device-facing endpoints (`GET /ota/check`, `GET /ota/download`). The existing placeholder in `ListenAndServeOTA` is replaced with the real mux. Device check-ins update `last_seen`, `ip`, and `fw_version` in the DB. The existing `ota_log` table records each update cycle. The OTA Svelte view becomes a live fleet dashboard with per-device update status and a bulk-push trigger.

**Tech Stack:** Go stdlib (`crypto/hmac`, `crypto/sha256`, `net/http`), chi router, SQLite via existing `db` package, Svelte + DaisyUI for the UI.

---

## Protocol Reference

**Authentication** — every device request must include:
```
X-Device-ID:  esp-AABBCC
X-Timestamp:  1713345600          (unix seconds, server rejects if |now - ts| > 5 min)
X-HMAC:       base64(HMAC-SHA256(PSK, "METHOD:/ota/path:TIMESTAMP"))
```

**GET /ota/check** — device heartbeat + update check
Request also carries `X-FW-Version: 1.0.0` (current firmware version on device).
Server updates `devices` row (last_seen, ip, fw_version) and responds:
```json
{ "latest_version": "1.1.0", "update_available": true }
```

**GET /ota/download** — download latest firmware binary
Responds with `Content-Type: application/octet-stream`, streams the binary from `/data/firmware/<latest_version>.bin`.
Server inserts a row into `ota_log` (from_ver → latest_ver, result=`"ok"`).

---

## File Map

| Action   | Path                                 | Responsibility |
|----------|--------------------------------------|----------------|
| Create   | `internal/ota/auth.go`               | HMAC-SHA256 PSK verification, timestamp replay protection |
| Create   | `internal/ota/handler.go`            | `/ota/check` + `/ota/download` HTTP handlers, mux builder |
| Create   | `internal/ota/handler_test.go`       | Integration tests for both endpoints |
| Create   | `internal/db/ota_log.go`             | `CreateOTALog`, `ListOTALogForDevice` |
| Modify   | `internal/db/device.go`              | Add `UpdateDeviceFWVersion(id, version string) error` |
| Modify   | `internal/api/server.go`             | Replace stub `ListenAndServeOTA` with real `ota.NewMux` |
| Modify   | `web/src/views/OTA.svelte`           | Live fleet OTA dashboard — per-device status + bulk push |

---

## Task 1: DB additions — OTA log CRUD + fw_version update

**Files:**
- Create: `internal/db/ota_log.go`
- Modify: `internal/db/device.go`

- [ ] **Step 1: Add `UpdateDeviceFWVersion` to device.go**

In `internal/db/device.go`, add after `UpdateDeviceStatus`:

```go
// UpdateDeviceFWVersion records the firmware version reported by a device on check-in.
func (d *Database) UpdateDeviceFWVersion(id, fwVersion, ip string) error {
    _, err := d.DB.Exec(
        `UPDATE devices SET fw_version = ?, ip = ?, last_seen = CURRENT_TIMESTAMP, status = 'online' WHERE id = ?`,
        fwVersion, ip, id)
    return err
}
```

- [ ] **Step 2: Create `internal/db/ota_log.go`**

```go
package db

import "time"

// OTALogRow is one OTA update event.
type OTALogRow struct {
    ID        int64     `json:"id"`
    DeviceID  string    `json:"device_id"`
    FromVer   string    `json:"from_ver"`
    ToVer     string    `json:"to_ver"`
    Result    string    `json:"result"`
    CreatedAt time.Time `json:"created_at"`
}

// CreateOTALog inserts an OTA event. Result is typically "ok" or "error".
func (d *Database) CreateOTALog(entry OTALogRow) error {
    _, err := d.DB.Exec(
        `INSERT INTO ota_log (device_id, from_ver, to_ver, result) VALUES (?, ?, ?, ?)`,
        entry.DeviceID, entry.FromVer, entry.ToVer, entry.Result)
    return err
}

// ListOTALogForDevice returns the 20 most recent OTA events for a device.
func (d *Database) ListOTALogForDevice(deviceID string) ([]OTALogRow, error) {
    rows, err := d.DB.Query(
        `SELECT id, device_id, from_ver, to_ver, result, created_at
         FROM ota_log WHERE device_id = ? ORDER BY created_at DESC LIMIT 20`,
        deviceID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []OTALogRow
    for rows.Next() {
        var r OTALogRow
        if err := rows.Scan(&r.ID, &r.DeviceID, &r.FromVer, &r.ToVer, &r.Result, &r.CreatedAt); err != nil {
            return nil, err
        }
        out = append(out, r)
    }
    return out, rows.Err()
}
```

- [ ] **Step 3: Build to verify**

```bash
/usr/local/go/bin/go build ./...
```
Expected: no output (clean build).

- [ ] **Step 4: Commit**

```bash
git add internal/db/device.go internal/db/ota_log.go
git commit -m "feat: db additions for OTA — fw_version update + ota_log CRUD"
```

---

## Task 2: OTA auth middleware

**Files:**
- Create: `internal/ota/auth.go`

- [ ] **Step 1: Write the failing test first**

Create `internal/ota/handler_test.go` with auth tests:

```go
package ota_test

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/base64"
    "fmt"
    "net/http"
    "net/http/httptest"
    "strconv"
    "testing"
    "time"

    "github.com/karthangar/matteresp32hub/internal/db"
    "github.com/karthangar/matteresp32hub/internal/ota"
    "github.com/karthangar/matteresp32hub/internal/tlsutil"
)

// newTestDB opens an in-memory SQLite database for testing.
func newTestDB(t *testing.T) *db.Database {
    t.Helper()
    database, err := db.Open(":memory:")
    if err != nil {
        t.Fatalf("open db: %v", err)
    }
    t.Cleanup(func() { database.DB.Close() })
    return database
}

func signedRequest(t *testing.T, method, path string, psk []byte, deviceID string) *http.Request {
    t.Helper()
    ts := strconv.FormatInt(time.Now().Unix(), 10)
    msg := fmt.Sprintf("%s:%s:%s", method, path, ts)
    h := hmac.New(sha256.New, psk)
    h.Write([]byte(msg))
    sig := base64.StdEncoding.EncodeToString(h.Sum(nil))

    req := httptest.NewRequest(method, path, nil)
    req.Header.Set("X-Device-ID", deviceID)
    req.Header.Set("X-Timestamp", ts)
    req.Header.Set("X-HMAC", sig)
    req.Header.Set("X-FW-Version", "1.0.0")
    return req
}

func TestOTA_MissingAuthHeaders(t *testing.T) {
    database := newTestDB(t)
    mux := ota.NewMux(database, t.TempDir())

    req := httptest.NewRequest(http.MethodGet, "/ota/check", nil)
    w := httptest.NewRecorder()
    mux.ServeHTTP(w, req)

    if w.Code != http.StatusUnauthorized {
        t.Errorf("want 401, got %d", w.Code)
    }
}

func TestOTA_Check_ValidAuth(t *testing.T) {
    database := newTestDB(t)
    psk := []byte("testpsk0123456789012345678901234") // 32 bytes

    // Register a device
    err := database.CreateDevice(db.Device{
        ID: "esp-AABBCC", Name: "test", TemplateID: "drv8833-bicolor-strip-c3",
        FWVersion: "1.0.0", PSK: psk,
    })
    if err != nil {
        t.Fatalf("create device: %v", err)
    }

    mux := ota.NewMux(database, t.TempDir())

    req := signedRequest(t, http.MethodGet, "/ota/check", psk, "esp-AABBCC")
    w := httptest.NewRecorder()
    mux.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("want 200, got %d: %s", w.Code, w.Body.String())
    }
}

func TestOTA_Check_WrongHMAC(t *testing.T) {
    database := newTestDB(t)
    psk := []byte("testpsk0123456789012345678901234")
    wrongPSK := []byte("wrongpsk012345678901234567890123")

    err := database.CreateDevice(db.Device{
        ID: "esp-AABBCC", Name: "test", TemplateID: "drv8833-bicolor-strip-c3",
        FWVersion: "1.0.0", PSK: psk,
    })
    if err != nil {
        t.Fatalf("create device: %v", err)
    }

    mux := ota.NewMux(database, t.TempDir())

    req := signedRequest(t, http.MethodGet, "/ota/check", wrongPSK, "esp-AABBCC")
    w := httptest.NewRecorder()
    mux.ServeHTTP(w, req)

    if w.Code != http.StatusUnauthorized {
        t.Errorf("want 401, got %d", w.Code)
    }
}
```

- [ ] **Step 2: Run tests — expect compile failure (ota package missing)**

```bash
/usr/local/go/bin/go test ./internal/ota/... 2>&1
```
Expected: `cannot find package "github.com/karthangar/matteresp32hub/internal/ota"`

- [ ] **Step 3: Create `internal/ota/auth.go`**

```go
package ota

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/base64"
    "fmt"
    "net/http"
    "strconv"
    "strings"
    "time"

    "github.com/karthangar/matteresp32hub/internal/db"
)

const timestampTolerance = 5 * time.Minute

func signatureMessage(method, path, ts string) string {
    return strings.ToUpper(method) + ":" + path + ":" + ts
}

// authenticate verifies X-Device-ID / X-Timestamp / X-HMAC headers.
// Returns the authenticated device on success, error on failure.
func authenticate(r *http.Request, database *db.Database) (db.Device, error) {
    deviceID := r.Header.Get("X-Device-ID")
    ts := r.Header.Get("X-Timestamp")
    mac := r.Header.Get("X-HMAC")

    if deviceID == "" || ts == "" || mac == "" {
        return db.Device{}, fmt.Errorf("missing auth headers")
    }

    tsUnix, err := strconv.ParseInt(ts, 10, 64)
    if err != nil {
        return db.Device{}, fmt.Errorf("invalid X-Timestamp")
    }
    age := time.Since(time.Unix(tsUnix, 0))
    if age < 0 {
        age = -age
    }
    if age > timestampTolerance {
        return db.Device{}, fmt.Errorf("timestamp out of window")
    }

    dev, err := database.GetDevice(deviceID)
    if err != nil {
        return db.Device{}, fmt.Errorf("unknown device")
    }

    h := hmac.New(sha256.New, dev.PSK)
    h.Write([]byte(signatureMessage(r.Method, r.URL.Path, ts)))
    expected := base64.StdEncoding.EncodeToString(h.Sum(nil))

    if !hmac.Equal([]byte(expected), []byte(mac)) {
        return db.Device{}, fmt.Errorf("HMAC mismatch")
    }

    return dev, nil
}
```

- [ ] **Step 4: Run auth tests — still fail (NewMux missing)**

```bash
/usr/local/go/bin/go test ./internal/ota/... 2>&1
```
Expected: compile error `undefined: ota.NewMux`

---

## Task 3: OTA handlers

**Files:**
- Create: `internal/ota/handler.go`

- [ ] **Step 1: Create `internal/ota/handler.go`**

```go
package ota

import (
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "path/filepath"

    "github.com/go-chi/chi/v5"
    "github.com/karthangar/matteresp32hub/internal/db"
)

type checkResponse struct {
    LatestVersion   string `json:"latest_version"`
    UpdateAvailable bool   `json:"update_available"`
}

// NewMux returns an http.Handler for the OTA endpoints.
// firmwareDir is the directory where firmware .bin files are stored (e.g. /data/firmware).
func NewMux(database *db.Database, firmwareDir string) http.Handler {
    r := chi.NewRouter()
    r.Use(authMiddleware(database))
    r.Get("/ota/check", handleCheck(database))
    r.Get("/ota/download", handleDownload(database, firmwareDir))
    return r
}

// authMiddleware authenticates every request using PSK HMAC-SHA256.
func authMiddleware(database *db.Database) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if _, err := authenticate(r, database); err != nil {
                http.Error(w, "unauthorized", http.StatusUnauthorized)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}

// handleCheck is GET /ota/check.
// Updates last_seen, ip, fw_version; responds with latest firmware info.
func handleCheck(database *db.Database) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        dev, err := authenticate(r, database)
        if err != nil {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }

        reportedVer := r.Header.Get("X-FW-Version")
        ip := r.RemoteAddr

        if err := database.UpdateDeviceFWVersion(dev.ID, reportedVer, ip); err != nil {
            log.Printf("ota check: update device %s: %v", dev.ID, err)
        }

        latest, err := database.GetLatestFirmware()
        if err != nil {
            // No firmware uploaded yet — still respond OK so device stays online
            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(checkResponse{LatestVersion: "", UpdateAvailable: false})
            return
        }

        resp := checkResponse{
            LatestVersion:   latest.Version,
            UpdateAvailable: reportedVer != latest.Version && latest.Version != "",
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(resp)
    }
}

// handleDownload is GET /ota/download.
// Streams the latest firmware binary and records the event in ota_log.
func handleDownload(database *db.Database, firmwareDir string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        dev, err := authenticate(r, database)
        if err != nil {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }

        latest, err := database.GetLatestFirmware()
        if err != nil {
            http.Error(w, "no firmware available", http.StatusNotFound)
            return
        }

        binPath := filepath.Join(firmwareDir, fmt.Sprintf("%s.bin", latest.Version))
        f, err := os.Open(binPath)
        if err != nil {
            http.Error(w, "firmware file not found", http.StatusNotFound)
            return
        }
        defer f.Close()

        // Log the OTA event before streaming so we have a record even if stream fails
        _ = database.CreateOTALog(db.OTALogRow{
            DeviceID: dev.ID,
            FromVer:  dev.FWVersion,
            ToVer:    latest.Version,
            Result:   "ok",
        })

        w.Header().Set("Content-Type", "application/octet-stream")
        w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.bin"`, latest.Version))
        io.Copy(w, f)
    }
}
```

- [ ] **Step 2: Run all OTA tests**

```bash
/usr/local/go/bin/go test ./internal/ota/... -v 2>&1
```
Expected: all 3 tests PASS.

- [ ] **Step 3: Run full test suite**

```bash
/usr/local/go/bin/go test ./... 2>&1
```
Expected: all packages pass.

- [ ] **Step 4: Commit**

```bash
git add internal/ota/ internal/db/ota_log.go internal/db/device.go
git commit -m "feat: OTA server — PSK HMAC auth, /ota/check, /ota/download"
```

---

## Task 4: Wire OTA server into `ListenAndServeOTA`

**Files:**
- Modify: `internal/api/server.go`

- [ ] **Step 1: Replace placeholder `ListenAndServeOTA` in `internal/api/server.go`**

Replace the entire `ListenAndServeOTA` method:

```go
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
```

Also add the import at the top of `server.go`:
```go
"github.com/karthangar/matteresp32hub/internal/ota"
```

- [ ] **Step 2: Check `s.cfg.DataDir` exists on the Config struct**

```bash
grep -n "DataDir" /home/Karthangar/Projets/MatterESP32Multicontroller/internal/config/*.go
```

If `DataDir` is missing, add it to `internal/config/types.go`:
```go
type Config struct {
    App       App
    WiFi      WiFi
    USB       USB
    PSKPolicy PSKPolicy
    WebPort   int
    OTAPort   int
    DataDir   string  // add this
}
```
And wire it in `Load()`:
```go
return &Config{
    ...
    DataDir: os.Getenv("DATA_DIR"),
}, nil
```

- [ ] **Step 3: Build**

```bash
/usr/local/go/bin/go build ./... 2>&1
```
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/api/server.go internal/config/
git commit -m "feat: wire OTA server — real HMAC handler replaces 501 stub"
```

---

## Task 5: OTA Svelte view — fleet OTA dashboard

**Files:**
- Modify: `web/src/views/OTA.svelte`

- [ ] **Step 1: Add `GET /api/devices` OTA status API endpoint**

In `internal/api/devices.go`, add a handler `otaStatus` (or reuse `listDevices` — it already returns `fw_version`). Also add `GET /api/ota/log/:deviceID` for the log.

Create `internal/api/ota.go`:

```go
package api

import (
    "encoding/json"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/karthangar/matteresp32hub/internal/db"
)

func otaRouter(database *db.Database) func(chi.Router) {
    return func(r chi.Router) {
        r.Get("/log/{deviceID}", func(w http.ResponseWriter, req *http.Request) {
            id := chi.URLParam(req, "deviceID")
            log, err := database.ListOTALogForDevice(id)
            if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }
            if log == nil {
                log = []db.OTALogRow{}
            }
            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(log)
        })
    }
}
```

Register it in `internal/api/router.go`:
```go
r.Route("/api/ota", otaRouter(database))
```

- [ ] **Step 2: Replace `web/src/views/OTA.svelte`**

```svelte
<script>
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../lib/api.js';

  let devices = [];
  let latestFW = null;
  let error = '';
  let loading = true;
  let pushingAll = false;
  let pushResults = {};

  let interval;

  onMount(async () => {
    await load();
    interval = setInterval(load, 15000); // refresh every 15s
  });

  onDestroy(() => clearInterval(interval));

  async function load() {
    try {
      const [devs, fwList] = await Promise.all([
        api.get('/api/devices'),
        api.get('/api/firmware'),
      ]);
      devices = devs || [];
      latestFW = fwList?.find(f => f.is_latest) ?? null;
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function needsUpdate(dev) {
    return latestFW && dev.fw_version && dev.fw_version !== latestFW.version;
  }

  function statusBadge(dev) {
    if (dev.status === 'online') return 'badge-success';
    if (dev.status === 'offline') return 'badge-error';
    return 'badge-ghost';
  }

  function fwBadge(dev) {
    if (!latestFW) return '';
    if (!dev.fw_version) return 'badge-ghost';
    if (dev.fw_version === latestFW.version) return 'badge-success';
    return 'badge-warning';
  }

  const outdated = () => devices.filter(d => needsUpdate(d));
  const upToDate = () => devices.filter(d => latestFW && d.fw_version === latestFW.version);
</script>

<div class="p-6 flex flex-col gap-6 max-w-4xl">
  <div class="flex items-center justify-between flex-wrap gap-2">
    <h2 class="text-lg font-semibold">OTA Updates</h2>
    {#if latestFW}
      <span class="text-sm text-base-content/60">
        Latest firmware: <span class="font-mono font-semibold">{latestFW.version}</span>
      </span>
    {/if}
  </div>

  {#if !latestFW && !loading}
    <div class="alert alert-warning text-sm">
      No firmware marked as latest. Upload and mark a version in the <strong>Firmware</strong> view.
    </div>
  {/if}

  {#if latestFW && outdated().length > 0}
    <div class="alert alert-info text-sm flex justify-between items-center gap-4">
      <span>{outdated().length} device{outdated().length !== 1 ? 's' : ''} running outdated firmware — devices will update automatically on next check-in.</span>
    </div>
  {/if}

  {#if loading}
    <div class="flex justify-center py-12"><span class="loading loading-spinner"></span></div>
  {:else if error}
    <div class="alert alert-error text-sm">{error}</div>
  {:else if devices.length === 0}
    <div class="text-sm text-base-content/50 text-center py-8">No devices registered yet.</div>
  {:else}
    <div class="overflow-x-auto rounded-lg border border-base-200">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>Device</th>
            <th>Status</th>
            <th>Current FW</th>
            <th>Latest FW</th>
            <th>Last Seen</th>
          </tr>
        </thead>
        <tbody>
          {#each devices as d (d.id)}
            <tr class="hover">
              <td>
                <div class="font-mono text-sm font-semibold">{d.name}</div>
                <div class="text-xs text-base-content/40">{d.id}</div>
              </td>
              <td><span class="badge badge-sm {statusBadge(d)}">{d.status}</span></td>
              <td>
                <span class="badge badge-sm {fwBadge(d)} font-mono">
                  {d.fw_version || '—'}
                </span>
              </td>
              <td class="font-mono text-sm text-base-content/60">
                {latestFW ? latestFW.version : '—'}
              </td>
              <td class="text-sm text-base-content/50">
                {d.last_seen ? new Date(d.last_seen).toLocaleString() : '—'}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <div class="flex gap-4 text-sm text-base-content/60">
      <span>✓ Up to date: <strong>{upToDate().length}</strong></span>
      <span>⚠ Outdated: <strong>{outdated().length}</strong></span>
      <span>Total: <strong>{devices.length}</strong></span>
    </div>
    <p class="text-xs text-base-content/40">Devices poll automatically every few minutes. Outdated devices download the latest firmware on next check-in.</p>
  {/if}
</div>
```

- [ ] **Step 3: Commit**

```bash
git add web/src/views/OTA.svelte internal/api/ota.go internal/api/router.go
git commit -m "feat: OTA view — live fleet firmware status dashboard"
```

---

## Task 6: Build and deploy

- [ ] **Step 1: Full test suite**

```bash
/usr/local/go/bin/go test ./... 2>&1
```
Expected: all packages pass.

- [ ] **Step 2: Rebuild Docker image and redeploy**

```bash
docker compose build && docker compose up -d
```

- [ ] **Step 3: Smoke test OTA endpoints**

```bash
# Should return 401 (no auth headers)
curl -sk https://localhost:48061/ota/check
# Expected: unauthorized

# Web UI OTA view loads
curl -s http://localhost:48060/api/devices | python3 -m json.tool | head
```

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "feat: Plan 5 complete — OTA server + fleet firmware dashboard"
```

---

## Self-Review

**Spec coverage:**
- ✅ PSK HMAC-SHA256 auth on every OTA request
- ✅ `GET /ota/check` — heartbeat + update_available response
- ✅ `GET /ota/download` — binary stream
- ✅ Device last_seen / ip / fw_version updated on check-in
- ✅ ota_log written on download
- ✅ OTA UI shows per-device firmware status
- ✅ Port 48061 keeps HTTPS (ESP32 direct, no Traefik)

**Not in scope (Plan 6):**
- Bulk manual OTA push button
- Auto-update policy config
- Flash/OTA history log UI per device
