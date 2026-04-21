# ESPHome Compile Queue Design

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the ephemeral per-request `docker run esphome` pattern with a persistent sidecar service, an in-process compile queue, SQLite-backed job persistence, SSE-based live monitoring, and full queue management (cancel, re-compile, re-flash) from both the Flash wizard and the Fleet view.

**Architecture:** Custom ESPHome sidecar (official image + thin Python HTTP wrapper) in the compose stack. Hub owns all queue/job logic in Go. Jobs stored in SQLite with full config JSON and firmware binary path. SSE pushes status and logs to the browser in real-time.

**Tech Stack:** Go (chi, modernc SQLite), Python 3 (stdlib http.server), Svelte 4, DaisyUI, Docker Compose

---

## Architecture

```
docker-compose stack
┌─────────────────────────────────────────────────────┐
│  matteresp32hub (Go)                                │
│  ┌─────────────┐   HTTP POST /compile/{device}      │
│  │ Queue worker│──────────────────────────────────► │
│  │ (goroutine) │◄── ndjson log stream               │
│  └──────┬──────┘                      esphome-svc   │
│         │ broadcasts                 ┌────────────┐ │
│  ┌──────▼──────┐  shared volumes     │ Python     │ │
│  │ SSE fanout  │  /config (r/w)      │ HTTP wrap  │ │
│  │ per job     │  /pio-home          │ port 6052  │ │
│  └──────┬──────┘                    └────────────┘ │
│  ┌──────▼──────┐                                    │
│  │ SQLite jobs │  data/esphome-builds/{id}.bin       │
│  └─────────────┘                                    │
└─────────────────────────────────────────────────────┘
         │ SSE /api/jobs/{id}/stream
         ▼
    Browser (Jobs page + Fleet badges)
```

- `esphome-svc` is always running (`restart: unless-stopped`). No containers are spawned per compile.
- The sidecar exposes port 6052 on the **internal Docker network only** — no host port.
- The hub writes config YAML + SDK header wrappers to the shared volume, then calls `POST /compile/{device}` on the sidecar and reads the ndjson log stream.
- The Docker socket is no longer needed for the compile path.
- On hub restart, any jobs left in `pending` or `running` state are reset to `failed`.

---

## ESPHome Sidecar (`Dockerfile.esphome` + `esphome-svc/server.py`)

### `Dockerfile.esphome`
```dockerfile
FROM ghcr.io/esphome/esphome:latest
COPY esphome-svc/server.py /server.py
ENTRYPOINT ["python3", "/server.py"]
```

### `esphome-svc/server.py` — HTTP compile server (~80 lines)

Endpoints:
- `GET /health` → `200 ok`
- `POST /compile/{device}` → streams ndjson log lines, final line is `{"result":"ok"}` or `{"result":"error","code":<int>}`
- `DELETE /compile` → SIGTERM the running compile process (cancellation)

Behaviour:
- One compile at a time. Returns `409` if a compile is already running.
- Streams stdout+stderr of `esphome compile /config/{device}/config.yaml` line by line as ndjson.
- On process exit, sends the final result line and closes the response.
- On `DELETE /compile`, sends SIGTERM to the subprocess; the streaming response ends with `{"result":"error","code":-1}`.

### `docker-compose.yml` additions
```yaml
  esphome-svc:
    build:
      context: .
      dockerfile: Dockerfile.esphome
    container_name: esphome-svc
    restart: unless-stopped
    volumes:
      - /Portainer/MatterESP32/esphome-cache:/config
      - /Portainer/MatterESP32/pio-home:/root/.platformio
    environment:
      - ESPHOME_CACHE_VOLUME=/Portainer/MatterESP32/esphome-cache
```

The hub service gains `ESPHOME_SVC_URL=http://esphome-svc:6052` and loses `ESPHOME_CACHE_VOLUME` / `PIO_HOME_VOLUME` (no longer spawning docker run).

---

## Job Data Model

