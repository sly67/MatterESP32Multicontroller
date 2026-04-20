# Frontend Fixes & Missing UIs — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix all silent-failure bugs, misleading UI, and missing views found in the frontend code review.

**Architecture:** Six independent tasks covering: (1) silent failure error surfaces, (2) YamlModal error prop, (3) Browser Flash firmware version correctness, (4) wizard state reset fixes, (5) OTA version normalization + per-device history, (6) Effects view + sidebar, (7) minor cleanups. All changes are in `web/src/` except Task 3 which also touches `internal/api/webflash.go`.

**Tech Stack:** Svelte 4, DaisyUI, Go (chi router), modernc SQLite

---

## File Structure

| File | Change |
|------|--------|
| `web/src/lib/YamlModal.svelte` | Add `error` prop, async save with catch |
| `web/src/views/Fleet.svelte` | Wrap `openPairModal` in try/catch + error display |
| `web/src/views/Templates.svelte` | Add `importError`, pass to `YamlModal` |
| `web/src/views/Modules.svelte` | Pass existing `importError` to `YamlModal` |
| `web/src/views/Firmware.svelte` | Convert `setLatest`/`remove` to `api` helper + error state |
| `internal/api/webflash.go` | Accept `fw_version` in prepare, add session-aware `/firmware` endpoint |
| `web/src/views/Flash.svelte` | Fix state resets (server + browser flash) |
| `web/src/views/OTA.svelte` | Normalize version comparison, per-device history modal |
| `web/src/views/Effects.svelte` | **New** — list, view YAML, import, delete effects |
| `web/src/lib/Sidebar.svelte` | Add Effects nav entry |
| `web/src/App.svelte` | Import and register Effects view |
| `web/src/lib/api.js` | Remove unused `BASE` constant |

---

## Task 1: YamlModal error prop + save guard

**Files:**
- Modify: `web/src/lib/YamlModal.svelte`

The `Save` button currently calls `() => onSave(yaml)` — not awaited, no catch. If `onSave` throws, the error is an unhandled rejection and the modal stays open with no feedback. This task adds an `error` prop for the caller to pass an error string, and wraps the save call so it cannot produce unhandled rejections.

- [ ] **Step 1: Replace YamlModal.svelte with the fixed version**

Write the full file:

```svelte
<script>
  export let title = '';
  export let yaml = '';
  export let open = false;
  export let readonly = false;
  export let error = '';   // caller sets this to show an error inside the modal

  export let onClose = () => {};
  export let onSave  = null;

  async function handleSave() {
    try { await onSave(yaml); } catch (_) { /* caller sets error prop */ }
  }
</script>

{#if open}
  <button class="fixed inset-0 z-40 bg-black/60 cursor-default" aria-label="Close modal" on:click={onClose}></button>

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
        <div class="px-5 py-3 border-t border-base-300 flex flex-col gap-2">
          {#if error}
            <div class="alert alert-error text-xs py-2">{error}</div>
          {/if}
          <div class="flex justify-end gap-2">
            <button class="btn btn-ghost btn-sm" on:click={onClose}>Cancel</button>
            <button class="btn btn-primary btn-sm" on:click={handleSave}>Save</button>
          </div>
        </div>
      {/if}
    </div>
  </div>
{/if}
```

- [ ] **Step 2: Build frontend to verify no errors**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller/web && npm run build 2>&1
```

Expected: clean build.

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/YamlModal.svelte
git commit -m "fix: YamlModal — async save guard + error prop"
```

---

## Task 2: Silent failure fixes — Fleet, Templates, Modules, Firmware

**Files:**
- Modify: `web/src/views/Fleet.svelte`
- Modify: `web/src/views/Templates.svelte`
- Modify: `web/src/views/Modules.svelte`
- Modify: `web/src/views/Firmware.svelte`

### Fleet.svelte — Pair modal error handling

- [ ] **Step 1: Add pairError state and wrap openPairModal**

In the `<script>` block of `web/src/views/Fleet.svelte`, add `let pairError = '';` after `let qrDataUrl = '';`.

