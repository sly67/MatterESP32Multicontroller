# ESPHome Compile Queue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the ephemeral per-request `docker run esphome` pattern with a persistent sidecar service, an in-process compile queue, SQLite-backed job persistence, SSE-based live monitoring, and full queue management from both the Flash wizard and a new Jobs view.

**Architecture:** Custom ESPHome sidecar (official image + thin Python HTTP wrapper) in the compose stack. Hub owns all queue/job logic in Go. Jobs stored in SQLite with full config JSON and firmware binary path. SSE pushes status and logs to the browser in real-time.

**Tech Stack:** Go (chi, modernc SQLite), Python 3 (stdlib http.server), Svelte 4, DaisyUI, Docker Compose

---

## File Structure

| File | Change |
|------|--------|
| `Dockerfile.esphome` | New — sidecar image |
| `esphome-svc/server.py` | New — Python HTTP compile server |
| `docker-compose.yml` | Add esphome-svc service |
| `internal/db/schema.sql` | Add `esphome_jobs` table |
| `internal/db/db.go` | Add migration for esphome_jobs |
| `internal/db/esphome_jobs.go` | New — CRUD for esphome_jobs |
| `internal/esphome/sidecar.go` | New — HTTP client for sidecar |
| `internal/esphome/queue.go` | New — Queue struct + worker goroutine |
| `internal/esphome/builder.go` | **Delete** — replaced by sidecar+queue |
| `internal/esphome/builder_test.go` | **Delete** — no longer relevant |
| `internal/api/jobs.go` | New — all job API handlers |
| `internal/api/router.go` | Add queue param; register /api/jobs |
| `internal/api/server.go` | Add queue field; update NewServer |
| `internal/api/flash.go` | Replace builder with queue.Enqueue |
| `internal/api/webflash.go` | esphome-prepare → return job ID |
| `internal/api/health_test.go` | Update newTestServer to pass nil queue |
| `cmd/server/main.go` | Init sidecar + queue; pass to NewServer |
| `web/src/views/JobMonitor.svelte` | New — SSE job monitor page |
| `web/src/views/Jobs.svelte` | New — job list table |
| `web/src/App.svelte` | Add Jobs + JobMonitor views |
| `web/src/lib/Sidebar.svelte` | Add Jobs nav entry |
| `web/src/views/Flash.svelte` | ESPHome path → POST /api/jobs → /jobs/{id} |
| `web/src/views/Fleet.svelte` | Per-device latest job badge |

---

## Task 1: ESPHome sidecar — Python server + Dockerfile + compose

**Files:**
- Create: `esphome-svc/server.py`
- Create: `Dockerfile.esphome`
- Modify: `docker-compose.yml`

- [ ] **Step 1: Create `esphome-svc/server.py`**

```python
#!/usr/bin/env python3
import json, os, signal, subprocess, threading
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = 6052
_lock = threading.Lock()
_proc = None

class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args): pass

    def do_GET(self):
        if self.path == '/health':
            self._send(200, b'ok')
        else:
            self._send(404, b'not found')

    def do_DELETE(self):
        global _proc
        if not self.path.startswith('/compile'):
            self._send(404, b'not found'); return
        with _lock:
            p = _proc
        if p and p.poll() is None:
            p.send_signal(signal.SIGTERM)
        self._send(204, b'')

    def do_POST(self):
        global _proc
        if not self.path.startswith('/compile/'):
            self._send(404, b'not found'); return
        device = self.path[len('/compile/'):]
        if not device:
            self._send(400, b'missing device'); return

        with _lock:
            if _proc is not None and _proc.poll() is None:
                self._send(409, b'compile in progress'); return

        cfg_path = f'/config/{device}/config.yaml'
        if not os.path.exists(cfg_path):
            body = json.dumps({'result': 'error', 'message': 'config not found'}).encode()
            self._send(400, body); return

        self.send_response(200)
        self.send_header('Content-Type', 'application/x-ndjson')
        self.send_header('Transfer-Encoding', 'chunked')
        self.end_headers()

        proc = subprocess.Popen(
            ['esphome', 'compile', cfg_path],
            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
        )
        with _lock:
            _proc = proc

        try:
            for line in proc.stdout:
                line = line.rstrip('\n')
                if line:
                    msg = json.dumps({'log': line}) + '\n'
                    self.wfile.write(msg.encode())
                    self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            proc.send_signal(signal.SIGTERM)
        finally:
            proc.wait()
            with _lock:
                _proc = None

        code = proc.returncode
        if code == 0:
            result = json.dumps({'result': 'ok'}) + '\n'
        else:
            result = json.dumps({'result': 'error', 'code': code}) + '\n'
        try:
            self.wfile.write(result.encode())
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass

    def _send(self, code, body=b''):
        self.send_response(code)
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', PORT), Handler)
    print(f'ESPHome sidecar listening on :{PORT}', flush=True)
    server.serve_forever()
```

- [ ] **Step 2: Create `Dockerfile.esphome`**

```dockerfile
FROM ghcr.io/esphome/esphome:latest
COPY esphome-svc/server.py /server.py
ENTRYPOINT ["python3", "/server.py"]
```

- [ ] **Step 3: Add `esphome-svc` service to `docker-compose.yml`**

Append the service after the `matteresp32hub` block. Also add `ESPHOME_SVC_URL` to `matteresp32hub`'s environment and remove the `ESPHOME_CACHE_VOLUME`/`PIO_HOME_VOLUME` env vars (no longer spawning docker run). Remove the `ESPHOME_CACHE_VOLUME` and `PIO_HOME_VOLUME` volumes from matteresp32hub and instead add `esphome-cache` as a shared volume. The full updated `docker-compose.yml`:

```yaml
services:
  matteresp32hub:
    build: .
    image: matteresp32hub:latest
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
      - /dev:/dev
      - /Portainer/MatterESP32/firmware-src:/firmware-src:ro
      - /var/run/docker.sock:/var/run/docker.sock
      - /Portainer/MatterESP32/esphome-cache:/data/esphome-cache
      - /Portainer/MatterESP32/pio-home:/data/pio-home
      - /Portainer/MatterESP32/esphome-builds:/data/esphome-builds
    device_cgroup_rules:
      - 'c 188:* rmw'
      - 'c 166:* rmw'
    environment:
      - DATA_DIR=/data
      - FIRMWARE_SRC_DIR=/firmware-src
      - ESPHOME_SVC_URL=http://esphome-svc:6052

  esphome-svc:
    build:
      context: .
      dockerfile: Dockerfile.esphome
    container_name: esphome-svc
    restart: unless-stopped
    volumes:
      - /Portainer/MatterESP32/esphome-cache:/config
      - /Portainer/MatterESP32/pio-home:/root/.platformio
```

