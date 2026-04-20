# Matter Commissioning Pairing Page — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Store Matter discriminator + passcode at flash time, expose a `/api/devices/{id}/pairing` endpoint, and add a "Pair" button to the Fleet view showing a scannable QR code so the user can commission an ESP32-C3 into Apple Home / Google Home without touching the serial console.

**Architecture:** Two storage paths — server flash (orchestrator) creates a DB record and saves creds there; browser flash (webflash) is ephemeral so its pairing info is returned inline in the `prepareWebFlash` JSON response and displayed in the Flash wizard Done screen. A new `internal/matter` package computes the `MT:XXXX` Matter setup QR payload from discriminator + passcode using the standard Base38 encoding specified in the Matter core spec. The Fleet view fetches pairing info from the new endpoint and renders the QR code client-side using the `qrcode` npm package.

**Tech Stack:** Go (chi router, modernc SQLite), Svelte 4, DaisyUI, `qrcode` npm package

---

## File Structure

| File | Change |
|------|--------|
| `internal/db/schema.sql` | Add `matter_discrim`, `matter_passcode` columns to `devices` |
| `internal/db/db.go` | Add `ALTER TABLE ADD COLUMN` upgrades for existing installs |
| `internal/db/device.go` | Add fields to `Device` struct; add `UpdateDeviceMatterCreds` |
| `internal/db/db_test.go` | Add test for `UpdateDeviceMatterCreds` |
| `internal/matter/payload.go` | **New** — `SetupQRPayload(discrim, passcode)` → `"MT:XXXX"` |
| `internal/matter/payload_test.go` | **New** — tests for Base38 encoding + payload format |
| `internal/flash/orchestrator.go` | Call `UpdateDeviceMatterCreds` after `CreateDevice` |
| `internal/api/webflash.go` | Add `discriminator`, `passcode`, `qr_payload` to response |
| `internal/api/devices.go` | Add `getPairingInfo` handler |
| `internal/api/router.go` | Register `GET /api/devices/{id}/pairing` |
| `internal/api/devices_test.go` | Add test for pairing endpoint |
| `web/package.json` | Add `"qrcode": "^1.5.3"` dependency |
| `web/src/views/Fleet.svelte` | Pair button + modal with QR code |
| `web/src/views/Flash.svelte` | Show pairing info in Browser Flash done screen |

---

## Task 1: DB schema + Device struct + UpdateDeviceMatterCreds

**Files:**
- Modify: `internal/db/schema.sql`
- Modify: `internal/db/db.go`
- Modify: `internal/db/device.go`
- Test: `internal/db/db_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/db/db_test.go`:

```go
func TestDevice_UpdateMatterCreds(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	_, err = database.DB.Exec(
		`INSERT INTO templates (id, name, board, yaml_body) VALUES (?, ?, ?, ?)`,
		"tpl-1", "T1", "esp32-c3", "id: tpl-1",
	)
	require.NoError(t, err)
	require.NoError(t, database.CreateDevice(db.Device{
		ID: "dev-1", Name: "1/Bedroom", TemplateID: "tpl-1", PSK: make([]byte, 32),
	}))

	require.NoError(t, database.UpdateDeviceMatterCreds("dev-1", 3840, 20202021))

	got, err := database.GetDevice("dev-1")
	require.NoError(t, err)
	assert.Equal(t, uint16(3840), got.MatterDiscrim)
	assert.Equal(t, uint32(20202021), got.MatterPasscode)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
go test ./internal/db/... -run TestDevice_UpdateMatterCreds -v
```

Expected: FAIL — `got.MatterDiscrim` field does not exist yet.

- [ ] **Step 3: Add columns to schema.sql**

In `internal/db/schema.sql`, change the devices table definition:

```sql
CREATE TABLE IF NOT EXISTS devices (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    template_id     TEXT NOT NULL REFERENCES templates(id),
    fw_version      TEXT NOT NULL DEFAULT '',
    psk             BLOB NOT NULL,
    status          TEXT NOT NULL DEFAULT 'unknown',
    last_seen       DATETIME,
    ip              TEXT NOT NULL DEFAULT '',
    matter_discrim  INTEGER NOT NULL DEFAULT 0,
    matter_passcode INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

- [ ] **Step 4: Add ALTER TABLE upgrades in db.go for existing installs**

In `internal/db/db.go`, after `sqldb.Exec(schema)` succeeds, add:

```go
	// Additive column upgrades — safe to ignore if column already exists (new installs).
	for _, up := range []string{
		`ALTER TABLE devices ADD COLUMN matter_discrim  INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE devices ADD COLUMN matter_passcode INTEGER NOT NULL DEFAULT 0`,
	} {
		sqldb.Exec(up) //nolint:errcheck // expected to fail on new installs where column already exists
	}