Replace the `openPairModal` function:

```js
async function openPairModal(device) {
  pairError = '';
  try {
    const res = await api.get(`/api/devices/${device.id}/pairing`);
    pairModal = res;
    qrDataUrl = await QRCode.toDataURL(res.qr_payload, { width: 220, margin: 2 });
  } catch (e) {
    pairError = e.message;
  }
}
```

Replace `function closePairModal`:
```js
function closePairModal() { pairModal = null; qrDataUrl = ''; pairError = ''; }
```

In the template, add an error banner above the table (after the filter `<input>`, before the `{#if loading}` block):

```html
{#if pairError}
  <div class="alert alert-error text-sm">{pairError}</div>
{/if}
```

Also remove the old `import { api } from '../lib/api.js';` — it's already imported. The fetch in `openPairModal` was using raw `fetch`; the replacement uses `api.get` which is already imported.

### Templates.svelte — importError

- [ ] **Step 2: Add importError state and wire to YamlModal**

In `web/src/views/Templates.svelte` `<script>` block, add after `let importYaml = '';`:
```js
let importError = '';
```

Replace `importTemplate`:
```js
async function importTemplate(yaml) {
  importError = '';
  const idMatch = yaml.match(/^id:\s*(\S+)/m);
  if (!idMatch) { importError = 'YAML must contain an "id:" field'; return; }
  const id = idMatch[1];
  const nameMatch = yaml.match(/^name:\s*"?([^"\n]+)"?/m);
  const name = nameMatch ? nameMatch[1].replace(/"$/, '').trim() : id;
  try {
    await api.post('/api/templates', { id, name, yaml_body: yaml });
    templates = await api.get('/api/templates');
    importOpen = false;
    importYaml = '';
    importError = '';
  } catch (e) {
    importError = e.message;
  }
}
```

Update the import `YamlModal` usage (add `error={importError}` and clear on close):
```html
<YamlModal title="Import Template YAML" bind:yaml={importYaml} open={importOpen}
  error={importError}
  onClose={() => { importOpen = false; importError = ''; }}
  onSave={importTemplate} />
```

### Modules.svelte — render importError

- [ ] **Step 3: Pass importError to YamlModal**

`importError` is already declared and set in `importModule`. The only change needed is to pass it to the import `YamlModal`:

```html
<YamlModal title="Import Module YAML" bind:yaml={importYaml} open={importOpen}
  error={importError}
  onClose={() => { importOpen = false; importError = ''; }}
  onSave={importModule} />
```

Also remove the `throw e;` at the end of `importModule`'s catch block (line 59) — after Task 1 the error display is handled by `importError`, re-throwing serves no purpose and becomes an unhandled rejection:

```js
async function importModule(yaml) {
  importError = '';
  try {
    const match = yaml.match(/^id:\s*(\S+)/m);
    if (!match) throw new Error('YAML must contain an "id:" field');
    const id = match[1];
    const nameMatch = yaml.match(/^name:\s*"?([^"\n]+)"?/m);
    const name = nameMatch ? nameMatch[1].replace(/"$/, '').trim() : id;
    const catMatch = yaml.match(/^category:\s*(\S+)/m);
    const category = catMatch ? catMatch[1] : '';
    await api.post('/api/modules', { id, name, category, yaml_body: yaml });
    modules = await api.get('/api/modules');
    importOpen = false;
    importYaml = '';
  } catch (e) {
    importError = e.message;
  }
}
```

### Firmware.svelte — setLatest / remove error handling

- [ ] **Step 4: Add firmwareError state; convert setLatest/remove**

In `web/src/views/Firmware.svelte` `<script>` block, add after `let loading = true;`:
```js
let firmwareError = '';
```

Add `import { api } from '../lib/api.js';` at the top of the script (it currently uses raw `fetch` for mutations).

Wait — Firmware.svelte uses raw fetch everywhere including `load()`. To minimize change, keep `load()` as-is and only fix the two mutation functions:

Replace `setLatest`:
```js
async function setLatest(v) {
  firmwareError = '';
  try {
    await api.post(`/api/firmware/${v}/set-latest`, undefined);
    await load();
  } catch (e) {
    firmwareError = e.message;
  }
}
```

Replace `remove`:
```js
async function remove(v) {
  if (!confirm(`Delete firmware ${v}?`)) return;
  firmwareError = '';
  try {
    await api.delete(`/api/firmware/${v}`);
    await load();
  } catch (e) {
    firmwareError = e.message;
  }
}
```

In the template, add the error banner just before the firmware list section (after the builder section, before `{#if loading}`):

```html
{#if firmwareError}
  <div class="alert alert-error text-sm">{firmwareError}</div>
{/if}
```

Also add `import { api } from '../lib/api.js';` to the `<script>` imports (after `import { onMount, onDestroy, tick } from 'svelte';`).

Note: `api.post` for `set-latest` has no body; the `request` helper passes `body: JSON.stringify(undefined)` which is fine — the server ignores body for this endpoint. Alternatively omit body by calling `fetch` directly, but `api.post(path, undefined)` works because `if (body !== undefined)` guards the opts.body assignment. **Actually** `undefined` is !== `undefined` — wait: `api.post: (path, body) => request('POST', path, body)` — if `body` param is `undefined`, then `if (body !== undefined)` is false, so `opts.body` is not set. That's correct.

- [ ] **Step 5: Build frontend**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller/web && npm run build 2>&1
```

Expected: clean build.

- [ ] **Step 6: Commit**

```bash
git add web/src/views/Fleet.svelte web/src/views/Templates.svelte web/src/views/Modules.svelte web/src/views/Firmware.svelte
git commit -m "fix: surface import/mutation errors in Fleet, Templates, Modules, Firmware"
```

---

## Task 3: Browser Flash firmware version selection (end-to-end)

**Files:**
- Modify: `internal/api/webflash.go`

The Browser Flash wizard step 3 lets the user pick a firmware version, but `prepareWebFlash` ignores `fw_version` from the POST body and always serves the latest. The session already stores `fwVersion` — we just need to (a) honour the requested version, (b) add a token-aware `/firmware` endpoint so the manifest can reference the session's version, and (c) update the manifest to use it.

`database.GetFirmware(version string)` already exists in `internal/db/firmware.go`.

- [ ] **Step 1: Update prepareWebFlash request struct and version lookup**

In `webflash.go`, the `prepareWebFlash` handler has a local request struct. Add `FWVersion` field:

```go
var req struct {
    TemplateID   string `json:"template_id"`
    DeviceName   string `json:"device_name"`
    WiFiSSID     string `json:"wifi_ssid"`
    WiFiPassword string `json:"wifi_password"`
    FWVersion    string `json:"fw_version"`
}
```

Replace the `GetLatestFirmware()` call (around line 109) with:

```go
var fw db.FirmwareRow
var fwErr error
if req.FWVersion != "" {
    fw, fwErr = database.GetFirmware(req.FWVersion)
} else {
    fw, fwErr = database.GetLatestFirmware()
}
if fwErr != nil {
    http.Error(w, "firmware not found", http.StatusNotFound)
    return
}
```

- [ ] **Step 2: Add session-aware /firmware endpoint**

Add a new handler function after `serveLatestFirmwareBin`:

```go
func serveSessionFirmwareBin(database *db.Database, firmwareDir string) http.HandlerFunc {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        token := r.URL.Query().Get("token")

        sessionMu.Lock()
        sess, ok := sessions[token]
        sessionMu.Unlock()

        if !ok {
            http.Error(w, "invalid or expired token", http.StatusBadRequest)
            return
        }

        path := filepath.Join(firmwareDir, sess.fwVersion+".bin")
        f, err := os.Open(path)
        if err != nil {
            http.Error(w, "firmware file not found", http.StatusNotFound)
            return
        }
        defer f.Close()
        w.Header().Set("Content-Type", "application/octet-stream")
        w.Header().Set("Content-Disposition", `attachment; filename="firmware.bin"`)
        io.Copy(w, f)
    })
}
```

Register it in `webflashRouter` (in the `return func(r chi.Router)` block), after the `/nvs` line:

```go
r.Get("/firmware", serveSessionFirmwareBin(database, firmwareDir))
```

- [ ] **Step 3: Update the dynamic manifest to use /firmware?token=**

In `serveWebFlashManifestDynamic`, replace the firmware part in `Parts`:

```go
{Path: fmt.Sprintf("/api/webflash/firmware?token=%s", token), Offset: 0x20000},
```

(was `{Path: "/api/webflash/firmware.bin", Offset: 0x20000}`)

- [ ] **Step 4: Build Go**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller && /usr/local/go/bin/go build ./... 2>&1
```