- [ ] **Step 4: Build to verify Dockerfile.esphome parses**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
docker build -f Dockerfile.esphome -t esphome-svc-test . 2>&1 | tail -5
```

Expected: `Successfully built` (or `Successfully tagged`). The build downloads the ESPHome base image (~500MB on first run).

- [ ] **Step 5: Commit**

```bash
git add Dockerfile.esphome esphome-svc/server.py docker-compose.yml
git commit -m "feat: ESPHome sidecar service (Python HTTP + Dockerfile + compose)"
```

---

## Task 2: esphome_jobs DB table + CRUD

**Files:**
- Modify: `internal/db/schema.sql`
- Modify: `internal/db/db.go`
- Create: `internal/db/esphome_jobs.go`
- Modify: `internal/db/db_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/db/db_test.go` (inside `package db_test`, with existing imports; add `"time"` if missing):

```go
func TestESPHomeJob_CRUD(t *testing.T) {
    database, err := db.Open(":memory:")
    require.NoError(t, err)
    defer database.Close()

    job := db.ESPHomeJob{
        ID:         "aabbcc",
        DeviceName: "Hub4",
        ConfigJSON: `{"board":"esp32-c3"}`,
        Status:     "pending",
    }
    require.NoError(t, database.CreateJob(job))

    got, err := database.GetJob("aabbcc")
    require.NoError(t, err)
    assert.Equal(t, "aabbcc", got.ID)
    assert.Equal(t, "Hub4", got.DeviceName)
    assert.Equal(t, "pending", got.Status)

    require.NoError(t, database.UpdateJobStatus("aabbcc", "running", "", ""))
    got, err = database.GetJob("aabbcc")
    require.NoError(t, err)
    assert.Equal(t, "running", got.Status)

    require.NoError(t, database.AppendJobLog("aabbcc", "line1"))
    require.NoError(t, database.AppendJobLog("aabbcc", "line2"))
    got, err = database.GetJob("aabbcc")
    require.NoError(t, err)
    assert.Contains(t, got.Log, "line1")
    assert.Contains(t, got.Log, "line2")

    require.NoError(t, database.UpdateJobDone("aabbcc", "/data/esphome-builds/aabbcc.bin", "dev-1"))
    got, err = database.GetJob("aabbcc")
    require.NoError(t, err)
    assert.Equal(t, "done", got.Status)
    assert.Equal(t, "/data/esphome-builds/aabbcc.bin", got.BinaryPath)
    assert.Equal(t, "dev-1", got.DeviceID)

    list, err := database.ListJobs()
    require.NoError(t, err)
    require.Len(t, list, 1)
}

func TestESPHomeJob_ResetStale(t *testing.T) {
    database, err := db.Open(":memory:")
    require.NoError(t, err)
    defer database.Close()

    for _, s := range []string{"pending", "running", "done", "failed"} {
        require.NoError(t, database.CreateJob(db.ESPHomeJob{
            ID: s, DeviceName: "d", ConfigJSON: "{}", Status: s,
        }))
    }
    require.NoError(t, database.ResetStaleJobs())

    for _, id := range []string{"pending", "running"} {
        got, err := database.GetJob(id)
        require.NoError(t, err)
        assert.Equal(t, "failed", got.Status, "job %s should be failed after reset", id)
    }
    for _, id := range []string{"done", "failed"} {
        got, err := database.GetJob(id)
        require.NoError(t, err)
        assert.Equal(t, id, got.Status, "job %s should be unchanged after reset", id)
    }
}