```

The full `Open` function body after the pragma block should be:

```go
	if _, err := sqldb.Exec(schema); err != nil {
		sqldb.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	for _, up := range []string{
		`ALTER TABLE devices ADD COLUMN matter_discrim  INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE devices ADD COLUMN matter_passcode INTEGER NOT NULL DEFAULT 0`,
	} {
		sqldb.Exec(up) //nolint:errcheck
	}
	return &Database{DB: sqldb}, nil
```

- [ ] **Step 5: Update Device struct and CRUD in device.go**

Add fields to `Device` struct (after `IP string`):

```go
	MatterDiscrim  uint16 `json:"-"` // commissioning only — never in list/get JSON responses
	MatterPasscode uint32 `json:"-"` // commissioning only — never in list/get JSON responses
```

Update `GetDevice` to scan the new columns:

```go
func (d *Database) GetDevice(id string) (Device, error) {
	row := d.DB.QueryRow(
		`SELECT id, name, template_id, fw_version, psk, status, last_seen, ip,
		        matter_discrim, matter_passcode, created_at
		 FROM devices WHERE id = ?`, id)
	var dev Device
	var lastSeen *time.Time
	if err := row.Scan(&dev.ID, &dev.Name, &dev.TemplateID, &dev.FWVersion,
		&dev.PSK, &dev.Status, &lastSeen, &dev.IP,
		&dev.MatterDiscrim, &dev.MatterPasscode, &dev.CreatedAt); err != nil {
		return Device{}, err
	}
	dev.LastSeen = lastSeen
	return dev, nil
}
```

Update `ListDevices` similarly:

```go
func (d *Database) ListDevices() ([]Device, error) {
	rows, err := d.DB.Query(
		`SELECT id, name, template_id, fw_version, psk, status, last_seen, ip,
		        matter_discrim, matter_passcode, created_at
		 FROM devices ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devs []Device
	for rows.Next() {
		var dev Device
		var lastSeen *time.Time
		if err := rows.Scan(&dev.ID, &dev.Name, &dev.TemplateID, &dev.FWVersion,
			&dev.PSK, &dev.Status, &lastSeen, &dev.IP,
			&dev.MatterDiscrim, &dev.MatterPasscode, &dev.CreatedAt); err != nil {
			return nil, err
		}
		dev.LastSeen = lastSeen
		devs = append(devs, dev)
	}
	return devs, rows.Err()
}
```

Add `UpdateDeviceMatterCreds` after the existing update functions:

```go
// UpdateDeviceMatterCreds stores the Matter commissioning discriminator and passcode.
func (d *Database) UpdateDeviceMatterCreds(id string, discrim uint16, passcode uint32) error {
	_, err := d.DB.Exec(
		`UPDATE devices SET matter_discrim = ?, matter_passcode = ? WHERE id = ?`,
		discrim, passcode, id)
	return err
}
```

- [ ] **Step 6: Run test to verify it passes**

```bash
go test ./internal/db/... -run TestDevice_UpdateMatterCreds -v
```

Expected: PASS

- [ ] **Step 7: Run all DB tests**

```bash
go test ./internal/db/... -v
```

Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add internal/db/schema.sql internal/db/db.go internal/db/device.go internal/db/db_test.go
git commit -m "feat: store Matter discrim+passcode in devices table"
```

---

## Task 2: Matter QR payload computation

**Files:**
- Create: `internal/matter/payload.go`
- Create: `internal/matter/payload_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/matter/payload_test.go`:

```go
package matter_test

import (
	"strings"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/matter"
	"github.com/stretchr/testify/assert"
)

func TestSetupQRPayload_Format(t *testing.T) {
	payload := matter.SetupQRPayload(3840, 20202021)
	assert.True(t, strings.HasPrefix(payload, "MT:"), "must start with MT:")
	// 3 prefix + 11 Base38 chars = 14 total
	assert.Equal(t, 14, len(payload), "MT: payload must be 14 chars (3 prefix + 11 Base38)")
}

func TestSetupQRPayload_Deterministic(t *testing.T) {
	a := matter.SetupQRPayload(1234, 56789012)
	b := matter.SetupQRPayload(1234, 56789012)
	assert.Equal(t, a, b, "same inputs must produce same output")
}

func TestSetupQRPayload_DifferentInputsDifferentOutput(t *testing.T) {
	a := matter.SetupQRPayload(1234, 56789012)
	b := matter.SetupQRPayload(5678, 56789012)
	assert.NotEqual(t, a, b, "different discriminators must produce different payloads")
}

func TestSetupQRPayload_OnlyBase38Chars(t *testing.T) {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-."
	payload := matter.SetupQRPayload(2048, 12345678)
	for _, ch := range payload[3:] { // skip "MT:" prefix
		assert.Contains(t, alphabet, string(ch), "char %q not in Base38 alphabet", ch)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/matter/... -v
```

Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Implement the Matter payload package**

Create `internal/matter/payload.go`:

```go
package matter

// base38Chars is the Matter Base38 encoding alphabet (38 characters).
const base38Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ-."

// base38Encode encodes data using Matter's Base38 scheme.
// Every 2 bytes produce 3 characters; a trailing single byte produces 2.
func base38Encode(data []byte) string {
	out := make([]byte, 0, (len(data)*3+1)/2)
	for i := 0; i+1 < len(data); i += 2 {
		val := uint(data[i]) | uint(data[i+1])<<8
		out = append(out,
			base38Chars[val%38],
			base38Chars[(val/38)%38],
			base38Chars[val/38/38],
		)
	}
	if len(data)%2 != 0 {
		val := uint(data[len(data)-1])
		out = append(out, base38Chars[val%38], base38Chars[val/38])
	}
	return string(out)
}

// SetupQRPayload returns the "MT:XXXX" Matter setup QR payload string.
// discriminator is 12-bit (0–4095); passcode is 27-bit (1–99999998).
// Uses standard commissioning flow with WiFi discovery capability.
func SetupQRPayload(discriminator uint16, passcode uint32) string {
	const (
		version    = 0 // 3 bits: spec version 0
		commFlow   = 0 // 2 bits: 0 = standard commissioning flow
		rendezvous = 4 // 10 bits: bit 2 set = WiFi onboarding
	)
	// Pack fields into a 54-bit little-endian integer per Matter core spec §5.1.3.1
	raw := uint64(version) |
		uint64(commFlow)<<3 |
		uint64(rendezvous)<<5 |
		uint64(discriminator&0x0FFF)<<15 |
		uint64(passcode&0x07FFFFFF)<<27
	var buf [7]byte // 56 bits, last 2 bits unused
	for i := range buf {
		buf[i] = byte(raw >> (8 * i))
	}
	return "MT:" + base38Encode(buf[:])
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/matter/... -v
```

Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/matter/payload.go internal/matter/payload_test.go
git commit -m "feat: Matter setup QR payload (Base38) generator"
```

---

## Task 3: Wire discriminator/passcode into flash paths

**Files:**
- Modify: `internal/flash/orchestrator.go`
- Modify: `internal/api/webflash.go`

- [ ] **Step 1: Update server flash orchestrator**

In `internal/flash/orchestrator.go`, the `Run` function currently calls `database.CreateDevice(...)` and returns. After the `CreateDevice` call (around line 93), add `UpdateDeviceMatterCreds`:

```go
	// 8. Register device in DB
	if err := database.CreateDevice(db.Device{
		ID:         chip.DeviceID,
		Name:       req.DeviceName,
		TemplateID: req.Template.ID,
		FWVersion:  req.FWVersion,
		PSK:        psk,
		Status:     "unknown",
	}); err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("register device: %w", err)}
	}

	// 9. Persist Matter commissioning credentials
	if err := database.UpdateDeviceMatterCreds(chip.DeviceID, discrim, passcode); err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("save matter creds: %w", err)}
	}

	return Result{DeviceID: chip.DeviceID, Name: req.DeviceName}
```

- [ ] **Step 2: Update browser flash to return pairing info in response**

In `internal/api/webflash.go`, add the `matter` import and change the `prepareWebFlash` response.

Add import (in the import block):
```go
"github.com/karthangar/matteresp32hub/internal/matter"
```

Change the final JSON encode (currently `json.NewEncoder(w).Encode(map[string]string{"token": token})`) to:

```go
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token":        token,
			"discriminator": discrim,
			"passcode":     passcode,
			"qr_payload":  matter.SetupQRPayload(discrim, passcode),
		})
```

- [ ] **Step 3: Build to verify no compilation errors**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
/usr/local/go/bin/go build ./...
```

Expected: clean build, no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/flash/orchestrator.go internal/api/webflash.go
git commit -m "feat: persist and return Matter commissioning credentials"
```

---

## Task 4: API pairing endpoint

**Files:**
- Modify: `internal/api/devices.go`
- Modify: `internal/api/router.go`
- Modify: `internal/api/devices_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/api/devices_test.go`:

```go
func TestDevices_GetPairing(t *testing.T) {
	srv := newTestServer(t)
	database := getDatabase(t, srv)

	require.NoError(t, database.CreateTemplate(db.TemplateRow{
		ID: "tpl-1", Name: "T1", Board: "esp32-c3", YAMLBody: "id: tpl-1\n",
	}))
	require.NoError(t, database.CreateDevice(db.Device{
		ID: "dev-1", Name: "1/Bedroom", TemplateID: "tpl-1", PSK: make([]byte, 32),
	}))
	require.NoError(t, database.UpdateDeviceMatterCreds("dev-1", 3840, 20202021))

	req := httptest.NewRequest(http.MethodGet, "/api/devices/dev-1/pairing", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, float64(3840), body["discriminator"])
	assert.Equal(t, float64(20202021), body["passcode"])
	qr, _ := body["qr_payload"].(string)
	assert.True(t, strings.HasPrefix(qr, "MT:"), "qr_payload must start with MT:")
}

func TestDevices_GetPairing_NotFound(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/devices/missing/pairing", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
```

Add `"strings"` to the import block in `devices_test.go` if not present.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/api/... -run TestDevices_GetPairing -v
```

Expected: FAIL — route not registered yet.

- [ ] **Step 3: Add the handler in devices.go**

Add to `internal/api/devices.go`:

```go
import (
    // existing imports...
    "github.com/karthangar/matteresp32hub/internal/matter"
)

func getPairingInfo(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dev, err := database.GetDevice(id)
		if err != nil {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"discriminator": dev.MatterDiscrim,
			"passcode":      dev.MatterPasscode,
			"qr_payload":   matter.SetupQRPayload(dev.MatterDiscrim, dev.MatterPasscode),
		})
	}
}
```

- [ ] **Step 4: Register the route in router.go**

In `internal/api/router.go`, inside the `devicesRouter` function, add:

```go
func devicesRouter(database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", listDevices(database))
		r.Get("/{id}", getDevice(database))
		r.Get("/{id}/pairing", getPairingInfo(database))
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/api/... -run TestDevices_GetPairing -v
```

Expected: both PASS.

- [ ] **Step 6: Run all API tests**

```bash
go test ./internal/api/... -v
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/api/devices.go internal/api/router.go internal/api/devices_test.go
git commit -m "feat: GET /api/devices/{id}/pairing endpoint"
```

---

## Task 5: Frontend — Fleet Pair button + Flash done screen QR

**Files:**
- Modify: `web/package.json`
- Modify: `web/src/views/Fleet.svelte`
- Modify: `web/src/views/Flash.svelte`

- [ ] **Step 1: Add qrcode npm package**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller/web
npm install qrcode@^1.5.3
```

Expected: `qrcode` and `@types/qrcode` (if installed) appear in `node_modules/`.

- [ ] **Step 2: Add Pair modal to Fleet.svelte**

Read `web/src/views/Fleet.svelte` first to find the exact insertion points.

Add to the `<script>` block (at the top, with other imports):

```js
import QRCode from 'qrcode';
```

Add state variables (after other `let` declarations):

```js
let pairModal = null; // { discriminator, passcode, qr_payload } | null
let qrDataUrl = '';

async function openPairModal(device) {
  const res = await fetch(`/api/devices/${device.id}/pairing`).then(r => r.json());
  pairModal = res;
  qrDataUrl = await QRCode.toDataURL(res.qr_payload, { width: 220, margin: 2 });
}
function closePairModal() { pairModal = null; qrDataUrl = ''; }
```

Add a "Pair" button to each device row in the table. Find the row actions area (wherever the existing row buttons/links are) and add:

```html
<button class="btn btn-xs btn-outline" on:click={() => openPairModal(d)}>Pair</button>
```

Add the modal at the bottom of the template (before the closing `</div>` of the root element):

```html
{#if pairModal}
  <button class="fixed inset-0 z-40 bg-black/60 cursor-default" on:click={closePairModal} />
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="bg-base-200 rounded-xl shadow-xl w-full max-w-sm flex flex-col">
      <div class="flex items-center justify-between px-5 py-3 border-b border-base-300">
        <span class="font-semibold text-sm">Commission device</span>
        <button class="btn btn-ghost btn-xs" on:click={closePairModal}>✕</button>
      </div>
      <div class="flex flex-col items-center gap-4 p-5">
        <p class="text-xs text-base-content/60 text-center">
          Scan with Apple Home, Google Home, or any Matter controller.<br>
          Device must be in commissioning mode (first boot after flash).
        </p>
        {#if qrDataUrl}
          <img src={qrDataUrl} alt="Matter QR code" class="rounded-lg border border-base-300" width="220" height="220" />
        {:else}
          <span class="loading loading-spinner loading-md"></span>
        {/if}
        <div class="w-full text-xs font-mono bg-base-300 rounded p-3 space-y-1">
          <div><span class="text-base-content/50">Discriminator:</span> {pairModal.discriminator}</div>
          <div><span class="text-base-content/50">Passcode:</span> {pairModal.passcode}</div>
          <div class="break-all"><span class="text-base-content/50">QR payload:</span> {pairModal.qr_payload}</div>
        </div>
      </div>
      <div class="px-5 pb-4 flex justify-end">
        <button class="btn btn-ghost btn-sm" on:click={closePairModal}>Close</button>
      </div>
    </div>
  </div>
{/if}
```

- [ ] **Step 3: Add pairing info to Browser Flash done screen**

In `web/src/views/Flash.svelte`, find the state variables for browser flash and add:

```js
let bfPairing = null; // { discriminator, passcode, qr_payload } | null
let bfQrDataUrl = '';
```

Find the `bfDoFlash` function (or wherever `prepareWebFlash` response is handled — it sets `browserFlashState = 'done'`). After the fetch, extract pairing info and generate QR:

```js
// After: const data = await res.json();
bfPairing = { discriminator: data.discriminator, passcode: data.passcode, qr_payload: data.qr_payload };
bfQrDataUrl = await QRCode.toDataURL(data.qr_payload, { width: 180, margin: 2 });
```

In the `bfReset` function, clear pairing state:
```js
bfPairing = null;
bfQrDataUrl = '';
```

Find the `browserFlashState === 'done'` block in the template. After the existing success alert and Serial Debug hint, add:

```html
{#if bfPairing}
  <div class="alert alert-success mt-2">
    <div class="flex flex-col gap-3 w-full">
      <span class="font-semibold text-sm">Commission this device</span>
      <p class="text-xs">Scan with Apple Home or Google Home. Device must be unplugged and replugged first.</p>
      {#if bfQrDataUrl}
        <img src={bfQrDataUrl} alt="Matter QR code" class="rounded border border-base-300 self-center" width="180" />
      {/if}
      <div class="font-mono text-xs space-y-1">
        <div>Discriminator: {bfPairing.discriminator}</div>
        <div>Passcode: {bfPairing.passcode}</div>
      </div>
    </div>
  </div>
{/if}
```

- [ ] **Step 4: Build frontend**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller/web
npm run build
```

Expected: clean build, no errors or warnings about missing modules.

- [ ] **Step 5: Build Go embed**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
/usr/local/go/bin/go build ./...
```

Expected: clean build.

- [ ] **Step 6: Run all tests**

```bash
go test ./... 2>&1
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add web/package.json web/package-lock.json web/src/views/Fleet.svelte web/src/views/Flash.svelte
git commit -m "feat: Matter pairing QR in Fleet + Browser Flash done screen"
```

---

## Verification

1. **Go tests pass:** `go test ./...` — all green
2. **Frontend builds:** `cd web && npm run build` — no errors
3. **Go embed compiles:** `go build ./...` — no errors
4. **Manual test — Fleet Pair button:**
   - Server-flash a device (any template)
   - Open Fleet view → find the device row → click "Pair"
   - Modal opens with QR code image + discriminator + passcode numbers
   - QR payload starts with "MT:"
   - Close modal
5. **Manual test — Browser Flash done screen:**
   - Complete a Browser Flash wizard through to Done
   - Done screen shows QR code image + discriminator + passcode
6. **Manual test — commissioning:**
   - Flash a device
   - Unplug / replug
   - Open Apple Home → Add Accessory → scan QR code
   - Device should appear as "Color Temperature Light"