### SQLite table `esphome_jobs`

| column | type | notes |
|---|---|---|
| `id` | TEXT PK | 6-byte random hex |
| `device_id` | TEXT nullable | FK → `devices.id` — set when flashing a known fleet device |
| `device_name` | TEXT | display name, e.g. "Hub4" |
| `config_json` | TEXT | full `ESPHomeRequest` serialised to JSON — enables re-submit |
| `status` | TEXT | `pending` / `running` / `done` / `failed` / `cancelled` |
| `log` | TEXT | accumulated compile log (for catch-up on SSE reconnect) |
| `binary_path` | TEXT nullable | `data/esphome-builds/{id}.bin` — set on success |
| `error` | TEXT nullable | error message on failure |
| `created_at` | DATETIME | |
| `updated_at` | DATETIME | |

### State machine
```
pending ──► running ──► done
                    ╲─► failed
pending ──► cancelled
running ──► cancelled  (context cancel → SIGTERM to sidecar)
```

On **hub restart**: any `pending` or `running` rows are set to `failed` with error "hub restarted".

### Firmware binaries
- Stored at `data/esphome-builds/{jobID}.bin` on success.
- A background goroutine purges binaries older than 7 days (and their DB rows if status is `done`/`failed`/`cancelled`).
- Re-flash: hub reads binary from disk and serves it at `GET /api/jobs/{id}/firmware` for WebUSB.
- Re-compile: creates a new job with the same `config_json`; does not reuse the old binary.

---

## Compile Queue (`internal/esphome/queue.go`)

```
Queue struct {
    mu      sync.Mutex
    pending []*Job        // ordered, front = next to run
    active  *Job          // currently compiling, nil if idle
    cancel  context.CancelFunc  // cancels the active compile
    jobsMu  sync.RWMutex
    subs    map[string][]chan Event  // SSE subscribers per job ID
}
```

- **Single worker goroutine** drains `pending` one at a time.
- **Enqueue**: appends to `pending`, broadcasts `{status: pending, position: N}` to any existing subscribers, persists to DB.
- **Cancel pending**: removes from `pending` slice, marks DB row `cancelled`, broadcasts cancellation event.
- **Cancel running**: calls `cancel()` (context cancel) → hub's HTTP request to sidecar is aborted → sidecar receives connection close → kills subprocess. Hub marks job `cancelled`.
- **SSE fanout**: each job has a subscriber slice (channels). Worker broadcasts log lines and status changes. On connect, hub replays `log` from DB first (catch-up), then attaches the channel to live updates.

---

## Go Package Changes

| file | change |
|---|---|
| `internal/esphome/sidecar.go` | **New** — `Client` struct with `Compile(ctx, device, logWriter) error` and `Cancel() error` HTTP calls to sidecar |
| `internal/esphome/queue.go` | **New** — `Queue` struct, worker goroutine, SSE fanout, cancel logic |
| `internal/esphome/builder.go` | **Delete** — replaced by sidecar + queue |
| `internal/db/esphome_jobs.go` | **New** — CRUD for `esphome_jobs` table |
| `internal/db/schema.sql` | Add `esphome_jobs` table |
| `internal/db/db.go` | Add `ALTER TABLE` migration guard (new installs get it from schema) |
| `internal/api/jobs.go` | **New** — all job API handlers |
| `internal/api/router.go` | Register `/api/jobs` routes |
| `internal/api/flash.go` | Replace `NewBuilder` call with queue enqueue |
| `internal/api/webflash.go` | Replace `NewBuilder` call with queue enqueue; `esphome-prepare` returns job ID instead of streaming |
| `cmd/server/main.go` | Initialise queue, inject into API layer |

---

## API Endpoints