func TestESPHomeJob_DeleteOld(t *testing.T) {
    database, err := db.Open(":memory:")
    require.NoError(t, err)
    defer database.Close()

    require.NoError(t, database.CreateJob(db.ESPHomeJob{
        ID: "old", DeviceName: "d", ConfigJSON: "{}", Status: "done",
    }))
    // Use a future cutoff to delete the just-created job
    require.NoError(t, database.DeleteOldJobs(time.Now().Add(time.Hour)))
    _, err = database.GetJob("old")
    assert.Error(t, err, "old job should be deleted")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
go test ./internal/db/... -run "TestESPHomeJob" -v 2>&1 | head -20
```

Expected: FAIL — `db.ESPHomeJob` undefined.

- [ ] **Step 3: Add `esphome_jobs` table to `internal/db/schema.sql`**

Append to the end of the file (before the last `CREATE INDEX`):

```sql
CREATE TABLE IF NOT EXISTS esphome_jobs (
    id          TEXT PRIMARY KEY,
    device_id   TEXT,
    device_name TEXT NOT NULL,
    config_json TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',
    log         TEXT NOT NULL DEFAULT '',
    binary_path TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_esphome_jobs_device ON esphome_jobs(device_id);
CREATE INDEX IF NOT EXISTS idx_esphome_jobs_created ON esphome_jobs(created_at DESC);
```

- [ ] **Step 4: Add migration guard in `internal/db/db.go`**

After the existing `ALTER TABLE` loop (after the `if fwTypeCount == 0` block closes), add:

```go
	// esphome_jobs migration: add table if not present on older installs.
	// CREATE TABLE IF NOT EXISTS in schema.sql is idempotent; no extra guard needed.
	// Reset any jobs left in pending/running state from a previous crash.
	sqldb.Exec(`UPDATE esphome_jobs SET status = 'failed', error = 'hub restarted', updated_at = CURRENT_TIMESTAMP WHERE status IN ('pending','running')`) //nolint:errcheck
```

- [ ] **Step 5: Create `internal/db/esphome_jobs.go`**

```go
package db

import (
	"database/sql"
	"time"
)

// ESPHomeJob is a row in the esphome_jobs table.
type ESPHomeJob struct {
	ID         string
	DeviceID   string
	DeviceName string
	ConfigJSON string
	Status     string
	Log        string
	BinaryPath string
	Error      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CreateJob inserts a new job row.
func (d *Database) CreateJob(job ESPHomeJob) error {
	_, err := d.DB.Exec(
		`INSERT INTO esphome_jobs (id, device_name, config_json, status)
		 VALUES (?, ?, ?, ?)`,
		job.ID, job.DeviceName, job.ConfigJSON, job.Status)
	return err
}

// GetJob retrieves a job by ID.
func (d *Database) GetJob(id string) (ESPHomeJob, error) {
	row := d.DB.QueryRow(
		`SELECT id, COALESCE(device_id,''), device_name, config_json, status,
		        log, binary_path, error, created_at, updated_at
		 FROM esphome_jobs WHERE id = ?`, id)
	var j ESPHomeJob
	err := row.Scan(&j.ID, &j.DeviceID, &j.DeviceName, &j.ConfigJSON, &j.Status,
		&j.Log, &j.BinaryPath, &j.Error, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return ESPHomeJob{}, err
	}
	return j, nil
}

// ListJobs returns all jobs ordered by created_at descending.
func (d *Database) ListJobs() ([]ESPHomeJob, error) {
	rows, err := d.DB.Query(
		`SELECT id, COALESCE(device_id,''), device_name, config_json, status,
		        log, binary_path, error, created_at, updated_at
		 FROM esphome_jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []ESPHomeJob
	for rows.Next() {
		var j ESPHomeJob
		if err := rows.Scan(&j.ID, &j.DeviceID, &j.DeviceName, &j.ConfigJSON, &j.Status,
			&j.Log, &j.BinaryPath, &j.Error, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// UpdateJobStatus sets status, optionally binary_path and error.
func (d *Database) UpdateJobStatus(id, status, binaryPath, errMsg string) error {
	_, err := d.DB.Exec(
		`UPDATE esphome_jobs SET status = ?, binary_path = ?, error = ?,
		 updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, binaryPath, errMsg, id)
	return err
}

// UpdateJobDone marks a job done and records the binary path and device_id.
func (d *Database) UpdateJobDone(id, binaryPath, deviceID string) error {
	var devID interface{}
	if deviceID != "" {
		devID = deviceID
	}
	_, err := d.DB.Exec(
		`UPDATE esphome_jobs SET status = 'done', binary_path = ?, device_id = ?,
		 updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		binaryPath, devID, id)
	return err
}

// AppendJobLog appends a log line (newline-separated) to the job's log field.
func (d *Database) AppendJobLog(id, line string) error {
	_, err := d.DB.Exec(
		`UPDATE esphome_jobs SET
		   log = CASE WHEN log = '' THEN ? ELSE log || char(10) || ? END,
		   updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		line, line, id)
	return err
}

// ResetStaleJobs marks pending/running jobs as failed (called on hub startup).
func (d *Database) ResetStaleJobs() error {
	_, err := d.DB.Exec(
		`UPDATE esphome_jobs SET status = 'failed', error = 'hub restarted',
		 updated_at = CURRENT_TIMESTAMP WHERE status IN ('pending','running')`)
	return err
}

// DeleteOldJobs removes done/failed/cancelled jobs created before cutoff.
func (d *Database) DeleteOldJobs(cutoff time.Time) error {
	_, err := d.DB.Exec(
		`DELETE FROM esphome_jobs WHERE status IN ('done','failed','cancelled')
		 AND created_at < ?`, cutoff)
	return err
}

// GetLatestJobForDevice returns the most recent job for a device_id (for Fleet badge).
func (d *Database) GetLatestJobForDevice(deviceID string) (ESPHomeJob, error) {
	row := d.DB.QueryRow(
		`SELECT id, COALESCE(device_id,''), device_name, config_json, status,
		        log, binary_path, error, created_at, updated_at
		 FROM esphome_jobs WHERE device_id = ? ORDER BY created_at DESC LIMIT 1`, deviceID)
	var j ESPHomeJob
	err := row.Scan(&j.ID, &j.DeviceID, &j.DeviceName, &j.ConfigJSON, &j.Status,
		&j.Log, &j.BinaryPath, &j.Error, &j.CreatedAt, &j.UpdatedAt)
	if err == sql.ErrNoRows {
		return ESPHomeJob{}, nil
	}
	return j, err
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go test ./internal/db/... -run "TestESPHomeJob" -v
```

Expected: all 3 PASS.

- [ ] **Step 7: Run all DB tests**

```bash
go test ./internal/db/... -v 2>&1 | tail -15
```

Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/db/schema.sql internal/db/db.go internal/db/esphome_jobs.go internal/db/db_test.go
git commit -m "feat: esphome_jobs SQLite table + CRUD"
```

---

## Task 3: Sidecar HTTP client

**Files:**
- Create: `internal/esphome/sidecar.go`

- [ ] **Step 1: Create `internal/esphome/sidecar.go`**

```go
package esphome

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client calls the ESPHome sidecar HTTP service.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a sidecar client targeting baseURL (e.g. "http://esphome-svc:6052").
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{}, // no global timeout; callers use context
	}
}

// Compile calls POST /compile/{device} on the sidecar, streaming log lines to logWriter.
func (c *Client) Compile(ctx context.Context, device string, logWriter io.Writer) error {
	url := fmt.Sprintf("%s/compile/%s", c.baseURL, device)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sidecar unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("sidecar busy: another compile is running")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sidecar HTTP %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		var msg struct {
			Log    string `json:"log"`
			Result string `json:"result"`
			Code   int    `json:"code"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Log != "" {
			fmt.Fprintln(logWriter, msg.Log)
		}
		if msg.Result == "ok" {
			return nil
		}
		if msg.Result == "error" {
			return fmt.Errorf("esphome compile failed (exit code %d)", msg.Code)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read sidecar response: %w", err)
	}
	return fmt.Errorf("sidecar stream ended without result")
}

// Cancel calls DELETE /compile on the sidecar to SIGTERM the running compile.
func (c *Client) Cancel() error {
	url := fmt.Sprintf("%s/compile", c.baseURL)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// Ping checks if the sidecar is reachable.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check HTTP %d", resp.StatusCode)
	}
	return nil
}
```

- [ ] **Step 2: Build to verify no compile errors**

```bash
go build ./internal/esphome/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/esphome/sidecar.go
git commit -m "feat: ESPHome sidecar HTTP client"
```

---

## Task 4: Compile queue + delete builder

**Files:**
- Create: `internal/esphome/queue.go`
- Delete: `internal/esphome/builder.go`
- Delete: `internal/esphome/builder_test.go`

- [ ] **Step 1: Delete builder files**

```bash
rm internal/esphome/builder.go internal/esphome/builder_test.go
```

- [ ] **Step 2: Create `internal/esphome/queue.go`**

```go
package esphome

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/library"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
)

// JobStatus is the state of a compile job.
type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobDone      JobStatus = "done"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

// JobConfig is stored as config_json and contains everything needed to re-compile.
type JobConfig struct {
	Board         string            `json:"board"`
	DeviceName    string            `json:"device_name"`
	DeviceID      string            `json:"device_id"`
	WiFiSSID      string            `json:"wifi_ssid"`
	WiFiPassword  string            `json:"wifi_password"`
	HAIntegration bool              `json:"ha_integration"`
	APIKey        string            `json:"api_key"`
	OTAPassword   string            `json:"ota_password"`
	Components    []ComponentConfig `json:"components"`
}

// Event is an SSE event broadcast to job subscribers.
type Event struct {
	Type     string `json:"type"`              // "log","status","done","position"
	Data     string `json:"data,omitempty"`    // log line / status / queue position
	OK       bool   `json:"ok,omitempty"`      // for "done"
	ErrorMsg string `json:"error,omitempty"`   // for "done" when !OK
}

type queueJob struct {
	id     string
	config JobConfig
}

// Queue manages ESPHome compile jobs with a single worker goroutine.
type Queue struct {
	mu      sync.Mutex
	pending []*queueJob
	active  *queueJob
	cancel  context.CancelFunc

	subsMu sync.RWMutex
	subs   map[string][]chan Event

	database *db.Database
	sidecar  *Client
	dataDir  string
	notify   chan struct{}
}

// NewQueue creates a Queue and starts the background worker and purge goroutines.
func NewQueue(database *db.Database, sidecar *Client, dataDir string) *Queue {
	q := &Queue{
		subs:     make(map[string][]chan Event),
		database: database,
		sidecar:  sidecar,
		dataDir:  dataDir,
		notify:   make(chan struct{}, 1),
	}
	os.MkdirAll(filepath.Join(dataDir, "esphome-builds"), 0755) //nolint:errcheck
	go q.worker()
	go q.purgeBinaries()
	return q
}

// Enqueue adds a new compile job and returns the job ID.
func (q *Queue) Enqueue(cfg JobConfig) (string, error) {
	id, err := randomHexID(6)
	if err != nil {
		return "", err
	}
	cfgJSON, _ := json.Marshal(cfg)
	if err := q.database.CreateJob(db.ESPHomeJob{
		ID:         id,
		DeviceName: cfg.DeviceName,
		ConfigJSON: string(cfgJSON),
		Status:     string(JobPending),
	}); err != nil {
		return "", fmt.Errorf("persist job: %w", err)
	}
	jb := &queueJob{id: id, config: cfg}
	q.mu.Lock()
	q.pending = append(q.pending, jb)
	pos := len(q.pending)
	q.mu.Unlock()

	q.broadcastPos(id, pos)
	select {
	case q.notify <- struct{}{}:
	default:
	}
	return id, nil
}

// Cancel cancels a pending or running job.
func (q *Queue) Cancel(id string) error {
	q.mu.Lock()
	for i, jb := range q.pending {
		if jb.id == id {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			q.mu.Unlock()
			q.database.UpdateJobStatus(id, string(JobCancelled), "", "cancelled") //nolint:errcheck
			q.broadcast(id, Event{Type: "done", ErrorMsg: "cancelled"})
			q.closeSubscribers(id)
			return nil
		}
	}
	if q.active != nil && q.active.id == id {
		if q.cancel != nil {
			q.cancel()
		}
		q.mu.Unlock()
		return nil
	}
	q.mu.Unlock()
	return fmt.Errorf("job %s not found or already finished", id)
}

// Subscribe returns a read-only event channel and a cleanup func.
// It replays existing log from DB before attaching to live updates.
func (q *Queue) Subscribe(id string) (<-chan Event, func(), error) {
	job, err := q.database.GetJob(id)
	if err != nil {
		return nil, nil, fmt.Errorf("job not found: %w", err)
	}
	ch := make(chan Event, 128)

	// Replay accumulated log
	if job.Log != "" {
		for _, line := range strings.Split(job.Log, "\n") {
			if line != "" {
				ch <- Event{Type: "log", Data: line}
			}
		}
	}
	ch <- Event{Type: "status", Data: job.Status}

	switch JobStatus(job.Status) {
	case JobDone:
		ch <- Event{Type: "done", OK: true}
		close(ch)
		return ch, func() {}, nil
	case JobFailed:
		ch <- Event{Type: "done", ErrorMsg: job.Error}
		close(ch)
		return ch, func() {}, nil
	case JobCancelled:
		ch <- Event{Type: "done", ErrorMsg: "cancelled"}
		close(ch)
		return ch, func() {}, nil
	}

	q.subsMu.Lock()
	q.subs[id] = append(q.subs[id], ch)
	q.subsMu.Unlock()

	cleanup := func() {
		q.subsMu.Lock()
		defer q.subsMu.Unlock()
		list := q.subs[id]
		for i, c := range list {
			if c == ch {
				q.subs[id] = append(list[:i], list[i+1:]...)
				return
			}
		}
	}
	return ch, cleanup, nil
}

func (q *Queue) broadcast(id string, ev Event) {
	q.subsMu.RLock()
	list := make([]chan Event, len(q.subs[id]))
	copy(list, q.subs[id])
	q.subsMu.RUnlock()
	for _, ch := range list {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (q *Queue) broadcastPos(id string, pos int) {
	q.broadcast(id, Event{Type: "position", Data: fmt.Sprintf("%d", pos)})
}

func (q *Queue) closeSubscribers(id string) {
	q.subsMu.Lock()
	list := q.subs[id]
	delete(q.subs, id)
	q.subsMu.Unlock()
	for _, ch := range list {
		close(ch)
	}
}

func (q *Queue) worker() {
	for {
		<-q.notify

		for {
			q.mu.Lock()
			if len(q.pending) == 0 {
				q.mu.Unlock()
				break
			}
			jb := q.pending[0]
			q.pending = q.pending[1:]
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			q.active = jb
			q.cancel = cancel
			q.mu.Unlock()

			q.runJob(ctx, jb)
			cancel()

			q.mu.Lock()
			q.active = nil
			q.cancel = nil
			q.mu.Unlock()
		}
	}
}

// jobLogWriter implements io.Writer to broadcast log lines and persist them.
type jobLogWriter struct {
	jobID string
	queue *Queue
}

func (w *jobLogWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line != "" {
			w.queue.database.AppendJobLog(w.jobID, line) //nolint:errcheck
			w.queue.broadcast(w.jobID, Event{Type: "log", Data: line})
		}
	}
	return len(p), nil
}

func (q *Queue) runJob(ctx context.Context, jb *queueJob) {
	q.database.UpdateJobStatus(jb.id, string(JobRunning), "", "") //nolint:errcheck
	q.broadcast(jb.id, Event{Type: "status", Data: string(JobRunning)})

	cfg := jb.config

	// Assemble YAML
	mods, err := library.LoadModules()
	if err != nil {
		q.failJob(jb, fmt.Errorf("load modules: %w", err))
		return
	}
	modMap := make(map[string]*yamldef.Module, len(mods))
	for _, m := range mods {
		modMap[m.ID] = m
	}
	yamlStr, err := Assemble(Config{
		Board:         cfg.Board,
		DeviceName:    cfg.DeviceName,
		DeviceID:      cfg.DeviceID,
		WiFiSSID:      cfg.WiFiSSID,
		WiFiPassword:  cfg.WiFiPassword,
		HAIntegration: cfg.HAIntegration,
		APIKey:        cfg.APIKey,
		OTAPassword:   cfg.OTAPassword,
		Components:    cfg.Components,
	}, modMap)
	if err != nil {
		q.failJob(jb, fmt.Errorf("assemble YAML: %w", err))
		return
	}

	// Write YAML + SDK header wrappers to shared cache volume
	devSlug := slug(cfg.DeviceName)
	devDir := filepath.Join(q.dataDir, "esphome-cache", devSlug)
	if err := os.MkdirAll(devDir, 0755); err != nil {
		q.failJob(jb, fmt.Errorf("create dev dir: %w", err))
		return
	}
	if err := os.WriteFile(filepath.Join(devDir, "config.yaml"), []byte(yamlStr), 0644); err != nil {
		q.failJob(jb, fmt.Errorf("write config: %w", err))
		return
	}
	for rel, content := range map[string]string{
		"driver/ledc.h": "#pragma once\n#include <driver/ledc.h>\n",
	} {
		wrapPath := filepath.Join(devDir, rel)
		os.MkdirAll(filepath.Dir(wrapPath), 0755) //nolint:errcheck
		os.WriteFile(wrapPath, []byte(content), 0644) //nolint:errcheck
	}

	// Compile via sidecar
	logWriter := &jobLogWriter{jobID: jb.id, queue: q}
	if err := q.sidecar.Compile(ctx, devSlug, logWriter); err != nil {
		if ctx.Err() != nil {
			q.database.UpdateJobStatus(jb.id, string(JobCancelled), "", "cancelled") //nolint:errcheck
			q.broadcast(jb.id, Event{Type: "done", ErrorMsg: "cancelled"})
			q.closeSubscribers(jb.id)
			return
		}
		q.failJob(jb, err)
		return
	}

	// Read compiled binary from shared volume
	binSrc := filepath.Join(q.dataDir, "esphome-cache", devSlug,
		".esphome", "build", devSlug, ".pioenvs", devSlug, "firmware.factory.bin")
	binData, err := os.ReadFile(binSrc)
	if err != nil {
		q.failJob(jb, fmt.Errorf("read firmware binary: %w", err))
		return
	}

	// Save binary to persistent store
	binDest := filepath.Join(q.dataDir, "esphome-builds", jb.id+".bin")
	if err := os.WriteFile(binDest, binData, 0644); err != nil {
		q.failJob(jb, fmt.Errorf("save firmware: %w", err))
		return
	}

	// Register device in DB (idempotent on re-compile)
	espCfgJSON, _ := json.Marshal(struct {
		Board         string            `json:"board"`
		HAIntegration bool              `json:"ha_integration"`
		OTAPassword   string            `json:"ota_password"`
		Components    []ComponentConfig `json:"components"`
	}{cfg.Board, cfg.HAIntegration, cfg.OTAPassword, cfg.Components})
	q.database.CreateDevice(db.Device{ //nolint:errcheck // may already exist on re-compile
		ID:            cfg.DeviceID,
		Name:          cfg.DeviceName,
		FirmwareType:  "esphome",
		ESPHomeConfig: string(espCfgJSON),
		ESPHomeAPIKey: cfg.APIKey,
		PSK:           []byte{},
	})

	q.database.UpdateJobDone(jb.id, binDest, cfg.DeviceID) //nolint:errcheck
	q.broadcast(jb.id, Event{Type: "done", OK: true})
	q.closeSubscribers(jb.id)
}

func (q *Queue) failJob(jb *queueJob, err error) {
	q.database.UpdateJobStatus(jb.id, string(JobFailed), "", err.Error()) //nolint:errcheck
	q.broadcast(jb.id, Event{Type: "done", ErrorMsg: err.Error()})
	q.closeSubscribers(jb.id)
}

func (q *Queue) purgeBinaries() {
	for {
		time.Sleep(24 * time.Hour)
		cutoff := time.Now().Add(-7 * 24 * time.Hour)
		q.database.DeleteOldJobs(cutoff) //nolint:errcheck
		entries, _ := os.ReadDir(filepath.Join(q.dataDir, "esphome-builds"))
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err == nil && info.ModTime().Before(cutoff) {
				os.Remove(filepath.Join(q.dataDir, "esphome-builds", e.Name())) //nolint:errcheck
			}
		}
	}
}

func randomHexID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
```

- [ ] **Step 3: Build to verify no compile errors**

```bash
go build ./internal/esphome/...
```

Expected: no errors. (`builder.go` deleted, replaced by `queue.go` + `sidecar.go`.)

- [ ] **Step 4: Commit**

```bash
git add internal/esphome/sidecar.go internal/esphome/queue.go
git rm internal/esphome/builder.go internal/esphome/builder_test.go
git commit -m "feat: compile queue + sidecar client; delete builder"
```

---

## Task 5: Job API handlers + router update

**Files:**
- Create: `internal/api/jobs.go`
- Modify: `internal/api/router.go`
- Modify: `internal/api/server.go`
- Modify: `internal/api/health_test.go` (update `newTestServer`)

- [ ] **Step 1: Update `internal/api/health_test.go`**

Change `newTestServer` and `newTestServerWithDataDir` to pass `nil` as the queue:

```go
func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() {
		delete(testDBs, t)
		database.Close()
	})
	testDBs[t] = database
	cfg := &config.Config{WebPort: 48060, OTAPort: 48061}
	return api.NewRouter(cfg, database, nil)
}