Expected: clean build.

- [ ] **Step 5: Run Go tests**

```bash
/usr/local/go/bin/go test ./... 2>&1
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/api/webflash.go
git commit -m "fix: Browser Flash firmware version — honour fw_version selection end-to-end"
```

---

## Task 4: Flash wizard state reset fixes

**Files:**
- Modify: `web/src/views/Flash.svelte`

Two small state bugs:
1. Server Flash `reset()` doesn't clear `selectedPort` or `selectedFW`
2. Browser Flash "Back" from step 4 doesn't clear `bfPairing`/`bfQrDataUrl`

- [ ] **Step 1: Fix server flash reset**

Replace the `reset` function in `Flash.svelte`:

```js
function reset() {
  step = 1; selectedTemplate = null; deviceNames = [''];
  wifiSSID = ''; wifiPassword = ''; results = []; flashError = '';
  selectedPort = ''; selectedFW = latestVersion;
}
```

- [ ] **Step 2: Fix browser flash back button**

Find the Back button in `bfStep === 4` (the `on:click` that sets `bfStep = 3`):

```svelte
<button class="btn btn-ghost btn-sm"
  disabled={bfFlashing || browserFlashState !== 'idle'}
  on:click={() => { bfStep = 3; bfToken = ''; browserFlashState = 'idle'; bfPairing = null; bfQrDataUrl = ''; }}>← Back</button>
```

