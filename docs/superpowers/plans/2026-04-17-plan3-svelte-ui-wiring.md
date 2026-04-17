# Plan 3 — Svelte UI Wiring
**Date:** 2026-04-17
**Status:** In Progress

## Goal

Wire the Svelte frontend to the real API endpoints built in Plan 2. Deliver working,
data-driven views for Fleet, Module Library, and Templates. Keep Flash/OTA/Firmware
as stubs — those depend on USB and OTA server work not yet built.

## Current State

Backend ready:
- `GET/POST/DELETE /api/modules` ✅
- `GET/POST/DELETE /api/effects` ✅
- `GET/POST/DELETE /api/templates` ✅
- `GET /api/devices` → 501 stub (needs implementation)
- `GET /api/health` ✅

Frontend: all views are placeholder stubs ("Coming in Plan 4.").

## Out of Scope for Plan 3

- Flash wizard (needs USB / esptool — Plan 4)
- OTA push UI (needs OTA server — Plan 5)
- Firmware upload (Plan 5)
- Matter commissioning UI (Plan 6)

---

## Task 1: Devices API Handler

**Files:**
- Modify: `internal/db/device.go` — add JSON tags to `Device` struct
- Modify: `internal/api/devices.go` — implement list + get handlers
- Create: `internal/api/devices_test.go`

### Step 1: Add JSON tags to `Device`

```go
// internal/db/device.go — Device struct updated
type Device struct {
    ID         string     `json:"id"`
    Name       string     `json:"name"`
    TemplateID string     `json:"template_id"`
    FWVersion  string     `json:"fw_version"`
    PSK        []byte     `json:"-"`         // never expose PSK in API
    Status     string     `json:"status"`
    LastSeen   *time.Time `json:"last_seen"`
    IP         string     `json:"ip"`
    CreatedAt  time.Time  `json:"created_at"`
}
```

### Step 2: Write failing test `internal/api/devices_test.go`

```go
package api_test

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/karthangar/matteresp32hub/internal/db"
)

func TestDevices_ListEmpty(t *testing.T) {
    srv := newTestServer(t)
    req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
    w := httptest.NewRecorder()
    srv.ServeHTTP(w, req)
    assert.Equal(t, http.StatusOK, w.Code)
    var body []interface{}
    require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
    assert.Empty(t, body)
}

func TestDevices_GetMissing(t *testing.T) {
    srv := newTestServer(t)
    req := httptest.NewRequest(http.MethodGet, "/api/devices/nope", nil)
    w := httptest.NewRecorder()
    srv.ServeHTTP(w, req)
    assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDevices_ListOne(t *testing.T) {
    srv := newTestServer(t)
    database := getDatabase(t, srv)

    // insert a template first (FK constraint)
    require.NoError(t, database.CreateTemplate(db.TemplateRow{
        ID: "tpl-1", Name: "T1", Board: "esp32-c3", YAMLBody: "id: tpl-1\n",
    }))
    require.NoError(t, database.CreateDevice(db.Device{
        ID: "dev-1", Name: "1/Bedroom", TemplateID: "tpl-1",
        FWVersion: "1.0.0", PSK: make([]byte, 32),
    }))

    req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
    w := httptest.NewRecorder()
    srv.ServeHTTP(w, req)
    assert.Equal(t, http.StatusOK, w.Code)
    var list []map[string]interface{}
    require.NoError(t, json.NewDecoder(w.Body).Decode(&list))
    require.Len(t, list, 1)
    assert.Equal(t, "1/Bedroom", list[0]["name"])
    assert.Nil(t, list[0]["psk"], "PSK must not appear in API response")
}
```

### Step 3: Run — verify fails

```bash
export PATH=$PATH:/usr/local/go/bin && go test ./internal/api/... -v -run TestDevices 2>&1 | head -10
```

Expected: FAIL — handlers return 501

### Step 4: Implement `internal/api/devices.go`

```go
package api

import (
    "database/sql"
    "encoding/json"
    "errors"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/karthangar/matteresp32hub/internal/db"
)

func devicesRouter(database *db.Database) func(chi.Router) {
    return func(r chi.Router) {
        r.Get("/", listDevices(database))
        r.Get("/{id}", getDevice(database))
    }
}

func listDevices(database *db.Database) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        devs, err := database.ListDevices()
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        if devs == nil {
            devs = []db.Device{}
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(devs)
    }
}

func getDevice(database *db.Database) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        id := chi.URLParam(r, "id")
        dev, err := database.GetDevice(id)
        if errors.Is(err, sql.ErrNoRows) {
            http.Error(w, "not found", http.StatusNotFound)
            return
        }
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(dev)
    }
}
```