func newTestServerWithDataDir(t *testing.T, dataDir string) http.Handler {
	t.Helper()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() {
		delete(testDBs, t)
		database.Close()
	})
	testDBs[t] = database
	cfg := &config.Config{WebPort: 48060, OTAPort: 48061, DataDir: dataDir}
	return api.NewRouter(cfg, database, nil)
}
```

- [ ] **Step 2: Update `internal/api/server.go`**

Add `queue` field and update `NewServer` and `ListenAndServeTLS`:

```go
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
```

- [ ] **Step 3: Update `internal/api/router.go`**

```go
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/esphome"
)

func NewRouter(cfg *config.Config, database *db.Database, queue *esphome.Queue) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Get("/api/health", handleHealth)
	r.Route("/api/devices", devicesRouter(database))
	r.Route("/api/templates", templatesRouter(database))
	r.Route("/api/modules", modulesRouter(database))
	r.Route("/api/effects", effectsRouter(database))
	r.Route("/api/firmware", firmwareRouter(database))
	r.Route("/api/build", buildRouter(cfg))
	r.Route("/api/flash", flashRouter(database, queue))
	r.Route("/api/webflash", webflashRouter(cfg, database, queue))
	r.Route("/api/settings", settingsRouter(cfg, database))
	r.Route("/api/ota", otaRouter(database))
	if queue != nil {
		r.Route("/api/jobs", jobsRouter(queue, database))
	}

	r.Handle("/*", staticHandler())
	return r
}
```

- [ ] **Step 4: Write the failing test for jobs API**

Create `internal/api/jobs_test.go`:

```go
package api_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/api"
	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/esphome"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newJobsTestServer creates a test server wired with a real Queue pointing at a