| method | path | description |
|---|---|---|
| `POST` | `/api/jobs` | Enqueue new compile job. Body: `ESPHomeRequest` JSON. Returns `{id}` immediately. |
| `GET` | `/api/jobs` | List all jobs ordered by `created_at` desc. |
| `GET` | `/api/jobs/{id}` | Job detail: status, device, timestamps, error. |
| `GET` | `/api/jobs/{id}/stream` | SSE stream. Events: `{"type":"log","data":"..."}`, `{"type":"status","data":"running"}`, `{"type":"done","ok":true}` / `{"type":"done","ok":false,"error":"..."}`, `{"type":"position","data":2}` |
| `DELETE` | `/api/jobs/{id}` | Cancel pending or running job. Returns `204`. |
| `POST` | `/api/jobs/{id}/resubmit` | Enqueue a new job from same `config_json`. Returns new `{id}`. |
| `GET` | `/api/jobs/{id}/firmware` | Download stored binary (used by WebUSB flash). `404` if not available. |

---

## Frontend Changes

### Flash wizard (`web/src/views/Flash.svelte`)
- ESPHome compile step: `POST /api/jobs` → receive `{id}` → navigate to `/jobs/{id}`.
- The wizard no longer streams logs inline; it hands off to the job monitor.

### Job monitor page (`web/src/views/JobMonitor.svelte`) — **New**
- Reads job ID from URL (`/jobs/{id}`).
- `GET /api/jobs/{id}` on load for initial state.
- Opens SSE connection to `/api/jobs/{id}/stream`.
- While `pending`: shows spinner + "Position N in queue" + Cancel button.
- While `running`: shows live scrolling log + Cancel button.
- On `done`: success banner + "Flash device" button (opens WebUSB flow with `/api/jobs/{id}/firmware`) + "Re-compile" button.
- On `failed` / `cancelled`: error banner + "Re-compile" button.
- SSE auto-reconnects on disconnect (standard browser EventSource behaviour).

### Jobs nav section (`web/src/views/Jobs.svelte`) — **New**
- Table of all jobs, newest first.
- Columns: device name, status badge, created at, actions.
- Status badges colour-coded: pending (yellow), running (blue + spinner), done (green), failed (red), cancelled (grey).
- Actions per row: Cancel (pending/running), Re-compile (any), Flash (done with binary).
- Clicking a row navigates to `/jobs/{id}`.

### Fleet view (`web/src/views/Fleet.svelte`)
- Each device card gains a small ESPHome job status badge showing the latest job for that device (from `device_id` FK).
- Badge links to `/jobs/{id}`.
- Badge absent if the device has never had an ESPHome compile job.

### Router (`web/src/router.js` or equivalent)
- Add `/jobs/:id` → `JobMonitor.svelte`
- Add `/jobs` → `Jobs.svelte`
- Add "Jobs" entry to nav sidebar.

---

## Error Handling & Edge Cases

- **Sidecar unreachable**: hub marks job `failed` with "ESPHome service unavailable". Queue continues to next job.
- **Sidecar busy (409)**: should not happen if the hub queue is working correctly (queue ensures only one active compile). Hub logs a warning and retries once after 5 seconds before failing the job.
- **Compile timeout**: 15-minute context timeout on the hub side cancels the HTTP request, sidecar connection drops, subprocess is killed.
- **Hub restart mid-compile**: DB rows reset to `failed` on startup. Binary file is incomplete and not stored.
- **Binary missing for done job**: `GET /api/jobs/{id}/firmware` returns `404`. UI shows "Re-compile" instead of "Flash".
- **Concurrent SSE subscribers**: safe — each subscriber gets its own channel; writer holds no locks while sending.

---

## Testing

- `internal/esphome/queue_test.go`: unit tests for enqueue, cancel pending, cancel running (mock sidecar), SSE event ordering.
- `internal/db/esphome_jobs_test.go`: CRUD, state transitions, startup reset.
- `internal/api/jobs_test.go`: handler tests for all endpoints using `httptest`.
- Integration: `esphome-svc/server_test.py` — basic smoke test of the Python server (compile a trivial YAML, check result line).