### Step 5: Run — verify pass

```bash
export PATH=$PATH:/usr/local/go/bin && go test ./internal/api/... -v -run TestDevices 2>&1
```

Expected: 3 PASS

### Step 6: Full suite + commit

```bash
export PATH=$PATH:/usr/local/go/bin && go test ./... 2>&1
git add internal/db/device.go internal/api/devices.go internal/api/devices_test.go
git commit -m "feat: devices API handler (list, get) with JSON tags"
```

---

## Task 2: Shared API Client

**Files:**
- Create: `web/src/lib/api.js`

A thin fetch wrapper so all views use consistent base URL, error handling, and JSON parsing.

### Step 1: Write `web/src/lib/api.js`

```js
const BASE = '';   // same origin — relative paths work via Go's embedded static

async function request(method, path, body) {
  const opts = {
    method,
    headers: { 'Content-Type': 'application/json' },
  };
  if (body !== undefined) opts.body = JSON.stringify(body);
  const res = await fetch(BASE + path, opts);
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status} ${res.statusText}: ${text.trim()}`);
  }
  const ct = res.headers.get('content-type') || '';
  if (ct.includes('application/json')) return res.json();
  return null;
}

export const api = {
  get:    (path)        => request('GET',    path),
  post:   (path, body)  => request('POST',   path, body),
  delete: (path)        => request('DELETE', path),
};
```

No test for this file — it's validated by the view integration tests below.

---

## Task 3: Fleet View

**Files:**
- Modify: `web/src/views/Fleet.svelte`

Show all registered devices in a table. Status badge colour: `online` → green, `unknown` → gray, `offline` → red. Filter by name/status via a search box.

### Step 1: Write `web/src/views/Fleet.svelte`

```svelte
<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';

  let devices = [];
  let error = '';
  let filter = '';
  let loading = true;

  onMount(async () => {
    try {
      devices = await api.get('/api/devices');
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  $: filtered = devices.filter(d =>
    d.name.toLowerCase().includes(filter.toLowerCase()) ||
    d.status.toLowerCase().includes(filter.toLowerCase())
  );

  const statusClass = s => ({
    online:  'badge-success',
    offline: 'badge-error',
  }[s] || 'badge-ghost');
</script>

<div class="p-6 flex flex-col gap-4">
  <div class="flex items-center justify-between">
    <h2 class="text-lg font-semibold">Fleet</h2>
    <span class="text-sm text-base-content/50">{devices.length} device{devices.length !== 1 ? 's' : ''}</span>
  </div>

  <input
    class="input input-bordered input-sm w-full max-w-xs"
    placeholder="Filter by name or status…"
    bind:value={filter}
  />

  {#if loading}
    <div class="flex justify-center py-12"><span class="loading loading-spinner"></span></div>
  {:else if error}
    <div class="alert alert-error text-sm">{error}</div>
  {:else if filtered.length === 0}
    <div class="text-sm text-base-content/50 py-8 text-center">
      {devices.length === 0 ? 'No devices registered yet. Flash a device to get started.' : 'No devices match the filter.'}
    </div>
  {:else}
    <div class="overflow-x-auto rounded-lg border border-base-200">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>Name</th>
            <th>Status</th>
            <th>Template</th>
            <th>Firmware</th>
            <th>IP</th>
            <th>Last Seen</th>
          </tr>
        </thead>
        <tbody>
          {#each filtered as d (d.id)}
            <tr class="hover">
              <td class="font-mono text-sm">{d.name}</td>
              <td><span class="badge badge-sm {statusClass(d.status)}">{d.status}</span></td>
              <td class="text-sm text-base-content/70">{d.template_id}</td>
              <td class="text-sm font-mono">{d.fw_version || '—'}</td>
              <td class="text-sm font-mono">{d.ip || '—'}</td>
              <td class="text-sm text-base-content/50">{d.last_seen ? new Date(d.last_seen).toLocaleString() : '—'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
```

---

## Task 4: Module Library View

**Files:**
- Modify: `web/src/views/Modules.svelte`
- Create: `web/src/lib/YamlModal.svelte`

Show built-in and imported modules. Filter by category (driver/sensor/io) and name. Click a module to view its YAML in a modal. Import new module via pasting YAML.

### Step 1: Write `web/src/lib/YamlModal.svelte`

Reusable modal for displaying/editing YAML. Used by both Modules and Templates views.

```svelte
<script>
  export let title = '';
  export let yaml = '';
  export let open = false;
  export let readonly = false;

  export let onClose = () => {};
  export let onSave  = null;   // if provided, shows Save button
</script>

{#if open}
  <!-- backdrop -->
  <div class="fixed inset-0 z-40 bg-black/60" on:click={onClose}></div>

  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="bg-base-200 rounded-xl shadow-xl w-full max-w-2xl flex flex-col max-h-[80vh]">
      <div class="flex items-center justify-between px-5 py-3 border-b border-base-300">
        <span class="font-semibold text-sm">{title}</span>
        <button class="btn btn-ghost btn-xs" on:click={onClose}>✕</button>
      </div>
      <div class="flex-1 overflow-auto p-4">
        <textarea
          class="textarea textarea-bordered font-mono text-xs w-full h-full min-h-64 resize-none"
          {readonly}
          bind:value={yaml}
        ></textarea>
      </div>
      {#if onSave}
        <div class="px-5 py-3 border-t border-base-300 flex justify-end gap-2">
          <button class="btn btn-ghost btn-sm" on:click={onClose}>Cancel</button>
          <button class="btn btn-primary btn-sm" on:click={() => onSave(yaml)}>Save</button>
        </div>
      {/if}
    </div>
  </div>
{/if}
```

### Step 2: Write `web/src/views/Modules.svelte`

```svelte
<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import YamlModal from '../lib/YamlModal.svelte';

  let modules = [];
  let error = '';
  let loading = true;
  let filter = '';
  let categoryFilter = 'all';

  let modalOpen = false;
  let modalTitle = '';
  let modalYaml = '';

  let importOpen = false;
  let importYaml = '';
  let importError = '';
  let importLoading = false;

  onMount(async () => {
    try {
      modules = await api.get('/api/modules');
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  $: filtered = modules.filter(m => {
    const matchName = m.name.toLowerCase().includes(filter.toLowerCase()) ||
                      m.id.toLowerCase().includes(filter.toLowerCase());
    const matchCat = categoryFilter === 'all' || m.category === categoryFilter;
    return matchName && matchCat;
  });

  function viewModule(m) {
    modalTitle = m.name;
    modalYaml = m.yaml_body;
    modalOpen = true;
  }

  async function importModule(yaml) {
    importError = '';
    importLoading = true;
    try {
      // parse id from yaml (first line "id: xxx")
      const match = yaml.match(/^id:\s*(\S+)/m);
      if (!match) throw new Error('YAML must contain an "id:" field');
      const id = match[1];
      const nameMatch = yaml.match(/^name:\s*"?([^"\n]+)"?/m);
      const name = nameMatch ? nameMatch[1] : id;
      const catMatch = yaml.match(/^category:\s*(\S+)/m);
      const category = catMatch ? catMatch[1] : '';
      await api.post('/api/modules', { id, name, category, yaml_body: yaml });
      modules = await api.get('/api/modules');
      importOpen = false;
      importYaml = '';
    } catch (e) {
      importError = e.message;
    } finally {
      importLoading = false;
    }
  }

  const categoryBadge = c => ({
    driver: 'badge-primary',
    sensor: 'badge-secondary',
    io:     'badge-accent',
  }[c] || 'badge-ghost');
</script>

<YamlModal title={modalTitle} yaml={modalYaml} open={modalOpen} readonly
  onClose={() => modalOpen = false} />

<YamlModal title="Import Module YAML" bind:yaml={importYaml} open={importOpen}
  onClose={() => { importOpen = false; importError = ''; }}
  onSave={importModule} />

<div class="p-6 flex flex-col gap-4">
  <div class="flex items-center justify-between">
    <h2 class="text-lg font-semibold">Module Library</h2>
    <button class="btn btn-primary btn-sm" on:click={() => importOpen = true}>+ Import YAML</button>
  </div>

  <div class="flex gap-2 flex-wrap">
    <input
      class="input input-bordered input-sm flex-1 min-w-48"
      placeholder="Filter by name or ID…"
      bind:value={filter}
    />
    <select class="select select-bordered select-sm" bind:value={categoryFilter}>
      <option value="all">All categories</option>
      <option value="driver">Driver</option>
      <option value="sensor">Sensor</option>
      <option value="io">I/O</option>
    </select>
  </div>

  {#if loading}
    <div class="flex justify-center py-12"><span class="loading loading-spinner"></span></div>
  {:else if error}
    <div class="alert alert-error text-sm">{error}</div>
  {:else if filtered.length === 0}
    <div class="text-sm text-base-content/50 py-8 text-center">
      {modules.length === 0 ? 'No modules loaded.' : 'No modules match the filter.'}
    </div>
  {:else}
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
      {#each filtered as m (m.id)}
        <button
          class="card bg-base-200 border border-base-300 hover:border-primary/50 transition-all text-left p-4"
          on:click={() => viewModule(m)}
        >
          <div class="flex items-start justify-between gap-2">
            <div>
              <div class="font-semibold text-sm">{m.name}</div>
              <div class="text-xs font-mono text-base-content/50 mt-0.5">{m.id}</div>
            </div>
            <span class="badge badge-sm {categoryBadge(m.category)} shrink-0">{m.category}</span>
          </div>
          {#if m.builtin}
            <div class="text-xs text-base-content/40 mt-2">Built-in</div>
          {/if}
        </button>
      {/each}
    </div>
  {/if}
</div>
```

---

## Task 5: Templates View

**Files:**
- Modify: `web/src/views/Templates.svelte`

Show templates as cards. Click to view YAML. Import via YAML paste. Delete with confirmation.

### Step 1: Write `web/src/views/Templates.svelte`

```svelte
<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import YamlModal from '../lib/YamlModal.svelte';

  let templates = [];
  let error = '';
  let loading = true;

  let modalOpen = false;
  let modalTitle = '';
  let modalYaml = '';

  let importOpen = false;
  let importYaml = '';
  let importError = '';
  let importLoading = false;

  let deleteTarget = null;

  onMount(async () => {
    try {
      templates = await api.get('/api/templates');
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  function viewTemplate(t) {
    modalTitle = t.name || t.id;
    modalYaml = t.yaml_body;
    modalOpen = true;
  }

  async function importTemplate(yaml) {
    importError = '';
    importLoading = true;
    try {
      const idMatch = yaml.match(/^id:\s*(\S+)/m);
      if (!idMatch) throw new Error('YAML must contain an "id:" field');
      const id = idMatch[1];
      const nameMatch = yaml.match(/^name:\s*"?([^"\n]+)"?/m);
      const name = nameMatch ? nameMatch[1] : id;
      await api.post('/api/templates', { id, name, yaml_body: yaml });
      templates = await api.get('/api/templates');
      importOpen = false;
      importYaml = '';
    } catch (e) {
      importError = e.message;
    } finally {
      importLoading = false;
    }
  }

  async function confirmDelete(t) {
    deleteTarget = t;
  }

  async function doDelete() {
    if (!deleteTarget) return;
    try {
      await api.delete(`/api/templates/${deleteTarget.id}`);
      templates = await api.get('/api/templates');
    } catch (e) {
      error = e.message;
    } finally {
      deleteTarget = null;
    }
  }
</script>

<YamlModal title={modalTitle} yaml={modalYaml} open={modalOpen} readonly
  onClose={() => modalOpen = false} />

<YamlModal title="Import Template YAML" bind:yaml={importYaml} open={importOpen}
  onClose={() => { importOpen = false; importError = ''; }}
  onSave={importTemplate} />

<!-- Delete confirmation modal -->
{#if deleteTarget}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
    <div class="bg-base-200 rounded-xl p-6 shadow-xl max-w-sm w-full mx-4">
      <h3 class="font-semibold mb-2">Delete template?</h3>
      <p class="text-sm text-base-content/70 mb-4">
        "<strong>{deleteTarget.name || deleteTarget.id}</strong>" will be permanently removed.
      </p>
      <div class="flex justify-end gap-2">
        <button class="btn btn-ghost btn-sm" on:click={() => deleteTarget = null}>Cancel</button>
        <button class="btn btn-error btn-sm" on:click={doDelete}>Delete</button>
      </div>
    </div>
  </div>
{/if}

<div class="p-6 flex flex-col gap-4">
  <div class="flex items-center justify-between">
    <h2 class="text-lg font-semibold">Templates</h2>
    <button class="btn btn-primary btn-sm" on:click={() => importOpen = true}>+ Import YAML</button>
  </div>

  {#if loading}
    <div class="flex justify-center py-12"><span class="loading loading-spinner"></span></div>
  {:else if error}
    <div class="alert alert-error text-sm">{error}</div>
  {:else if templates.length === 0}
    <div class="text-sm text-base-content/50 py-8 text-center">
      No templates yet. Import a YAML to get started.
    </div>
  {:else}
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
      {#each templates as t (t.id)}
        <div class="card bg-base-200 border border-base-300 p-4">
          <div class="font-semibold text-sm">{t.name || t.id}</div>
          <div class="text-xs font-mono text-base-content/50 mt-0.5">{t.id}</div>
          <div class="text-xs text-base-content/40 mt-1">Board: {t.board}</div>
          <div class="flex gap-2 mt-3">
            <button class="btn btn-ghost btn-xs flex-1" on:click={() => viewTemplate(t)}>View YAML</button>
            <button class="btn btn-error btn-xs" on:click={() => confirmDelete(t)}>✕</button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
```

---

## Task 6: Settings View

**Files:**
- Modify: `web/src/views/Settings.svelte`

Show system info: health status, API version ping. No config editing yet (that's Plan 4).

### Step 1: Write `web/src/views/Settings.svelte`

```svelte
<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';

  let health = null;
  let error = '';

  onMount(async () => {
    try {
      health = await api.get('/api/health');
    } catch (e) {
      error = e.message;
    }
  });
</script>

<div class="p-6 flex flex-col gap-6 max-w-lg">
  <h2 class="text-lg font-semibold">Settings</h2>

  <div class="card bg-base-200 border border-base-300 p-4">
    <div class="text-sm font-semibold mb-3">System Health</div>
    {#if error}
      <div class="alert alert-error text-xs">{error}</div>
    {:else if health}
      <div class="flex items-center gap-2">
        <span class="badge badge-success badge-sm">●</span>
        <span class="text-sm">API reachable — status: <strong>{health.status}</strong></span>
      </div>
    {:else}
      <span class="loading loading-dots loading-sm"></span>
    {/if}
  </div>

  <div class="card bg-base-200 border border-base-300 p-4">
    <div class="text-sm font-semibold mb-2">About</div>
    <div class="text-xs text-base-content/60 space-y-1">
      <div>Web UI port: <span class="font-mono">48060</span></div>
      <div>OTA port: <span class="font-mono">48061</span></div>
      <div>Database: SQLite (WAL mode)</div>
      <div>Transport: HTTPS (self-signed TLS)</div>
    </div>
  </div>

  <div class="card bg-base-200 border border-base-300 p-4">
    <div class="text-sm font-semibold mb-2">Flash / OTA Configuration</div>
    <div class="text-xs text-base-content/50">Available in Plan 4 (USB flashing) and Plan 5 (OTA server).</div>
  </div>
</div>
```

---

## Task 7: Build, Embed & Verify

**Files:**
- Run `npm run build` in `web/`
- Verify the Go binary embeds new dist correctly

### Step 1: Build frontend

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller/web && npm run build 2>&1
```

### Step 2: Full Go test suite

```bash
export PATH=$PATH:/usr/local/go/bin && go test ./... 2>&1
```

### Step 3: Verify Go binary builds

```bash
export PATH=$PATH:/usr/local/go/bin && go build ./cmd/server && echo "BUILD OK" && rm -f server
```

### Step 4: Commit

```bash
git add web/src/ web/dist/ internal/api/devices.go internal/api/devices_test.go internal/db/device.go
git commit -m "feat: Svelte UI wiring — Fleet, Modules, Templates views with real API; devices API handler"
```

---

## Self-Review Checklist

| View | API endpoint used | Done |
|---|---|---|
| Fleet | `GET /api/devices` | Task 3 |
| Module Library | `GET /api/modules`, `POST /api/modules` | Task 4 |
| Templates | `GET /api/templates`, `POST /api/templates`, `DELETE /api/templates/:id` | Task 5 |
| Settings | `GET /api/health` | Task 6 |
| Flash | stub (Plan 4) | — |
| OTA | stub (Plan 5) | — |
| Firmware | stub (Plan 5) | — |

**Shared components:**
- `src/lib/api.js` — fetch wrapper (Task 2)
- `src/lib/YamlModal.svelte` — reusable YAML viewer/editor modal (Task 4)

**PSK safety:** `Device.PSK` tagged `json:"-"` — never serialized in API responses (Task 1).