// mock sidecar that immediately returns success.
func newJobsTestServer(t *testing.T) (http.Handler, *db.Database) {
	t.Helper()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	// Mock sidecar: always returns {"result":"ok"}
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
	assert.Equal(t, id, got["id"])
	assert.Equal(t, "TestDevice", got["device_name"])
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
	// 204 (cancelled) or 409 (already running — also acceptable)
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

	// Stream — give queue a moment to process
	// (mock sidecar is synchronous so job may already be done)
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
```

- [ ] **Step 5: Run test to verify it fails**

```bash
go test ./internal/api/... -run "TestJobs" -v 2>&1 | head -30
```

Expected: FAIL — `api.NewRouter` takes 2 args, `jobsRouter` undefined.

- [ ] **Step 6: Create `internal/api/jobs.go`**

```go
package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/esphome"
)

func jobsRouter(queue *esphome.Queue, database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Post("/", createJob(queue))
		r.Get("/", listJobs(database))
		r.Get("/{id}", getJob(database))
		r.Get("/{id}/stream", streamJob(queue))
		r.Delete("/{id}", cancelJob(queue))
		r.Post("/{id}/resubmit", resubmitJob(queue, database))
		r.Get("/{id}/firmware", serveFirmware(database))
	}
}

func createJob(queue *esphome.Queue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Board         string                   `json:"board"`
			DeviceName    string                   `json:"device_name"`
			WiFiSSID      string                   `json:"wifi_ssid"`
			WiFiPassword  string                   `json:"wifi_password"`
			HAIntegration bool                     `json:"ha_integration"`
			Components    []esphome.ComponentConfig `json:"components"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.DeviceName == "" || req.Board == "" {
			http.Error(w, "device_name and board are required", http.StatusBadRequest)
			return
		}

		deviceID, err := randomHex(6)
		if err != nil {
			http.Error(w, "generate device id: "+err.Error(), http.StatusInternalServerError)
			return
		}
		otaBuf := make([]byte, 16)
		rand.Read(otaBuf) //nolint:errcheck
		otaPassword := hex.EncodeToString(otaBuf)

		var apiKey string
		if req.HAIntegration {
			keyBuf := make([]byte, 32)
			rand.Read(keyBuf) //nolint:errcheck
			apiKey = base64.StdEncoding.EncodeToString(keyBuf)
		}

		id, err := queue.Enqueue(esphome.JobConfig{
			Board:         req.Board,
			DeviceName:    req.DeviceName,
			DeviceID:      deviceID,
			WiFiSSID:      req.WiFiSSID,
			WiFiPassword:  req.WiFiPassword,
			HAIntegration: req.HAIntegration,
			APIKey:        apiKey,
			OTAPassword:   otaPassword,
			Components:    req.Components,
		})
		if err != nil {
			http.Error(w, "enqueue: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"id": id}) //nolint:errcheck
	}
}

func listJobs(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobs, err := database.ListJobs()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if jobs == nil {
			jobs = []db.ESPHomeJob{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jobs) //nolint:errcheck
	}
}

func getJob(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		job, err := database.GetJob(id)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(job) //nolint:errcheck
	}
}

func streamJob(queue *esphome.Queue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		ch, cleanup, err := queue.Subscribe(id)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer cleanup()

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		flusher, _ := w.(http.Flusher)

		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return
				}
				data, _ := json.Marshal(ev)
				fmt.Fprintf(w, "data: %s\n\n", data)
				if flusher != nil {
					flusher.Flush()
				}
				if ev.Type == "done" {
					return
				}
			case <-r.Context().Done():
				return
			}
		}
	}
}

func cancelJob(queue *esphome.Queue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := queue.Cancel(id); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func resubmitJob(queue *esphome.Queue, database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		job, err := database.GetJob(id)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var cfg esphome.JobConfig
		if err := json.Unmarshal([]byte(job.ConfigJSON), &cfg); err != nil {
			http.Error(w, "corrupt config_json: "+err.Error(), http.StatusInternalServerError)
			return
		}
		newID, err := queue.Enqueue(cfg)
		if err != nil {
			http.Error(w, "enqueue: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"id": newID}) //nolint:errcheck
	}
}

func serveFirmware(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		job, err := database.GetJob(id)
		if err != nil || job.BinaryPath == "" {
			http.Error(w, "firmware not available", http.StatusNotFound)
			return
		}
		f, err := os.Open(job.BinaryPath)
		if err != nil {
			http.Error(w, "firmware not available", http.StatusNotFound)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="firmware-factory.bin"`)
		io.Copy(w, f) //nolint:errcheck
	}
}

// randomHex is defined in webflash.go (same package). No duplicate here.
```

**Important:** The `encodeBase64` helper above is a manual implementation to avoid a `encoding/base64` import collision. Actually, just import `"encoding/base64"` and use `base64.StdEncoding.EncodeToString(keyBuf)`. Replace the `encodeBase64` helper and its call site in `createJob` with:

```go
import "encoding/base64"
// ...
apiKey = base64.StdEncoding.EncodeToString(keyBuf)
```

Remove the `encodeBase64` function and the broken `import_b64` closure. The final `createJob` function (HA integration block) should read:

```go
		var apiKey string
		if req.HAIntegration {
			keyBuf := make([]byte, 32)
			rand.Read(keyBuf) //nolint:errcheck
			apiKey = base64.StdEncoding.EncodeToString(keyBuf)
		}
