# Web Browser Flash Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Flash an ESP32-C3 with hub firmware directly from the browser via the Web Serial API — ESP32 plugged into the user's laptop, no USB access to the Raspberry Pi server required.

**Architecture:** Backend serves four static endpoints (`/api/webflash/`) — bootloader, partition-table, ota_data_initial (embedded in the Go binary), and latest firmware (from DATA_DIR). A manifest.json endpoint assembles these into an esp-web-tools-compatible descriptor. The Svelte Flash view gains a "Browser Flash" tab that loads the `esp-web-tools` custom element; the user clicks once, picks their serial port, and the browser handles all esptool protocol.

**Tech Stack:** Go (chi router, embed.FS), `esp-web-tools` npm package, Svelte 4, Vite, Web Serial API (Chrome/Edge only), DaisyUI.

---

## File Map

| Action   | Path                                              | Responsibility                                    |
|----------|---------------------------------------------------|---------------------------------------------------|
| Create   | `data/flash/esp32c3/bootloader.bin`               | Static ESP32-C3 bootloader (from Docker build)    |
| Create   | `data/flash/esp32c3/partition-table.bin`          | Static partition table (from Docker build)        |
| Create   | `data/flash/esp32c3/ota_data_initial.bin`         | Static OTA data initial (from Docker build)       |
| Modify   | `data/embed.go`                                   | Add `FlashFS` embed directive                     |
| Create   | `internal/api/webflash.go`                        | manifest + static file + firmware.bin handlers    |
| Create   | `internal/api/webflash_test.go`                   | Tests for all webflash endpoints                  |
| Modify   | `internal/api/router.go`                          | Wire `/api/webflash/` sub-router                  |
| Modify   | `web/src/views/Flash.svelte`                      | Add "Browser Flash" tab with esp-web-install-button |

---

### Task 1: Extract static ESP32-C3 flash artifacts from Docker build

The `matter-fw-builder` Docker image produced three static binaries needed for a clean flash: bootloader, partition table, and OTA data initial. These don't change between firmware versions (same chip config), so we commit them to the repo.

**Files:**
- Create: `data/flash/esp32c3/bootloader.bin`
- Create: `data/flash/esp32c3/partition-table.bin`
- Create: `data/flash/esp32c3/ota_data_initial.bin`

- [ ] **Step 1: Create the output directory**

```bash
mkdir -p /home/Karthangar/Projets/MatterESP32Multicontroller/data/flash/esp32c3
```

- [ ] **Step 2: Extract binaries from matter-fw-builder image**

```bash
docker run --rm \
  -v /home/Karthangar/Projets/MatterESP32Multicontroller/data/flash/esp32c3:/out \
  --entrypoint="" \
  matter-fw-builder \
  sh -c "cp /firmware/build/bootloader/bootloader.bin /out/ && \
         cp /firmware/build/partition_table/partition-table.bin /out/ && \
         cp /firmware/build/ota_data_initial.bin /out/"
```

- [ ] **Step 3: Verify the files exist and have sane sizes**

```bash
ls -lh /home/Karthangar/Projets/MatterESP32Multicontroller/data/flash/esp32c3/
```

Expected output (sizes approximate):
```
-rw-r--r-- 1 root root  26K bootloader.bin
-rw-r--r-- 1 root root 3.0K partition-table.bin
-rw-r--r-- 1 root root 8.0K ota_data_initial.bin
```

- [ ] **Step 4: Fix ownership so Git can track them**

```bash
sudo chown -R $USER /home/Karthangar/Projets/MatterESP32Multicontroller/data/flash/
```

- [ ] **Step 5: Commit the extracted binaries**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
git add data/flash/
git commit -m "feat: add ESP32-C3 static flash artifacts (bootloader, partition-table, ota_data_initial)"
```

---

### Task 2: Embed flash artifacts + Go backend endpoints

Add `FlashFS` to `data/embed.go`, then implement `internal/api/webflash.go` with five handlers: manifest, three static binaries, and the latest firmware binary.

**Files:**
- Modify: `data/embed.go`
- Create: `internal/api/webflash.go`
- Create: `internal/api/webflash_test.go`

#### Step-by-step

- [ ] **Step 1: Write the failing tests for the manifest endpoint**

Create `internal/api/webflash_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/karthangar/matteresp32hub/internal/db"
)