- [ ] **Step 3: Build frontend**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller/web && npm run build 2>&1
```

Expected: clean build.

- [ ] **Step 4: Commit**

```bash
git add web/src/views/Flash.svelte
git commit -m "fix: flash wizard — reset selectedPort/FW; clear pairing state on back"
```

---

## Task 5: OTA — version normalization + per-device history modal

**Files:**
- Modify: `web/src/views/OTA.svelte`

Two issues: (1) `needsUpdate` does strict string compare so `v1.2.3` ≠ `1.2.3`. (2) `/api/ota/log/{deviceID}` returns up to 20 OTA events per device but is never shown.

`OTALogRow` shape from the API:
```json
{ "id": 1, "device_id": "...", "from_ver": "1.1.5", "to_ver": "1.1.6", "result": "ok", "created_at": "..." }
```

- [ ] **Step 1: Replace OTA.svelte with the fixed version**

```svelte
<script>
  import { onMount, onDestroy } from 'svelte';
  import { api } from '../lib/api.js';

  let devices = [];
  let latestFW = null;
  let error = '';
  let loading = true;

  let historyDevice = null;  // { id, name } | null
  let historyLog = [];
  let historyLoading = false;
  let historyError = '';

  let interval;

  onMount(async () => {
    await load();
    interval = setInterval(load, 15000);
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

  function normalizeVer(v) { return (v || '').replace(/^v/i, '').trim(); }

  function needsUpdate(dev) {
    return latestFW && dev.fw_version &&
      normalizeVer(dev.fw_version) !== normalizeVer(latestFW.version);
  }

  function statusBadge(dev) {
    if (dev.status === 'online') return 'badge-success';
    if (dev.status === 'offline') return 'badge-error';
    return 'badge-ghost';
  }

  function fwBadge(dev) {
    if (!latestFW) return 'badge-ghost';
    if (!dev.fw_version) return 'badge-ghost';
    if (normalizeVer(dev.fw_version) === normalizeVer(latestFW.version)) return 'badge-success';
    return 'badge-warning';
  }

  async function openHistory(dev) {
    historyDevice = dev;
    historyLog = [];
    historyError = '';
    historyLoading = true;
    try {
      historyLog = await api.get(`/api/ota/log/${dev.id}`);
    } catch (e) {
      historyError = e.message;
    } finally {
      historyLoading = false;
    }
  }

  function closeHistory() { historyDevice = null; historyLog = []; historyError = ''; }

  $: outdatedCount = devices.filter(d => needsUpdate(d)).length;
  $: upToDateCount = latestFW ? devices.filter(d => normalizeVer(d.fw_version) === normalizeVer(latestFW.version)).length : 0;
</script>

{#if historyDevice}
  <button class="fixed inset-0 z-40 bg-black/60 cursor-default" on:click={closeHistory} />
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="bg-base-200 rounded-xl shadow-xl w-full max-w-lg flex flex-col max-h-[80vh]">
      <div class="flex items-center justify-between px-5 py-3 border-b border-base-300">
        <span class="font-semibold text-sm">OTA History — {historyDevice.name}</span>
        <button class="btn btn-ghost btn-xs" on:click={closeHistory}>✕</button>
      </div>
      <div class="flex-1 overflow-auto p-4">
        {#if historyLoading}
          <div class="flex justify-center py-8"><span class="loading loading-spinner"></span></div>
        {:else if historyError}
          <div class="alert alert-error text-sm">{historyError}</div>
        {:else if historyLog.length === 0}
          <div class="text-sm text-base-content/50 text-center py-6">No OTA updates recorded yet.</div>
        {:else}
          <table class="table table-sm">
            <thead>
              <tr><th>Date</th><th>From</th><th>To</th><th>Result</th></tr>
            </thead>
            <tbody>
              {#each historyLog as e (e.id)}
                <tr>
                  <td class="text-xs text-base-content/50">{new Date(e.created_at).toLocaleString()}</td>
                  <td class="font-mono text-xs">{e.from_ver || '—'}</td>
                  <td class="font-mono text-xs">{e.to_ver}</td>
                  <td>
                    <span class="badge badge-xs {e.result === 'ok' ? 'badge-success' : 'badge-error'}">{e.result}</span>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      </div>
      <div class="px-5 pb-4 flex justify-end">
        <button class="btn btn-ghost btn-sm" on:click={closeHistory}>Close</button>
      </div>
    </div>
  </div>
{/if}

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

  {#if latestFW && outdatedCount > 0}
    <div class="alert alert-info text-sm">
      {outdatedCount} device{outdatedCount !== 1 ? 's' : ''} running outdated firmware — they will update automatically on next check-in.
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
            <th></th>
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
              <td><span class="badge badge-sm font-mono {fwBadge(d)}">{d.fw_version || '—'}</span></td>
              <td class="font-mono text-sm text-base-content/60">{latestFW ? latestFW.version : '—'}</td>
              <td class="text-sm text-base-content/50">
                {d.last_seen ? new Date(d.last_seen).toLocaleString() : '—'}
              </td>
              <td>
                <button class="btn btn-xs btn-ghost" on:click={() => openHistory(d)}>History</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <div class="flex gap-4 text-sm text-base-content/60">
      <span>✓ Up to date: <strong>{upToDateCount}</strong></span>
      <span>⚠ Outdated: <strong>{outdatedCount}</strong></span>
      <span>Total: <strong>{devices.length}</strong></span>
    </div>
    <p class="text-xs text-base-content/40">Devices poll the OTA server automatically. Outdated devices download the latest firmware on next check-in.</p>
  {/if}
</div>
```

- [ ] **Step 2: Build frontend**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller/web && npm run build 2>&1
```

Expected: clean build.

- [ ] **Step 3: Commit**

```bash
git add web/src/views/OTA.svelte
git commit -m "feat: OTA — normalize version comparison, per-device history modal"
```

---

## Task 6: Effects view + sidebar

**Files:**
- Create: `web/src/views/Effects.svelte`
- Modify: `web/src/lib/Sidebar.svelte`
- Modify: `web/src/App.svelte`

The API:
- `GET /api/effects` → `[{ id, name, builtin, yaml_body, created_at }]`
- `POST /api/effects` body: `{ id, name, yaml_body }` → 201
- `DELETE /api/effects/{id}` → 204

Built-in effects (`builtin: true`) should not have a delete button.

- [ ] **Step 1: Create Effects.svelte**

Create `web/src/views/Effects.svelte`:

```svelte
<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';
  import YamlModal from '../lib/YamlModal.svelte';

  let effects = [];
  let error = '';
  let loading = true;

  let modalOpen = false;
  let modalTitle = '';
  let modalYaml = '';

  let importOpen = false;
  let importYaml = '';
  let importError = '';

  let deleteTarget = null;

  onMount(async () => {
    try {
      effects = await api.get('/api/effects');
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  function viewEffect(e) {
    modalTitle = e.name || e.id;
    modalYaml = e.yaml_body;
    modalOpen = true;
  }

  async function importEffect(yaml) {
    importError = '';
    const idMatch = yaml.match(/^id:\s*(\S+)/m);
    if (!idMatch) { importError = 'YAML must contain an "id:" field'; return; }
    const id = idMatch[1];
    const nameMatch = yaml.match(/^name:\s*"?([^"\n]+)"?/m);
    const name = nameMatch ? nameMatch[1].replace(/"$/, '').trim() : id;
    try {
      await api.post('/api/effects', { id, name, yaml_body: yaml });
      effects = await api.get('/api/effects');
      importOpen = false;
      importYaml = '';
      importError = '';
    } catch (e) {
      importError = e.message;
    }
  }

  async function doDelete() {
    if (!deleteTarget) return;
    try {
      await api.delete(`/api/effects/${deleteTarget.id}`);
      effects = await api.get('/api/effects');
    } catch (e) {
      error = e.message;
    } finally {
      deleteTarget = null;
    }
  }
</script>

<YamlModal title={modalTitle} yaml={modalYaml} open={modalOpen} readonly
  onClose={() => modalOpen = false} />

<YamlModal title="Import Effect YAML" bind:yaml={importYaml} open={importOpen}
  error={importError}
  onClose={() => { importOpen = false; importError = ''; }}
  onSave={importEffect} />

{#if deleteTarget}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
    <div class="bg-base-200 rounded-xl p-6 shadow-xl max-w-sm w-full mx-4">
      <h3 class="font-semibold mb-2">Delete effect?</h3>
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
    <h2 class="text-lg font-semibold">Effects</h2>
    <button class="btn btn-primary btn-sm" on:click={() => importOpen = true}>+ Import YAML</button>
  </div>

  {#if loading}
    <div class="flex justify-center py-12"><span class="loading loading-spinner"></span></div>
  {:else if error}
    <div class="alert alert-error text-sm">{error}</div>
  {:else if effects.length === 0}
    <div class="text-sm text-base-content/50 py-8 text-center">No effects yet. Import a YAML to get started.</div>
  {:else}
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
      {#each effects as e (e.id)}
        <div class="card bg-base-200 border border-base-300 p-4">
          <div class="flex items-start justify-between gap-2">
            <div>
              <div class="font-semibold text-sm">{e.name || e.id}</div>
              <div class="text-xs font-mono text-base-content/50 mt-0.5">{e.id}</div>
            </div>
            {#if e.builtin}
              <span class="badge badge-ghost badge-sm shrink-0">built-in</span>
            {/if}
          </div>
          <div class="flex gap-2 mt-3">
            <button class="btn btn-ghost btn-xs flex-1" on:click={() => viewEffect(e)}>View YAML</button>
            {#if !e.builtin}
              <button class="btn btn-error btn-xs" on:click={() => deleteTarget = e}>✕</button>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>
```

- [ ] **Step 2: Add Effects to Sidebar**

In `web/src/lib/Sidebar.svelte`, add `{ id: 'effects', label: 'Effects', icon: '✦' }` after the `modules` entry:

```js
const nav = [
  { section: 'Overview' },
  { id: 'fleet',     label: 'Fleet',          icon: '⊞' },
  { id: 'flash',     label: 'Flash Devices',  icon: '⚡' },
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

- [ ] **Step 3: Register Effects in App.svelte**

In `web/src/App.svelte`, add the import and register:

```svelte
<script>
  import Sidebar from './lib/Sidebar.svelte';
  import Fleet     from './views/Fleet.svelte';
  import Flash     from './views/Flash.svelte';
  import Templates from './views/Templates.svelte';
  import Modules   from './views/Modules.svelte';
  import Effects   from './views/Effects.svelte';
  import OTA       from './views/OTA.svelte';
  import Firmware  from './views/Firmware.svelte';
  import Settings  from './views/Settings.svelte';

  let current = 'fleet';

  const views = { Fleet, Flash, Templates, Modules, Effects, OTA, Firmware, Settings };
  $: ViewComponent = views[current.charAt(0).toUpperCase() + current.slice(1)];
</script>
```

- [ ] **Step 4: Build frontend**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller/web && npm run build 2>&1
```

Expected: clean build.

- [ ] **Step 5: Commit**

```bash
git add web/src/views/Effects.svelte web/src/lib/Sidebar.svelte web/src/App.svelte
git commit -m "feat: Effects view — list, import, delete, view YAML"
```

---

## Task 7: Minor cleanups

**Files:**
- Modify: `web/src/lib/api.js`
- Modify: `web/src/views/Fleet.svelte`

Two one-liners:
- Remove the unused `const BASE = ''` from `api.js`
- Null-guard Fleet filter to prevent a crash if a device ever has a null name/status

- [ ] **Step 1: Remove BASE from api.js**

Replace the full `api.js`:

```js
async function request(method, path, body) {
  const opts = {
    method,
    headers: { 'Content-Type': 'application/json' },
  };
  if (body !== undefined) opts.body = JSON.stringify(body);
  const res = await fetch(path, opts);
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status} ${res.statusText}: ${text.trim()}`);
  }
  const ct = res.headers.get('content-type') || '';
  if (ct.includes('application/json')) return res.json();
  return null;
}

export const api = {
  get:    (path)       => request('GET',    path),
  post:   (path, body) => request('POST',   path, body),
  delete: (path)       => request('DELETE', path),
};
```

- [ ] **Step 2: Null-guard Fleet filter**

In `web/src/views/Fleet.svelte`, replace the `$: filtered` line:

```js
$: filtered = devices.filter(d =>
  (d.name || '').toLowerCase().includes(filter.toLowerCase()) ||
  (d.status || '').toLowerCase().includes(filter.toLowerCase())
);
```

- [ ] **Step 3: Build frontend**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller/web && npm run build 2>&1
```

Expected: clean build.

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/api.js web/src/views/Fleet.svelte
git commit -m "chore: remove unused BASE const; null-guard fleet filter"
```

---

## Verification

1. `npm run build` — clean
2. `go build ./...` — clean
3. `go test ./...` — all pass
4. **Fleet Pair button** — click → QR modal; kill server → error banner appears, no hang
5. **Templates import** — paste bad YAML → error shown inside modal; paste good YAML → imports and closes
6. **Modules import** — same as above
7. **Firmware Set Latest / Delete** — server 500 → error banner appears below builder
8. **Browser Flash** — select old firmware version → that version is flashed (not latest)
9. **Browser Flash back** — go back from step 4 → pairing state cleared
10. **Server Flash reset** — flash, click "Flash more devices" → port and firmware selectors reset
11. **OTA** — `v1.2.3` device vs `1.2.3` latest → shows as up-to-date; History button → shows per-device log
12. **Effects** — sidebar shows "Effects"; view lists built-ins; import → appears in list; delete non-builtin → gone; built-in → no delete button