```

The full import block for `internal/api/jobs.go`:

```go
import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/esphome"
)
```

And remove the `encodeBase64` function entirely.

- [ ] **Step 7: Build to verify no compile errors**

```bash
go build ./internal/api/...
```

Expected: no errors.

- [ ] **Step 8: Run jobs tests**

```bash
go test ./internal/api/... -run "TestJobs" -v
```

Expected: all PASS. (The mock sidecar never actually compiles, so the binary won't exist; `TestJobs_StreamReturnsEvents` may see a `failed` status from the binary-read step — that's acceptable as the SSE stream still emits events.)

- [ ] **Step 9: Run all API tests**

```bash
go test ./internal/api/... -v 2>&1 | tail -20
```

Expected: all PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/api/jobs.go internal/api/router.go internal/api/server.go internal/api/health_test.go internal/api/jobs_test.go
git commit -m "feat: /api/jobs handlers + queue wired into router"
```

---

## Task 6: Wire queue into main.go + update flash/webflash

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `internal/api/flash.go`
- Modify: `internal/api/webflash.go`

- [ ] **Step 1: Update `cmd/server/main.go`**

```go
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
	// Stale job reset (pending/running → failed) happens automatically in db.Open.
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
```

- [ ] **Step 2: Update `internal/api/flash.go`**

The `flashRouter` needs a `queue` parameter and `runESPHomeFlash` should enqueue a job and return a job ID instead of streaming compile logs. Change:

```go
func flashRouter(database *db.Database, queue *esphome.Queue) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/ports", listPorts)
		r.Post("/run", runFlash(database))
		r.Post("/esphome", runESPHomeFlash(database, queue))
	}
}
```

Replace the entire `runESPHomeFlash` function:

```go
func runESPHomeFlash(database *db.Database, queue *esphome.Queue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			DeviceName    string                    `json:"device_name"`
			WiFiSSID      string                    `json:"wifi_ssid"`
			WiFiPassword  string                    `json:"wifi_password"`
			Board         string                    `json:"board"`
			HAIntegration bool                      `json:"ha_integration"`
			Components    []esphome.ComponentConfig  `json:"components"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.DeviceName == "" || req.Board == "" {
			http.Error(w, "device_name and board are required", http.StatusBadRequest)
			return
		}

		// randomHex is defined in webflash.go (same package api)
		deviceID, _ := randomHex(6)
		otaBuf := make([]byte, 16)
		rand.Read(otaBuf) //nolint:errcheck
		otaPassword := hex.EncodeToString(otaBuf)

		var apiKey string
		if req.HAIntegration {
			keyBuf := make([]byte, 32)
			rand.Read(keyBuf) //nolint:errcheck
			apiKey = base64.StdEncoding.EncodeToString(keyBuf)
		}

		id, err := queue.Enqueue(esphome.JobConfig{
			Board:         req.Board,
			DeviceName:    req.DeviceName,
			DeviceID:      deviceID,
			WiFiSSID:      req.WiFiSSID,
			WiFiPassword:  req.WiFiPassword,
			HAIntegration: req.HAIntegration,
			APIKey:        apiKey,
			OTAPassword:   otaPassword,
			Components:    req.Components,
		})
		if err != nil {
			http.Error(w, "enqueue: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"id": id}) //nolint:errcheck
	}
}
```

Update the imports in `flash.go` — remove unused imports (`bufio`, `io`, `time`, `library`, `yamldef`, `flash` package) and add what's needed:

```go
import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/esphome"
	"github.com/karthangar/matteresp32hub/internal/flash"
	"github.com/karthangar/matteresp32hub/internal/usb"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
)
```

(Keep `flash`, `usb`, `yamldef` for `runFlash` which still uses `flash.FlashDevice`.)

- [ ] **Step 3: Update `internal/api/webflash.go`**

Change `webflashRouter` signature to accept `queue`:

```go
func webflashRouter(cfg *config.Config, database *db.Database, queue *esphome.Queue) func(chi.Router) {
```

Replace `prepareWebFlashESPHome` entirely. The new handler enqueues a job and returns `{id}` immediately (no streaming):

```go
func prepareWebFlashESPHome(database *db.Database, queue *esphome.Queue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Board         string                    `json:"board"`
			Components    []esphome.ComponentConfig  `json:"components"`
			DeviceName    string                    `json:"device_name"`
			WiFiSSID      string                    `json:"wifi_ssid"`
			WiFiPassword  string                    `json:"wifi_password"`
			HAIntegration bool                      `json:"ha_integration"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.DeviceName == "" || req.Board == "" {
			http.Error(w, "device_name and board are required", http.StatusBadRequest)
			return
		}

		deviceID, err := randomHex(6)
		if err != nil {
			http.Error(w, "device id: "+err.Error(), http.StatusInternalServerError)
			return
		}
		otaBuf := make([]byte, 16)
		rand.Read(otaBuf) //nolint:errcheck
		otaPassword := hex.EncodeToString(otaBuf)

		var apiKey string
		if req.HAIntegration {
			keyBuf := make([]byte, 32)
			rand.Read(keyBuf) //nolint:errcheck
			apiKey = base64.StdEncoding.EncodeToString(keyBuf)
		}

		id, err := queue.Enqueue(esphome.JobConfig{
			Board:         req.Board,
			DeviceName:    req.DeviceName,
			DeviceID:      deviceID,
			WiFiSSID:      req.WiFiSSID,
			WiFiPassword:  req.WiFiPassword,
			HAIntegration: req.HAIntegration,
			APIKey:        apiKey,
			OTAPassword:   otaPassword,
			Components:    req.Components,
		})
		if err != nil {
			http.Error(w, "enqueue: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"id": id}) //nolint:errcheck
	}
}
```

Update the route registration inside `webflashRouter` to call the new signature:

```go
r.Post("/esphome-prepare", prepareWebFlashESPHome(database, queue))
```

Remove unused imports from `webflash.go` (`bufio`, `io`, `time`, `strings`, `library`, `yamldef`). Keep `base64`, `rand`, `hex`, `encoding/json`, `fmt`, `net/http`, `os`, `path/filepath`, `regexp`, `sync`, `chi`, `godata`, `config`, `db`, `esphome`, `matter`, `nvs`.

Also remove: `serveWebFlashESPHomeManifest`, `serveWebFlashESPHomeFirmware` — these routes served the binary from in-memory session, now replaced by `/api/jobs/{id}/firmware`. Remove the route registrations for those endpoints too.

- [ ] **Step 4: Build to verify**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Run all tests**

```bash
go test ./... 2>&1 | tail -20
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/server/main.go internal/api/flash.go internal/api/webflash.go
git commit -m "feat: wire queue into server; flash/webflash use queue.Enqueue"
```

---

## Task 7: Frontend — JobMonitor.svelte + Jobs.svelte + routing

**Files:**
- Create: `web/src/views/JobMonitor.svelte`
- Create: `web/src/views/Jobs.svelte`
- Modify: `web/src/App.svelte`
- Modify: `web/src/lib/Sidebar.svelte`

- [ ] **Step 1: Create `web/src/views/JobMonitor.svelte`**

```svelte
<script>
  import { onMount, onDestroy } from 'svelte';

  export let jobId = ''; // passed from App.svelte via routing

  let job = null;
  let logs = [];
  let status = '';
  let position = null;
  let error = '';
  let loading = true;
  let es = null;

  onMount(async () => {
    try {
      const res = await fetch(`/api/jobs/${jobId}`);
      if (!res.ok) throw new Error(await res.text());
      job = await res.json();
      status = job.status;
      if (job.log) logs = job.log.split('\n').filter(Boolean);
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }

    if (status !== 'done' && status !== 'failed' && status !== 'cancelled') {
      connectSSE();
    }
  });

  onDestroy(() => { if (es) es.close(); });

  function connectSSE() {
    es = new EventSource(`/api/jobs/${jobId}/stream`);
    es.onmessage = (e) => {
      const ev = JSON.parse(e.data);
      if (ev.type === 'log') {
        logs = [...logs, ev.data];
      } else if (ev.type === 'status') {
        status = ev.data;
      } else if (ev.type === 'position') {
        position = ev.data;
      } else if (ev.type === 'done') {
        status = ev.ok ? 'done' : 'failed';
        error = ev.error || '';
        es.close();
        // Refresh job to get binary_path
        fetch(`/api/jobs/${jobId}`).then(r => r.json()).then(j => { job = j; });
      }
    };
    es.onerror = () => {
      // EventSource auto-reconnects; nothing to do
    };
  }

  function recompile() {
    fetch(`/api/jobs/${jobId}/resubmit`, { method: 'POST' })
      .then(r => r.json())
      .then(d => { window.location.hash = '#/jobs/' + d.id; window.location.reload(); });
  }

  const statusClass = {
    pending:   'badge-warning',
    running:   'badge-info',
    done:      'badge-success',
    failed:    'badge-error',
    cancelled: 'badge-ghost',
  };
</script>

<div class="p-6 max-w-3xl mx-auto">
  {#if loading}
    <span class="loading loading-spinner loading-lg"></span>
  {:else if error && !job}
    <div class="alert alert-error">{error}</div>
  {:else}
    <div class="flex items-center gap-3 mb-4">
      <h2 class="text-lg font-semibold">{job?.device_name || jobId}</h2>
      <span class="badge {statusClass[status] || 'badge-ghost'}">{status}</span>
    </div>

    {#if status === 'pending'}
      <div class="alert alert-warning mb-4">
        <span class="loading loading-spinner loading-sm"></span>
        {#if position}Queued — position {position}{:else}Waiting in queue…{/if}
        <button class="btn btn-sm btn-ghost ml-auto"
          on:click={() => fetch(`/api/jobs/${jobId}`, { method: 'DELETE' }).then(() => { status = 'cancelled'; })}>
          Cancel
        </button>
      </div>
    {/if}

    {#if status === 'running'}
      <div class="flex items-center gap-2 mb-2 text-sm text-info">
        <span class="loading loading-spinner loading-xs"></span> Compiling…
        <button class="btn btn-xs btn-ghost ml-auto"
          on:click={() => fetch(`/api/jobs/${jobId}`, { method: 'DELETE' })}>
          Cancel
        </button>
      </div>
    {/if}

    {#if status === 'done'}
      <div class="alert alert-success mb-4">
        Firmware compiled successfully.
        {#if job?.binary_path}
          <a href={`/api/jobs/${jobId}/firmware`}
             class="btn btn-sm btn-outline ml-auto" download>Download .bin</a>
        {/if}
        <button class="btn btn-sm btn-outline" on:click={recompile}>Re-compile</button>
      </div>
    {/if}

    {#if status === 'failed'}
      <div class="alert alert-error mb-4">
        Compile failed: {error || job?.error || 'unknown error'}
        <button class="btn btn-sm btn-outline ml-auto" on:click={recompile}>Re-compile</button>
      </div>
    {/if}

    {#if status === 'cancelled'}
      <div class="alert mb-4">
        Cancelled.
        <button class="btn btn-sm btn-outline ml-auto" on:click={recompile}>Re-compile</button>
      </div>
    {/if}

    {#if logs.length > 0}
      <div class="bg-base-300 rounded-lg p-3 font-mono text-xs overflow-y-auto max-h-96 space-y-0.5">
        {#each logs as line}
          <div class="whitespace-pre-wrap break-all">{line}</div>
        {/each}
      </div>
    {/if}
  {/if}
</div>
```

- [ ] **Step 2: Create `web/src/views/Jobs.svelte`**

```svelte
<script>
  import { onMount } from 'svelte';

  let jobs = [];
  let error = '';
  let loading = true;

  onMount(async () => {
    try {
      const res = await fetch('/api/jobs');
      if (!res.ok) throw new Error(await res.text());
      jobs = await res.json();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  const statusClass = {
    pending:   'badge-warning',
    running:   'badge-info',
    done:      'badge-success',
    failed:    'badge-error',
    cancelled: 'badge-ghost',
  };

  function openJob(id) {
    window.dispatchEvent(new CustomEvent('navigate', { detail: { view: 'jobmonitor', jobId: id } }));
  }

  async function cancelJob(id, e) {
    e.stopPropagation();
    await fetch(`/api/jobs/${id}`, { method: 'DELETE' });
    jobs = jobs.map(j => j.id === id ? { ...j, status: 'cancelled' } : j);
  }

  async function resubmit(id, e) {
    e.stopPropagation();
    const res = await fetch(`/api/jobs/${id}/resubmit`, { method: 'POST' });
    const d = await res.json();
    window.dispatchEvent(new CustomEvent('navigate', { detail: { view: 'jobmonitor', jobId: d.id } }));
  }
</script>

<div class="p-6">
  <h2 class="text-lg font-semibold mb-4">ESPHome Compile Jobs</h2>

  {#if loading}
    <span class="loading loading-spinner loading-lg"></span>
  {:else if error}
    <div class="alert alert-error">{error}</div>
  {:else if jobs.length === 0}
    <div class="text-base-content/50 text-sm">No compile jobs yet.</div>
  {:else}
    <div class="overflow-x-auto">
      <table class="table table-sm w-full">
        <thead>
          <tr>
            <th>Device</th>
            <th>Status</th>
            <th>Created</th>
            <th class="text-right">Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each jobs as j}
            <tr class="hover cursor-pointer" on:click={() => openJob(j.id)}>
              <td class="font-medium">{j.device_name}</td>
              <td>
                <span class="badge badge-sm {statusClass[j.status] || 'badge-ghost'}">
                  {#if j.status === 'running'}<span class="loading loading-spinner loading-xs mr-1"></span>{/if}
                  {j.status}
                </span>
              </td>
              <td class="text-xs text-base-content/50">
                {new Date(j.created_at).toLocaleString()}
              </td>
              <td class="text-right space-x-1">
                {#if j.status === 'pending' || j.status === 'running'}
                  <button class="btn btn-xs btn-ghost" on:click={(e) => cancelJob(j.id, e)}>Cancel</button>
                {/if}
                <button class="btn btn-xs btn-ghost" on:click={(e) => resubmit(j.id, e)}>Re-compile</button>
                {#if j.status === 'done' && j.binary_path}
                  <a class="btn btn-xs btn-outline" href={`/api/jobs/${j.id}/firmware`}
                     download on:click={(e) => e.stopPropagation()}>Download</a>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
```

- [ ] **Step 3: Update `web/src/App.svelte`**

```svelte
<script>
  import Sidebar from './lib/Sidebar.svelte';
  import Fleet       from './views/Fleet.svelte';
  import Flash       from './views/Flash.svelte';
  import Templates   from './views/Templates.svelte';
  import Modules     from './views/Modules.svelte';
  import Effects     from './views/Effects.svelte';
  import OTA         from './views/OTA.svelte';
  import Firmware    from './views/Firmware.svelte';
  import Settings    from './views/Settings.svelte';
  import Jobs        from './views/Jobs.svelte';
  import JobMonitor  from './views/JobMonitor.svelte';

  let current = 'fleet';
  let jobMonitorId = '';

  // Allow sub-components to navigate to JobMonitor via custom event
  window.addEventListener('navigate', (e) => {
    const { view, jobId } = e.detail;
    if (view === 'jobmonitor' && jobId) {
      jobMonitorId = jobId;
      current = 'jobmonitor';
    } else {
      current = view;
    }
  });

  const plainViews = { fleet: Fleet, flash: Flash, templates: Templates, modules: Modules,
    effects: Effects, ota: OTA, firmware: Firmware, settings: Settings, jobs: Jobs };
  $: isJobMonitor = current === 'jobmonitor';
  $: ViewComponent = plainViews[current];
</script>

<div class="flex h-screen w-screen overflow-hidden bg-base-100 text-base-content">
  <Sidebar bind:current />
  <main class="flex-1 flex flex-col overflow-hidden">
    <div class="navbar bg-base-200 border-b border-base-200 px-4 min-h-12 flex-shrink-0">
      <span class="font-semibold text-sm capitalize">
        {isJobMonitor ? 'Job Monitor' : current}
      </span>
    </div>
    <div class="flex-1 overflow-y-auto">
      {#if isJobMonitor}
        <JobMonitor jobId={jobMonitorId} />
      {:else if ViewComponent}
        <svelte:component this={ViewComponent} />
      {/if}
    </div>
  </main>
</div>
```

- [ ] **Step 4: Update `web/src/lib/Sidebar.svelte`**

Add "Jobs" entry to the nav array, in the "Overview" section after Flash:

```js
  const nav = [
    { section: 'Overview' },
    { id: 'fleet',     label: 'Fleet',          icon: '⊞' },
    { id: 'flash',     label: 'Flash Devices',  icon: '⚡' },
    { id: 'jobs',      label: 'Compile Jobs',   icon: '⚙' },
    { section: 'Configuration' },
    { id: 'templates', label: 'Templates',      icon: '⚙' },
    { id: 'modules',   label: 'Module Library', icon: '⬡' },
    { id: 'effects',   label: 'Effects',        icon: '✦' },
    { section: 'Updates' },
    { id: 'ota',       label: 'OTA Updates',    icon: '⇅' },
    { id: 'firmware',  label: 'Firmware',       icon: '💾' },
    { section: 'System' },
    { id: 'settings',  label: 'Settings',       icon: '⚙' },
  ];
```

- [ ] **Step 5: Build frontend**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller/web
npm run build 2>&1 | tail -20
```

Expected: clean build, no errors.

- [ ] **Step 6: Build Go embed**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
go build ./...
```

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add web/src/views/JobMonitor.svelte web/src/views/Jobs.svelte web/src/App.svelte web/src/lib/Sidebar.svelte web/dist/
git commit -m "feat: JobMonitor + Jobs views + routing"
```

---

## Task 8: Frontend — Flash.svelte ESPHome path + Fleet badge

**Files:**
- Modify: `web/src/views/Flash.svelte`
- Modify: `web/src/views/Fleet.svelte`

- [ ] **Step 1: Read Flash.svelte to locate the ESPHome compile section**

Read the file at `/home/Karthangar/Projets/MatterESP32Multicontroller/web/src/views/Flash.svelte` to find `bfEspDoCompile` and the compile step template (step 4 of the ESPHome wizard).

- [ ] **Step 2: Update `bfEspDoCompile` in Flash.svelte**

Replace the existing `bfEspDoCompile` function (which calls `esphome-prepare` and streams ndjson) with one that calls `POST /api/jobs` and navigates to the job monitor:

```js
  async function bfEspDoCompile() {
    bfEspError = '';
    bfEspCompiling = true;
    try {
      const res = await fetch('/api/jobs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          board:          bfEspBoard,
          components:     bfEspComponents.map(c => ({ type: c.type, name: c.name, pins: c.pins })),
          device_name:    bfEspDeviceName,
          wifi_ssid:      bfEspWifiSSID,
          wifi_password:  bfEspWifiPassword,
          ha_integration: bfEspHA,
        }),
      });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      // Navigate to job monitor
      window.dispatchEvent(new CustomEvent('navigate', {
        detail: { view: 'jobmonitor', jobId: data.id }
      }));
    } catch (e) {
      bfEspError = e.message;
    } finally {
      bfEspCompiling = false;
    }
  }