func TestWebFlash_Manifest_NoFirmware(t *testing.T) {
	database := db.MustOpenTest(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/webflash/manifest.json", nil)
	serveWebFlashManifest(database).(http.HandlerFunc)(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestWebFlash_Manifest_WithFirmware(t *testing.T) {
	database := db.MustOpenTest(t)
	if err := database.CreateFirmware(db.FirmwareRow{Version: "2.0.0", Boards: "esp32c3", IsLatest: true}); err != nil {
		t.Fatal(err)
	}
	// mark as latest
	if err := database.SetLatestFirmware("2.0.0"); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/webflash/manifest.json", nil)
	serveWebFlashManifest(database).(http.HandlerFunc)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var manifest map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest["version"] != "2.0.0" {
		t.Errorf("expected version 2.0.0, got %v", manifest["version"])
	}
	builds, ok := manifest["builds"].([]interface{})
	if !ok || len(builds) == 0 {
		t.Fatal("expected builds array")
	}
	build := builds[0].(map[string]interface{})
	if build["chipFamily"] != "ESP32-C3" {
		t.Errorf("expected chipFamily ESP32-C3, got %v", build["chipFamily"])
	}
	parts := build["parts"].([]interface{})
	if len(parts) != 4 {
		t.Fatalf("expected 4 parts, got %d", len(parts))
	}
	// Check offsets match the partition layout
	expectedOffsets := []float64{0, 32768, 61440, 131072}
	for i, p := range parts {
		part := p.(map[string]interface{})
		if part["offset"].(float64) != expectedOffsets[i] {
			t.Errorf("part %d: expected offset %v, got %v", i, expectedOffsets[i], part["offset"])
		}
	}
}

func TestWebFlash_Firmware_NoLatest(t *testing.T) {
	database := db.MustOpenTest(t)
	dir := t.TempDir()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/webflash/firmware.bin", nil)
	serveLatestFirmwareBin(database, dir).(http.HandlerFunc)(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestWebFlash_Firmware_ServesLatest(t *testing.T) {
	database := db.MustOpenTest(t)
	dir := t.TempDir()
	if err := database.CreateFirmware(db.FirmwareRow{Version: "1.5.0", Boards: "esp32c3"}); err != nil {
		t.Fatal(err)
	}
	if err := database.SetLatestFirmware("1.5.0"); err != nil {
		t.Fatal(err)
	}
	// Write a fake firmware file
	content := []byte("fake-firmware-binary")
	if err := os.WriteFile(filepath.Join(dir, "1.5.0.bin"), content, 0644); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/webflash/firmware.bin", nil)
	serveLatestFirmwareBin(database, dir).(http.HandlerFunc)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != string(content) {
		t.Errorf("body mismatch: got %q", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("expected octet-stream, got %q", ct)
	}
}

// Suppress unused import warning — time is used implicitly via db seed
var _ = time.Now
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
go test ./internal/api/ -run TestWebFlash -v 2>&1 | head -30
```

Expected: compile error `serveWebFlashManifest undefined` or test failures.

- [ ] **Step 3: Add `FlashFS` embed directive to `data/embed.go`**

Current `data/embed.go`:
```go
package data

import "embed"

//go:embed all:modules
var ModulesFS embed.FS

//go:embed all:effects
var EffectsFS embed.FS

//go:embed all:boards
var BoardsFS embed.FS

//go:embed all:templates
var TemplatesFS embed.FS
```

Add after `TemplatesFS`:
```go
//go:embed all:flash
var FlashFS embed.FS
```

Full updated `data/embed.go`:
```go
package data

import "embed"

//go:embed all:modules
var ModulesFS embed.FS

//go:embed all:effects
var EffectsFS embed.FS

//go:embed all:boards
var BoardsFS embed.FS

//go:embed all:templates
var TemplatesFS embed.FS

//go:embed all:flash
var FlashFS embed.FS
```

- [ ] **Step 4: Create `internal/api/webflash.go`**

```go
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	godata "github.com/karthangar/matteresp32hub/data"
	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
)

func webflashRouter(cfg *config.Config, database *db.Database) func(chi.Router) {
	dataDir := cfg.DataDir
	if dataDir == "" {
		dataDir = "./data"
	}
	firmwareDir := filepath.Join(dataDir, "firmware")

	return func(r chi.Router) {
		r.Get("/manifest.json", serveWebFlashManifest(database))
		r.Get("/bootloader.bin", serveFlashStatic("flash/esp32c3/bootloader.bin", "bootloader.bin"))
		r.Get("/partition-table.bin", serveFlashStatic("flash/esp32c3/partition-table.bin", "partition-table.bin"))
		r.Get("/ota_data_initial.bin", serveFlashStatic("flash/esp32c3/ota_data_initial.bin", "ota_data_initial.bin"))
		r.Get("/firmware.bin", serveLatestFirmwareBin(database, firmwareDir))
	}
}

// serveWebFlashManifest returns an esp-web-tools-compatible manifest JSON.
// Returns 503 if no firmware is marked as latest.
func serveWebFlashManifest(database *db.Database) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fw, err := database.GetLatestFirmware()
		if err != nil {
			http.Error(w, "no firmware available", http.StatusServiceUnavailable)
			return
		}
		type part struct {
			Path   string `json:"path"`
			Offset int    `json:"offset"`
		}
		type build struct {
			ChipFamily string `json:"chipFamily"`
			Parts      []part `json:"parts"`
		}
		manifest := struct {
			Name    string  `json:"name"`
			Version string  `json:"version"`
			Builds  []build `json:"builds"`
		}{
			Name:    "Matter Hub Firmware",
			Version: fw.Version,
			Builds: []build{
				{
					ChipFamily: "ESP32-C3",
					Parts: []part{
						{Path: "/api/webflash/bootloader.bin", Offset: 0x0},
						{Path: "/api/webflash/partition-table.bin", Offset: 0x8000},
						{Path: "/api/webflash/ota_data_initial.bin", Offset: 0xf000},
						{Path: "/api/webflash/firmware.bin", Offset: 0x20000},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest)
	})
}

// serveFlashStatic serves a binary file embedded in data.FlashFS.
// embedPath is relative to the data package root (e.g. "flash/esp32c3/bootloader.bin").
// filename is the Content-Disposition download name.
func serveFlashStatic(embedPath, filename string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, err := godata.FlashFS.ReadFile(embedPath)
		if err != nil {
			http.Error(w, "static file not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
		w.Write(content)
	})
}

// serveLatestFirmwareBin streams the latest firmware binary from firmwareDir.
// Returns 503 if no latest firmware is set, 404 if the file is missing.
func serveLatestFirmwareBin(database *db.Database, firmwareDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fw, err := database.GetLatestFirmware()
		if err != nil {
			http.Error(w, "no firmware available", http.StatusServiceUnavailable)
			return
		}
		path := filepath.Join(firmwareDir, fw.Version+".bin")
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

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
go test ./internal/api/ -run TestWebFlash -v
```

Expected:
```
--- PASS: TestWebFlash_Manifest_NoFirmware (0.00s)
--- PASS: TestWebFlash_Manifest_WithFirmware (0.00s)
--- PASS: TestWebFlash_Firmware_NoLatest (0.00s)
--- PASS: TestWebFlash_Firmware_ServesLatest (0.00s)
PASS
```

- [ ] **Step 6: Verify full test suite still passes**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
go test ./... 2>&1 | tail -20
```

Expected: all packages `ok`, no failures.

- [ ] **Step 7: Commit**

```bash
git add data/embed.go internal/api/webflash.go internal/api/webflash_test.go
git commit -m "feat: web flash backend — manifest + static binary endpoints"
```

---

### Task 3: Wire webflash router

**Files:**
- Modify: `internal/api/router.go`

- [ ] **Step 1: Add the webflash route to router.go**

Current `internal/api/router.go`:
```go
func NewRouter(cfg *config.Config, database *db.Database) http.Handler {
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
	r.Route("/api/flash", flashRouter(database))
	r.Route("/api/settings", settingsRouter(cfg, database))
	r.Route("/api/ota", otaRouter(database))

	r.Handle("/*", staticHandler())
	return r
}
```

Add the webflash route after `/api/flash`:
```go
	r.Route("/api/webflash", webflashRouter(cfg, database))
```

Full updated `internal/api/router.go`:
```go
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/karthangar/matteresp32hub/internal/config"
	"github.com/karthangar/matteresp32hub/internal/db"
)

// NewRouter builds and returns the chi HTTP router.
func NewRouter(cfg *config.Config, database *db.Database) http.Handler {
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
	r.Route("/api/flash", flashRouter(database))
	r.Route("/api/webflash", webflashRouter(cfg, database))
	r.Route("/api/settings", settingsRouter(cfg, database))
	r.Route("/api/ota", otaRouter(database))

	// Frontend — served from embedded FS (wired in Task 7)
	r.Handle("/*", staticHandler())

	return r
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
go build ./...
```

Expected: no output (clean compile).

- [ ] **Step 3: Commit**

```bash
git add internal/api/router.go
git commit -m "feat: wire /api/webflash/ router"
```

---

### Task 4: Svelte Browser Flash tab

Add a "Browser Flash" tab to `web/src/views/Flash.svelte`. The tab uses `esp-web-tools`, installed as an npm package so it's bundled into the Vite build (no CDN dependency at runtime — important for local-network operation).

**Files:**
- Modify: `web/src/views/Flash.svelte`

- [ ] **Step 1: Install esp-web-tools**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller/web
npm install esp-web-tools
```

- [ ] **Step 2: Verify the package installed**

```bash
ls /home/Karthangar/Projets/MatterESP32Multicontroller/web/node_modules/esp-web-tools/dist/web/
```

Expected: `install-button.js` present.

- [ ] **Step 3: Update Flash.svelte to add the Browser Flash tab**

Replace the entire `web/src/views/Flash.svelte` with:

```svelte
<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';

  // ── Tab ────────────────────────────────────────────────────────────────────
  let activeTab = 'server'; // 'server' | 'browser'

  // ── Browser Flash ──────────────────────────────────────────────────────────
  import 'esp-web-tools';

  let installButton;
  let browserFlashState = 'idle'; // idle | connecting | writing | done | error
  let browserFlashMsg = '';
  let firmwareAvailable = false;
  let latestVersion = '';

  onMount(async () => {
    try {
      const firmware = await api.get('/api/firmware');
      const latest = firmware.find(f => f.is_latest);
      if (latest) {
        firmwareAvailable = true;
        latestVersion = latest.version;
      }
    } catch (_) {}

    if (installButton) {
      installButton.addEventListener('state-changed', (e) => {
        const state = e.detail?.state;
        if (!state) return;
        if (state === 'finished') {
          browserFlashState = 'done';
          browserFlashMsg = '';
        } else if (state === 'error') {
          browserFlashState = 'error';
          browserFlashMsg = e.detail?.message || 'Flash failed';
        } else if (state === 'initializing' || state === 'preparing') {
          browserFlashState = 'connecting';
          browserFlashMsg = 'Connecting to device…';
        } else if (state === 'writing') {
          browserFlashState = 'writing';
          browserFlashMsg = e.detail?.details || 'Writing firmware…';
        }
      });
    }
  });

  // ── Server Flash (existing wizard) ────────────────────────────────────────
  let step = 1;
  let templates = [];
  let firmware = [];
  let ports = [];
  let loadingInit = true;
  let error = '';

  let selectedTemplate = null;
  let deviceNames = [''];
  let wifiSSID = '';
  let wifiPassword = '';
  let selectedPort = '';
  let selectedFW = '';

  let flashing = false;
  let results = [];
  let flashError = '';

  onMount(async () => {
    try {
      [templates, firmware, ports] = await Promise.all([
        api.get('/api/templates'),
        api.get('/api/firmware'),
        api.get('/api/flash/ports'),
      ]);
      const latest = firmware.find(f => f.is_latest);
      if (latest) selectedFW = latest.version;
    } catch (e) {
      error = e.message;
    } finally {
      loadingInit = false;
    }
  });

  async function refreshPorts() {
    ports = await api.get('/api/flash/ports');
  }

  function addName() { deviceNames = [...deviceNames, '']; }
  function removeName(i) { deviceNames = deviceNames.filter((_, idx) => idx !== i); }

  async function doFlash() {
    flashError = '';
    flashing = true;
    results = [];
    try {
      results = await api.post('/api/flash/run', {
        template_id:   selectedTemplate.id,
        device_names:  deviceNames.filter(n => n.trim()),
        wifi_ssid:     wifiSSID,
        wifi_password: wifiPassword,
        port:          selectedPort,
        fw_version:    selectedFW,
      });
      step = 5;
    } catch (e) {
      flashError = e.message;
    } finally {
      flashing = false;
    }
  }

  function reset() {
    step = 1; selectedTemplate = null; deviceNames = [''];
    wifiSSID = ''; wifiPassword = ''; results = []; flashError = '';
  }
</script>

<div class="p-6 flex flex-col gap-4 max-w-2xl">
  <h2 class="text-lg font-semibold">Flash Devices</h2>

  <!-- Tab switcher -->
  <div role="tablist" class="tabs tabs-bordered">
    <button role="tab" class="tab {activeTab === 'server' ? 'tab-active' : ''}"
      on:click={() => activeTab = 'server'}>
      🖥 Server Flash <span class="ml-1 text-xs text-base-content/40">(ESP32 on RPi USB)</span>
    </button>
    <button role="tab" class="tab {activeTab === 'browser' ? 'tab-active' : ''}"
      on:click={() => activeTab = 'browser'}>
      🌐 Browser Flash <span class="ml-1 text-xs text-base-content/40">(ESP32 on your USB)</span>
    </button>
  </div>

  <!-- ── Browser Flash ──────────────────────────────────────────────────── -->
  {#if activeTab === 'browser'}
    <div class="flex flex-col gap-4">
      <div class="text-sm text-base-content/60">
        Plug your ESP32-C3 into <strong>your computer</strong> via USB, then click the button below.
        The browser will flash the latest hub firmware directly over the serial connection.<br>
        <span class="text-warning text-xs">⚠ Requires Chrome or Edge (Web Serial API).</span>
      </div>

      {#if !firmwareAvailable}
        <div class="alert alert-warning text-sm">
          No firmware marked as latest. Upload and set a firmware version in the
          <strong>Firmware</strong> view first.
        </div>
      {:else}
        <div class="flex items-center gap-3 p-3 rounded-lg bg-base-200 border border-base-300 text-sm">
          <span class="text-base-content/50">Firmware to flash:</span>
          <span class="font-mono font-semibold">{latestVersion}</span>
        </div>

        {#if browserFlashState === 'done'}
          <div class="alert alert-success text-sm">
            ✓ Flash complete! The device will reboot into the Matter hub firmware.
            Use the <strong>Server Flash</strong> tab to provision it with WiFi and device credentials.
          </div>
          <button class="btn btn-ghost btn-sm self-start"
            on:click={() => { browserFlashState = 'idle'; browserFlashMsg = ''; }}>
            Flash another device
          </button>

        {:else if browserFlashState === 'error'}
          <div class="alert alert-error text-sm">✗ {browserFlashMsg || 'Flash failed'}</div>
          <button class="btn btn-ghost btn-sm self-start"
            on:click={() => { browserFlashState = 'idle'; browserFlashMsg = ''; }}>
            Try again
          </button>

        {:else}
          {#if browserFlashState !== 'idle'}
            <div class="flex items-center gap-2 text-sm text-base-content/70">
              <span class="loading loading-spinner loading-xs"></span>
              {browserFlashMsg}
            </div>
          {/if}

          <esp-web-install-button
            bind:this={installButton}
            manifest="/api/webflash/manifest.json"
          >
            <button
              slot="activate"
              class="btn btn-primary"
              disabled={browserFlashState !== 'idle'}
            >
              ⚡ Connect &amp; Flash ESP32-C3
            </button>
            <span slot="unsupported" class="alert alert-error text-sm">
              Web Serial is not supported in this browser. Use Chrome or Edge.
            </span>
          </esp-web-install-button>
        {/if}
      {/if}
    </div>
  {/if}

  <!-- ── Server Flash (existing wizard) ────────────────────────────────── -->
  {#if activeTab === 'server'}
    {#if loadingInit}
      <div class="flex justify-center py-12"><span class="loading loading-spinner"></span></div>
    {:else if error}
      <div class="alert alert-error text-sm">{error}</div>
    {:else}

    <ul class="steps steps-horizontal w-full text-xs">
      <li class="step {step >= 1 ? 'step-primary' : ''}">Template</li>
      <li class="step {step >= 2 ? 'step-primary' : ''}">Names</li>
      <li class="step {step >= 3 ? 'step-primary' : ''}">WiFi &amp; Port</li>
      <li class="step {step >= 4 ? 'step-primary' : ''}">Flash</li>
      <li class="step {step >= 5 ? 'step-primary' : ''}">Done</li>
    </ul>

    {#if step === 1}
      <div class="flex flex-col gap-3">
        <div class="text-sm font-semibold">Select a template</div>
        {#if templates.length === 0}
          <div class="text-sm text-base-content/50">No templates yet — create one in the Templates view.</div>
        {:else}
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
            {#each templates as t}
              <button
                class="card p-3 border text-left transition-all
                  {selectedTemplate?.id === t.id ? 'border-primary bg-primary/10' : 'border-base-300 bg-base-200 hover:border-primary/40'}"
                on:click={() => selectedTemplate = t}
              >
                <div class="font-semibold text-sm">{t.name || t.id}</div>
                <div class="text-xs text-base-content/50">{t.board}</div>
              </button>
            {/each}
          </div>
          <button class="btn btn-primary btn-sm self-end" disabled={!selectedTemplate}
            on:click={() => step = 2}>Next →</button>
        {/if}
      </div>

    {:else if step === 2}
      <div class="flex flex-col gap-3">
        <div class="text-sm font-semibold">Device names <span class="text-base-content/40 font-normal">(e.g. 1/Bedroom)</span></div>
        {#each deviceNames as _, i}
          <div class="flex gap-2">
            <input class="input input-bordered input-sm flex-1" placeholder="e.g. {i+1}/Room"
              bind:value={deviceNames[i]} />
            {#if deviceNames.length > 1}
              <button class="btn btn-ghost btn-sm" on:click={() => removeName(i)}>✕</button>
            {/if}
          </div>
        {/each}
        <button class="btn btn-ghost btn-sm self-start" on:click={addName}>+ Add device</button>
        <div class="flex gap-2 justify-end">
          <button class="btn btn-ghost btn-sm" on:click={() => step = 1}>← Back</button>
          <button class="btn btn-primary btn-sm"
            disabled={deviceNames.every(n => !n.trim())}
            on:click={() => step = 3}>Next →</button>
        </div>
      </div>

    {:else if step === 3}
      <div class="flex flex-col gap-3">
        <div class="text-sm font-semibold">WiFi credentials</div>
        <input class="input input-bordered input-sm" placeholder="WiFi SSID" bind:value={wifiSSID} />
        <input class="input input-bordered input-sm" type="password" placeholder="WiFi password" bind:value={wifiPassword} />

        <div class="divider my-1"></div>
        <div class="flex items-center gap-2 text-sm font-semibold">
          USB port
          <button class="btn btn-ghost btn-xs" on:click={refreshPorts}>↻ Refresh</button>
        </div>
        {#if ports.length === 0}
          <div class="text-sm text-base-content/50">No USB ports detected. Plug in your ESP32 and refresh.</div>
        {:else}
          <select class="select select-bordered select-sm" bind:value={selectedPort}>
            <option value="">Select port…</option>
            {#each ports as p}<option value={p.path}>{p.name} ({p.path})</option>{/each}
          </select>
        {/if}

        <div class="divider my-1"></div>
        <div class="text-sm font-semibold">Firmware version</div>
        {#if firmware.length === 0}
          <div class="text-sm text-base-content/50">No firmware uploaded — go to the Firmware view first.</div>
        {:else}
          <select class="select select-bordered select-sm" bind:value={selectedFW}>
            <option value="">Select version…</option>
            {#each firmware as f}<option value={f.version}>{f.version}{f.is_latest ? ' (latest)' : ''}</option>{/each}
          </select>
        {/if}

        <div class="flex gap-2 justify-end">
          <button class="btn btn-ghost btn-sm" on:click={() => step = 2}>← Back</button>
          <button class="btn btn-primary btn-sm"
            disabled={!wifiSSID || !selectedPort || !selectedFW}
            on:click={() => step = 4}>Next →</button>
        </div>
      </div>

    {:else if step === 4}
      <div class="flex flex-col gap-3">
        <div class="card bg-base-200 border border-base-300 p-4 text-sm space-y-1">
          <div><strong>Template:</strong> {selectedTemplate.name || selectedTemplate.id}</div>
          <div><strong>Devices ({deviceNames.filter(n=>n.trim()).length}):</strong> {deviceNames.filter(n=>n.trim()).join(', ')}</div>
          <div><strong>Port:</strong> {selectedPort}</div>
          <div><strong>Firmware:</strong> {selectedFW}</div>
          <div><strong>WiFi:</strong> {wifiSSID}</div>
        </div>
        {#if flashError}<div class="alert alert-error text-sm">{flashError}</div>{/if}
        <div class="flex gap-2 justify-end">
          <button class="btn btn-ghost btn-sm" disabled={flashing} on:click={() => step = 3}>← Back</button>
          <button class="btn btn-warning btn-sm" disabled={flashing} on:click={doFlash}>
            {#if flashing}<span class="loading loading-spinner loading-xs"></span> Flashing…{:else}⚡ Flash Now{/if}
          </button>
        </div>
      </div>

    {:else if step === 5}
      <div class="flex flex-col gap-3">
        {#each results as r}
          <div class="flex items-center gap-3 p-3 rounded-lg border {r.ok ? 'border-success/40 bg-success/10' : 'border-error/40 bg-error/10'}">
            <span class="text-xl">{r.ok ? '✓' : '✗'}</span>
            <div class="flex-1">
              <div class="font-semibold text-sm">{r.name}</div>
              {#if r.device_id}<div class="text-xs font-mono text-base-content/50">{r.device_id}</div>{/if}
              {#if r.error}<div class="text-xs text-error mt-0.5">{r.error}</div>{/if}
            </div>
          </div>
        {/each}
        <button class="btn btn-ghost btn-sm self-start mt-2" on:click={reset}>Flash more devices</button>
      </div>
    {/if}

    {/if}
  {/if}
</div>
```

- [ ] **Step 4: Build the Svelte frontend**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller/web
npm run build 2>&1 | tail -20
```

Expected:
```
✓ built in ...ms
```
No errors. The `dist/` directory is updated.

- [ ] **Step 5: Verify Go still compiles with the updated embed**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
go build ./...
```

Expected: clean compile.

- [ ] **Step 6: Commit**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
git add web/src/views/Flash.svelte web/package.json web/package-lock.json web/dist/
git commit -m "feat: Browser Flash tab — esp-web-tools integration for laptop-connected ESP32"
```

---

## Self-Review Checklist

**Spec coverage:**
- ✅ ESP32 can be flashed from the browser via USB serial (esp-web-tools + Web Serial API)
- ✅ Firmware binary served from hub's DATA_DIR (latest version)
- ✅ Static flash artifacts (bootloader, partition-table, ota_data_initial) embedded and served
- ✅ esp-web-tools manifest JSON assembled from live firmware version
- ✅ UI shows firmware version being flashed, progress states (connecting/writing/done/error)
- ✅ Graceful degradation: 503 if no firmware available, warning shown in UI
- ✅ Browser incompatibility handled (unsupported slot in esp-web-install-button)
- ✅ Existing Server Flash workflow preserved unchanged as a tab
- ✅ No CDN dependency at runtime (esp-web-tools bundled via npm/Vite)

**Placeholder scan:** No TBD/TODO/placeholder patterns present.

**Type consistency:**
- `serveWebFlashManifest` signature: `(database *db.Database) http.Handler` — matches test usage ✅
- `serveLatestFirmwareBin` signature: `(database *db.Database, firmwareDir string) http.Handler` — matches test usage ✅
- `serveFlashStatic` signature: `(embedPath, filename string) http.Handler` — self-contained ✅
- `webflashRouter` signature: `(cfg *config.Config, database *db.Database) func(chi.Router)` — matches `router.go` call pattern ✅
- Manifest offsets: `0x0, 0x8000, 0xf000, 0x20000` — match `firmware/Dockerfile.build` esptool write_flash command ✅
- Embedded path: `flash/esp32c3/bootloader.bin` — matches `data/flash/esp32c3/` directory layout ✅