```

- [ ] **Step 3: Simplify the ESPHome compile step template**

The old step 4 in the ESPHome wizard showed a log stream box and a cancel button. Replace it with a simpler "submitting" indicator, since the actual monitoring happens in JobMonitor:

Find the ESPHome step 4 block (where `bfEspStep === 4`) and replace its content with:

```svelte
{#if bfEspStep === 4}
  <div class="flex flex-col items-center gap-4 py-8">
    {#if bfEspCompiling}
      <span class="loading loading-spinner loading-lg"></span>
      <span class="text-sm">Submitting compile job…</span>
    {:else if bfEspError}
      <div class="alert alert-error w-full">{bfEspError}</div>
      <button class="btn btn-primary" on:click={bfEspDoCompile}>Retry</button>
    {/if}
  </div>
{/if}
```

Also update the "Compile" / "Next" button in the step 3 → 4 transition to call `bfEspDoCompile` and advance to step 4:

```svelte
<button class="btn btn-primary" on:click={() => { bfEspStep = 4; bfEspDoCompile(); }}>
  Compile
</button>
```

- [ ] **Step 4: Add job badge to Fleet.svelte**

Read `/home/Karthangar/Projets/MatterESP32Multicontroller/web/src/views/Fleet.svelte` to find the device row template.

Add a `latestJobs` store (fetched once on mount, keyed by device_id):

```js
  let latestJobs = {}; // deviceId → { id, status }

  onMount(async () => {
    try {
      devices = await api.get('/api/devices');
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
    // Load latest job per esphome device
    try {
      const jobs = await api.get('/api/jobs');
      const seen = new Set();
      for (const j of jobs) {
        if (j.device_id && !seen.has(j.device_id)) {
          seen.add(j.device_id);
          latestJobs[j.device_id] = j;
        }
      }
      latestJobs = { ...latestJobs };
    } catch (_) {}
  });
```

In the device row template, after the status badge (or wherever actions/badges are shown), add:

```svelte
{#if latestJobs[d.id]}
  {@const lj = latestJobs[d.id]}
  <button class="badge badge-sm {jobBadgeClass(lj.status)} cursor-pointer"
    on:click={() => window.dispatchEvent(new CustomEvent('navigate', { detail: { view: 'jobmonitor', jobId: lj.id } }))}>
    ESPHome: {lj.status}
  </button>
{/if}
```

Add the `jobBadgeClass` helper with the other helpers:

```js
  const jobBadgeClass = s => ({
    pending:   'badge-warning',
    running:   'badge-info',
    done:      'badge-success',
    failed:    'badge-error',
    cancelled: 'badge-ghost',
  }[s] || 'badge-ghost');
```

- [ ] **Step 5: Build frontend**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller/web
npm run build 2>&1 | tail -20
```

Expected: clean build.

- [ ] **Step 6: Build Go embed**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
go build ./...
```

Expected: clean.

- [ ] **Step 7: Run all tests**

```bash
go test ./... 2>&1 | tail -20
```

Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add web/src/views/Flash.svelte web/src/views/Fleet.svelte web/dist/
git commit -m "feat: Flash wizard + Fleet badge use compile queue"
```

---

## Verification Checklist

1. `go test ./...` — all green
2. `cd web && npm run build` — no errors
3. `go build ./...` — no errors
4. Docker compose: `docker compose up --build` — both `matteresp32hub` and `esphome-svc` start
5. `curl http://localhost:6052/health` from inside the esphome-svc container — returns `ok`
6. Navigate to "Compile Jobs" sidebar entry — shows empty jobs table
7. Flash wizard → ESPHome tab → fill device name/board → click Compile → navigates to JobMonitor page showing live compile log
8. JobMonitor shows "done" when complete, with Download button
9. Fleet view shows ESPHome job badge for devices that have had a compile job
