# ESPHome Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add ESPHome as a second firmware path alongside Matter — user picks board + modules via component builder, hub assembles ESPHome YAML, compiles via Docker, flashes over USB, and registers the device in the fleet with live heartbeat status.

**Architecture:** Module YAML files gain an `esphome:` block alongside their existing `matter:` block. A new `internal/esphome` package assembles complete ESPHome YAML from a component list and compiles it via one-shot `docker run ghcr.io/esphome/esphome`. The flash wizard forks at step 1 (Matter vs ESPHome); initial device registration is USB flash time; ongoing fleet status comes from a 60s HTTP heartbeat embedded in the compiled firmware.

**Tech Stack:** Go (Docker Go SDK, chi, modernc SQLite, text/template), Svelte 4, DaisyUI, `ghcr.io/esphome/esphome` Docker image

**Spec:** `docs/superpowers/specs/2026-04-19-esphome-integration-design.md`

---

## File Structure

| File | Change |
|------|--------|
| `internal/db/schema.sql` | Modify devices DDL: nullable `template_id`, 3 new ESPHome columns |
| `internal/db/db.go` | Add ESPHome schema migration (table recreation for nullable template_id) |
| `internal/db/device.go` | Add Device fields; update CreateDevice, GetDevice, ListDevices |
| `internal/db/db_test.go` | Tests for ESPHome device creation + creds |
| `internal/yamldef/types.go` | Add `ESPHomeDef`, `ESPHomeComponent` types; add `ESPHome *ESPHomeDef` to `Module` |
| `internal/yamldef/module.go` | ESPHome block optional in validation |
| `data/modules/gpio-switch.yaml` | Add `esphome:` block |
| `data/modules/bh1750.yaml` | Add `esphome:` block |
| `data/modules/analog-in.yaml` | Add `esphome:` block |
| `data/modules/wrgb-led.yaml` | Add `esphome:` block |
| `data/modules/dht22.yaml` | **New** — DHT22 temp+humidity module |
| `data/modules/bme280.yaml` | **New** — BME280 temp+humidity+pressure |
| `data/modules/neopixel.yaml` | **New** — NeoPixel/WS2812 strip |
| `data/modules/binary-input.yaml` | **New** — GPIO binary input |
| `internal/esphome/assembler.go` | **New** — assembles ESPHome YAML from component config |
| `internal/esphome/assembler_test.go` | **New** |
| `internal/esphome/builder.go` | **New** — Docker compile client |
| `internal/esphome/builder_test.go` | **New** |
| `internal/flash/esphome.go` | **New** — `FlashESPHomeDevice` orchestrator |
| `internal/api/flash.go` | Add `POST /api/flash/esphome` route + handler |
| `internal/api/devices.go` | Add heartbeat + ESPHome key handlers |
| `internal/api/router.go` | Register new routes |
| `internal/api/devices_test.go` | Heartbeat + ESPHome key tests |
| `internal/api/flash_test.go` | ESPHome flash endpoint test |
| `docker-compose.yml` | Add `esphome-cache` volume |
| `go.mod` / `go.sum` | Add Docker Go SDK dependency |
| `web/src/views/Flash.svelte` | ESPHome wizard path (steps 1-6) |
| `web/src/views/Fleet.svelte` | ESPHome device display (key button + reconfigure) |

---

## Task 1: DB Schema — ESPHome Columns + Nullable template_id

**Files:**
- Modify: `internal/db/schema.sql`
- Modify: `internal/db/db.go`
- Modify: `internal/db/device.go`
- Test: `internal/db/db_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/db/db_test.go` (after existing tests):

```go
func TestDevice_CreateESPHome(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	require.NoError(t, database.CreateDevice(db.Device{
		ID:            "dev-esph",
		Name:          "Kitchen Sensor",
		FirmwareType:  "esphome",
		ESPHomeConfig: `{"board":"esp32-c3","components":[]}`,
		PSK:           []byte{},
	}))

	got, err := database.GetDevice("dev-esph")
	require.NoError(t, err)
	assert.Equal(t, "esphome", got.FirmwareType)
	assert.Equal(t, `{"board":"esp32-c3","components":[]}`, got.ESPHomeConfig)
	assert.Equal(t, "", got.TemplateID)
	assert.Equal(t, "matter", db.Device{}.FirmwareType) // zero value check via separate device
}

func TestDevice_ESPHomeShownInList(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	require.NoError(t, database.CreateDevice(db.Device{
		ID: "dev-esph", Name: "Sensor", FirmwareType: "esphome", PSK: []byte{},
	}))

	devs, err := database.ListDevices()
	require.NoError(t, err)
	require.Len(t, devs, 1)
	assert.Equal(t, "esphome", devs[0].FirmwareType)
}

func TestDevice_ESPHomeAPIKey(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer database.Close()

	require.NoError(t, database.CreateDevice(db.Device{
		ID: "dev-esph", Name: "Sensor", FirmwareType: "esphome",
		ESPHomeAPIKey: "apikey123",
		ESPHomeConfig: `{"ota_password":"otapass"}`,
		PSK:           []byte{},
	}))

	got, err := database.GetDevice("dev-esph")
	require.NoError(t, err)
	assert.Equal(t, "apikey123", got.ESPHomeAPIKey)
	assert.Equal(t, `{"ota_password":"otapass"}`, got.ESPHomeConfig)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
/usr/local/go/bin/go test ./internal/db/... -run "TestDevice_CreateESPHome|TestDevice_ESPHomeShownInList|TestDevice_ESPHomeAPIKey" -v 2>&1
```

Expected: FAIL — `FirmwareType` field does not exist yet.

- [ ] **Step 3: Update schema.sql DDL**

Replace the existing `CREATE TABLE IF NOT EXISTS devices` block in `internal/db/schema.sql`:

```sql
CREATE TABLE IF NOT EXISTS devices (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    template_id     TEXT,                    -- nullable: NULL for ESPHome devices
    fw_version      TEXT NOT NULL DEFAULT '',
    psk             BLOB NOT NULL DEFAULT x'',
    status          TEXT NOT NULL DEFAULT 'unknown',
    last_seen       DATETIME,
    ip              TEXT NOT NULL DEFAULT '',
    matter_discrim  INTEGER NOT NULL DEFAULT 0,
    matter_passcode INTEGER NOT NULL DEFAULT 0,
    firmware_type   TEXT NOT NULL DEFAULT 'matter',
    esphome_config  TEXT NOT NULL DEFAULT '',
    esphome_api_key TEXT NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

- [ ] **Step 4: Add migration in db.go**

In `internal/db/db.go`, after the existing ALTER TABLE loop, add the ESPHome migration block:

```go
	// ESPHome migration: make template_id nullable + add ESPHome columns.
	// Detect by checking whether firmware_type column exists.
	var fwTypeCount int
	sqldb.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('devices') WHERE name='firmware_type'`).Scan(&fwTypeCount)
	if fwTypeCount == 0 {
		stmts := []string{
			`PRAGMA foreign_keys=OFF`,
			`CREATE TABLE devices_v2 (
				id              TEXT PRIMARY KEY,
				name            TEXT NOT NULL,
				template_id     TEXT,
				fw_version      TEXT NOT NULL DEFAULT '',
				psk             BLOB NOT NULL DEFAULT x'',
				status          TEXT NOT NULL DEFAULT 'unknown',
				last_seen       DATETIME,
				ip              TEXT NOT NULL DEFAULT '',
				matter_discrim  INTEGER NOT NULL DEFAULT 0,
				matter_passcode INTEGER NOT NULL DEFAULT 0,
				firmware_type   TEXT NOT NULL DEFAULT 'matter',
				esphome_config  TEXT NOT NULL DEFAULT '',
				esphome_api_key TEXT NOT NULL DEFAULT '',
				created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
			`INSERT INTO devices_v2 (id, name, template_id, fw_version, psk, status, last_seen, ip, matter_discrim, matter_passcode, created_at)
			 SELECT id, name, template_id, fw_version, psk, status, last_seen, ip, matter_discrim, matter_passcode, created_at FROM devices`,
			`DROP TABLE devices`,
			`ALTER TABLE devices_v2 RENAME TO devices`,
			`CREATE INDEX IF NOT EXISTS idx_devices_name ON devices(name)`,
			`PRAGMA foreign_keys=ON`,
		}
		for _, s := range stmts {
			if _, err := sqldb.Exec(s); err != nil {
				sqldb.Close()
				return nil, fmt.Errorf("ESPHome migration (%q): %w", s, err)
			}
		}
	}
```

- [ ] **Step 5: Update Device struct in device.go**

Replace the `Device` struct:

```go
type Device struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	TemplateID     string     `json:"template_id"`
	FWVersion      string     `json:"fw_version"`
	PSK            []byte     `json:"-"`
	Status         string     `json:"status"`
	LastSeen       *time.Time `json:"last_seen"`
	IP             string     `json:"ip"`
	MatterDiscrim  uint16     `json:"-"`
	MatterPasscode uint32     `json:"-"`
	FirmwareType   string     `json:"firmware_type"`
	ESPHomeConfig  string     `json:"-"`
	ESPHomeAPIKey  string     `json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
}
```

- [ ] **Step 6: Update CreateDevice, GetDevice, ListDevices in device.go**

Replace `CreateDevice`:

```go
func (d *Database) CreateDevice(dev Device) error {
	ft := dev.FirmwareType
	if ft == "" {
		ft = "matter"
	}
	var templateID interface{}
	if dev.TemplateID != "" {
		templateID = dev.TemplateID
	}
	_, err := d.DB.Exec(
		`INSERT INTO devices (id, name, template_id, fw_version, psk, status,
		        matter_discrim, matter_passcode, firmware_type, esphome_config, esphome_api_key)
		 VALUES (?, ?, ?, ?, ?, 'unknown', ?, ?, ?, ?, ?)`,
		dev.ID, dev.Name, templateID, dev.FWVersion, dev.PSK,
		dev.MatterDiscrim, dev.MatterPasscode,
		ft, dev.ESPHomeConfig, dev.ESPHomeAPIKey)
	return err
}
```

Replace `GetDevice`:

```go
func (d *Database) GetDevice(id string) (Device, error) {
	row := d.DB.QueryRow(
		`SELECT id, name, COALESCE(template_id,''), fw_version, psk, status, last_seen, ip,
		        matter_discrim, matter_passcode, firmware_type, esphome_config, esphome_api_key, created_at
		 FROM devices WHERE id = ?`, id)
	var dev Device
	var lastSeen *time.Time
	if err := row.Scan(&dev.ID, &dev.Name, &dev.TemplateID, &dev.FWVersion,
		&dev.PSK, &dev.Status, &lastSeen, &dev.IP,
		&dev.MatterDiscrim, &dev.MatterPasscode,
		&dev.FirmwareType, &dev.ESPHomeConfig, &dev.ESPHomeAPIKey, &dev.CreatedAt); err != nil {
		return Device{}, err
	}
	dev.LastSeen = lastSeen
	return dev, nil
}
```

Replace `ListDevices`:

```go
func (d *Database) ListDevices() ([]Device, error) {
	rows, err := d.DB.Query(
		`SELECT id, name, COALESCE(template_id,''), fw_version, psk, status, last_seen, ip,
		        matter_discrim, matter_passcode, firmware_type, esphome_config, esphome_api_key, created_at
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
			&dev.MatterDiscrim, &dev.MatterPasscode,
			&dev.FirmwareType, &dev.ESPHomeConfig, &dev.ESPHomeAPIKey, &dev.CreatedAt); err != nil {
			return nil, err
		}
		dev.LastSeen = lastSeen
		devs = append(devs, dev)
	}
	return devs, rows.Err()
}
```

- [ ] **Step 7: Run new tests**

```bash
/usr/local/go/bin/go test ./internal/db/... -run "TestDevice_CreateESPHome|TestDevice_ESPHomeShownInList|TestDevice_ESPHomeAPIKey" -v 2>&1
```

Expected: PASS

- [ ] **Step 8: Run all DB tests**

```bash
/usr/local/go/bin/go test ./internal/db/... -v 2>&1
```

Expected: all PASS

- [ ] **Step 9: Commit**

```bash
git add internal/db/schema.sql internal/db/db.go internal/db/device.go internal/db/db_test.go
git commit -m "feat: ESPHome — DB schema (nullable template_id, firmware_type, esphome columns)"
```

---

## Task 2: yamldef Module ESPHome Types + Module YAML Files

**Files:**
- Modify: `internal/yamldef/types.go`
- Modify: `internal/yamldef/module.go`
- Modify: `data/modules/gpio-switch.yaml`
- Modify: `data/modules/bh1750.yaml`
- Modify: `data/modules/analog-in.yaml`
- Modify: `data/modules/wrgb-led.yaml`
- Create: `data/modules/dht22.yaml`
- Create: `data/modules/bme280.yaml`
- Create: `data/modules/neopixel.yaml`
- Create: `data/modules/binary-input.yaml`
- Test: `internal/yamldef/module_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/yamldef/module_test.go`:

```go
func TestParseModule_ESPHomeBlock(t *testing.T) {
	yaml := []byte(`
id: test-gpio
name: "Test GPIO"
version: "1.0"
category: io
io:
  - id: OUT
    type: digital_out
    label: "Output"
    constraints:
      digital: {active: high, initial_state: low}
matter:
  endpoint_type: on_off_light
  behaviors: [on_off]
esphome:
  components:
    - domain: switch
      template: "platform: gpio\npin: \"{OUT}\"\nname: \"{NAME}\""
`)
	mod, err := ParseModule(yaml)
	require.NoError(t, err)
	require.NotNil(t, mod.ESPHome)
	require.Len(t, mod.ESPHome.Components, 1)
	assert.Equal(t, "switch", mod.ESPHome.Components[0].Domain)
	assert.Contains(t, mod.ESPHome.Components[0].Template, "{OUT}")
}

func TestParseModule_NoESPHomeIsValid(t *testing.T) {
	yaml := []byte(`
id: gpio-switch
name: "GPIO Switch"
version: "1.0"
category: io
io:
  - id: OUT
    type: digital_out
    label: "Output"
    constraints:
      digital: {active: high, initial_state: low}
matter:
  endpoint_type: on_off_light
  behaviors: [on_off]
`)
	mod, err := ParseModule(yaml)
	require.NoError(t, err)
	assert.Nil(t, mod.ESPHome)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
/usr/local/go/bin/go test ./internal/yamldef/... -run "TestParseModule_ESPHomeBlock|TestParseModule_NoESPHomeIsValid" -v 2>&1
```

Expected: FAIL — `mod.ESPHome` field does not exist yet.

- [ ] **Step 3: Add ESPHome types to types.go**

In `internal/yamldef/types.go`, add after the existing `MatterDef` struct:

```go
// ESPHomeComponent is one ESPHome component block inside a module's esphome: section.
// Domain maps to the top-level ESPHome YAML key (e.g. "sensor", "switch", "light").
// Template is raw YAML with {PIN_ROLE} and {NAME} placeholders substituted at assembly time.
type ESPHomeComponent struct {
	Domain   string `yaml:"domain"`
	Template string `yaml:"template"`
}

// ESPHomeDef is the esphome: block in a module YAML.
type ESPHomeDef struct {
	Components []ESPHomeComponent `yaml:"components"`
}
```

Add `ESPHome *ESPHomeDef` field to the `Module` struct, after the `Matter MatterDef` field:

```go
	Matter      MatterDef    `yaml:"matter"`
	ESPHome     *ESPHomeDef  `yaml:"esphome,omitempty"`
	Measurement *Measurement `yaml:"measurement,omitempty"`
```

- [ ] **Step 4: Update module.go validation — ESPHome block is optional**

In `internal/yamldef/module.go`, read the `validateModule` function. The ESPHome block requires no additional validation beyond YAML parsing (domain and template are free-form strings). Confirm that the existing validation does not reject unknown top-level fields (it shouldn't since Go's yaml.v3 ignores unknown fields by default). No code change needed in module.go — the `omitempty` pointer handles optional presence.

If there is an existing `if mod.ESPHome != nil { ... }` check, skip adding it. If there is none, verify that `ParseModule` successfully returns a module with a nil ESPHome field when the block is absent.

- [ ] **Step 5: Run tests to verify they pass**

```bash
/usr/local/go/bin/go test ./internal/yamldef/... -run "TestParseModule_ESPHomeBlock|TestParseModule_NoESPHomeIsValid" -v 2>&1
```

Expected: PASS

- [ ] **Step 6: Add esphome block to existing module YAMLs**

**`data/modules/gpio-switch.yaml`** — append at end:

```yaml
esphome:
  components:
    - domain: switch
      template: "platform: gpio\n  pin: \"{OUT}\"\n  name: \"{NAME}\""
```

**`data/modules/bh1750.yaml`** — append at end:

```yaml
esphome:
  components:
    - domain: i2c
      template: "sda: \"{SDA}\"\n  scl: \"{SCL}\"\n  id: i2c_bus_{NAME}"
    - domain: sensor
      template: "platform: bh1750\n  name: \"{NAME} Illuminance\"\n  i2c_id: i2c_bus_{NAME}\n  update_interval: 10s"
```

**`data/modules/analog-in.yaml`** — append at end:

```yaml
esphome:
  components:
    - domain: sensor
      template: "platform: adc\n  pin: \"{SIG}\"\n  name: \"{NAME}\"\n  update_interval: 10s\n  attenuation: 11db"
```

**`data/modules/wrgb-led.yaml`** — append at end:

```yaml
esphome:
  components:
    - domain: light
      template: "platform: rgbw\n  name: \"{NAME}\"\n  red:\n    platform: ledc\n    pin: \"{R}\"\n    id: led_r_{NAME}\n  green:\n    platform: ledc\n    pin: \"{G}\"\n    id: led_g_{NAME}\n  blue:\n    platform: ledc\n    pin: \"{B}\"\n    id: led_b_{NAME}\n  white:\n    platform: ledc\n    pin: \"{W}\"\n    id: led_w_{NAME}"
```

- [ ] **Step 7: Create new module YAML files**

**`data/modules/dht22.yaml`**:

```yaml
id: dht22
name: "DHT22 Temperature & Humidity"
version: "1.0"
category: sensor
io:
  - id: DATA
    type: digital_out
    label: "Data pin"
    constraints:
      digital: {active: high, initial_state: low}
matter:
  endpoint_type: temperature_sensor
  behaviors: [temperature_reporting]
esphome:
  components:
    - domain: sensor
      template: "platform: dht\n  model: DHT22\n  pin: \"{DATA}\"\n  temperature:\n    name: \"{NAME} Temperature\"\n  humidity:\n    name: \"{NAME} Humidity\"\n  update_interval: 30s"
```

**`data/modules/bme280.yaml`**:

```yaml
id: bme280
name: "BME280 Temp/Humidity/Pressure"
version: "1.0"
category: sensor
io:
  - id: SDA
    type: i2c_data
    label: "I2C Data"
    constraints:
      i2c: {speed: 400000, pullup: internal}
  - id: SCL
    type: i2c_clock
    label: "I2C Clock"
    constraints:
      i2c: {speed: 400000, pullup: internal}
matter:
  endpoint_type: temperature_sensor
  behaviors: [temperature_reporting]
esphome:
  components:
    - domain: i2c
      template: "sda: \"{SDA}\"\n  scl: \"{SCL}\"\n  id: i2c_bus_{NAME}"
    - domain: sensor
      template: "platform: bme280_i2c\n  i2c_id: i2c_bus_{NAME}\n  temperature:\n    name: \"{NAME} Temperature\"\n  humidity:\n    name: \"{NAME} Humidity\"\n  pressure:\n    name: \"{NAME} Pressure\"\n  update_interval: 30s"
```

**`data/modules/neopixel.yaml`**:

```yaml
id: neopixel
name: "NeoPixel / WS2812 Strip"
version: "1.0"
category: driver
io:
  - id: DATA
    type: digital_out
    label: "Data pin"
    constraints:
      digital: {active: high, initial_state: low}
matter:
  endpoint_type: extended_color_light
  behaviors: [color_control, brightness]
esphome:
  components:
    - domain: light
      template: "platform: neopixelbus\n  type: GRB\n  variant: WS2812\n  pin: \"{DATA}\"\n  num_leds: 30\n  name: \"{NAME}\""
```

**`data/modules/binary-input.yaml`**:

```yaml
id: binary-input
name: "Binary Input (button / door sensor)"
version: "1.0"
category: io
io:
  - id: PIN
    type: digital_in
    label: "Input pin"
    constraints:
      digital: {active: low, initial_state: low}
matter:
  endpoint_type: contact_sensor
  behaviors: [boolean_state]
esphome:
  components:
    - domain: binary_sensor
      template: "platform: gpio\n  pin:\n    number: \"{PIN}\"\n    mode: INPUT_PULLUP\n    inverted: true\n  name: \"{NAME}\""
```

- [ ] **Step 8: Run all yamldef tests**

```bash
/usr/local/go/bin/go test ./internal/yamldef/... -v 2>&1
```

Expected: all PASS (new module files are loaded from embedded FS and must parse cleanly)

- [ ] **Step 9: Commit**

```bash
git add internal/yamldef/types.go internal/yamldef/module.go \
    data/modules/gpio-switch.yaml data/modules/bh1750.yaml \
    data/modules/analog-in.yaml data/modules/wrgb-led.yaml \
    data/modules/dht22.yaml data/modules/bme280.yaml \
    data/modules/neopixel.yaml data/modules/binary-input.yaml
git commit -m "feat: ESPHome — module ESPHome blocks + new sensor/actuator modules"
```

---

## Task 3: ESPHome YAML Assembler

**Files:**
- Create: `internal/esphome/assembler.go`
- Create: `internal/esphome/assembler_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/esphome/assembler_test.go`:

```go
package esphome_test

import (
	"testing"

	"github.com/karthangar/matteresp32hub/internal/esphome"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testModules() map[string]*yamldef.Module {
	return map[string]*yamldef.Module{
		"dht22": {
			ESPHome: &yamldef.ESPHomeDef{
				Components: []yamldef.ESPHomeComponent{
					{Domain: "sensor", Template: "platform: dht\n  model: DHT22\n  pin: \"{DATA}\"\n  temperature:\n    name: \"{NAME} Temperature\"\n  humidity:\n    name: \"{NAME} Humidity\"\n  update_interval: 30s"},
				},
			},
		},
		"relay": {
			ESPHome: &yamldef.ESPHomeDef{
				Components: []yamldef.ESPHomeComponent{
					{Domain: "switch", Template: "platform: gpio\n  pin: \"{OUT}\"\n  name: \"{NAME}\""},
				},
			},
		},
	}
}

func TestAssemble_ContainsHeader(t *testing.T) {
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "Kitchen Sensor", DeviceID: "aabbccddeeff",
		WiFiSSID: "MyNet", WiFiPassword: "s3cret",
		OTAPassword: "otapass", HubURL: "http://10.0.0.1:8080",
	}
	out, err := esphome.Assemble(cfg, testModules())
	require.NoError(t, err)
	assert.Contains(t, out, "esp32:")
	assert.Contains(t, out, "board: esp32-c3-devkitm-1")
	assert.Contains(t, out, `ssid: "MyNet"`)
	assert.Contains(t, out, "password: \"otapass\"")
	assert.Contains(t, out, "http://10.0.0.1:8080/api/devices/aabbccddeeff/heartbeat")
}

func TestAssemble_HAIntegration(t *testing.T) {
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "Sensor", DeviceID: "aabb",
		WiFiSSID: "net", WiFiPassword: "pass",
		OTAPassword: "otp", HubURL: "http://hub",
		HAIntegration: true, APIKey: "dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleTA=",
	}
	out, err := esphome.Assemble(cfg, testModules())
	require.NoError(t, err)
	assert.Contains(t, out, "api:")
	assert.Contains(t, out, "key: \"dGVzdGtleXRlc3RrZXl0ZXN0a2V5dGVzdGtleTA=\"")
}

func TestAssemble_NoHAIntegration(t *testing.T) {
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "Sensor", DeviceID: "aabb",
		WiFiSSID: "net", WiFiPassword: "pass",
		OTAPassword: "otp", HubURL: "http://hub",
		HAIntegration: false,
	}
	out, err := esphome.Assemble(cfg, testModules())
	require.NoError(t, err)
	assert.NotContains(t, out, "api:")
}

func TestAssemble_ComponentPinSubstitution(t *testing.T) {
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "k", DeviceID: "id",
		WiFiSSID: "n", WiFiPassword: "p", OTAPassword: "o", HubURL: "http://h",
		Components: []esphome.ComponentConfig{
			{Type: "dht22", Name: "Room Temp", Pins: map[string]string{"DATA": "GPIO4"}},
		},
	}
	out, err := esphome.Assemble(cfg, testModules())
	require.NoError(t, err)
	assert.Contains(t, out, "sensor:")
	assert.Contains(t, out, "pin: GPIO4")
	assert.Contains(t, out, "Room Temp Temperature")
	assert.NotContains(t, out, "{DATA}")
	assert.NotContains(t, out, "{NAME}")
}

func TestAssemble_MultipleComponentsSameDomain(t *testing.T) {
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "multi", DeviceID: "id",
		WiFiSSID: "n", WiFiPassword: "p", OTAPassword: "o", HubURL: "http://h",
		Components: []esphome.ComponentConfig{
			{Type: "dht22", Name: "Sensor 1", Pins: map[string]string{"DATA": "GPIO4"}},
			{Type: "dht22", Name: "Sensor 2", Pins: map[string]string{"DATA": "GPIO5"}},
		},
	}
	out, err := esphome.Assemble(cfg, testModules())
	require.NoError(t, err)
	assert.Contains(t, out, "GPIO4")
	assert.Contains(t, out, "GPIO5")
	assert.Contains(t, out, "Sensor 1 Temperature")
	assert.Contains(t, out, "Sensor 2 Temperature")
}

func TestAssemble_UnknownModuleError(t *testing.T) {
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "x", DeviceID: "y",
		WiFiSSID: "n", WiFiPassword: "p", OTAPassword: "o", HubURL: "http://h",
		Components: []esphome.ComponentConfig{
			{Type: "nonexistent", Name: "X", Pins: map[string]string{}},
		},
	}
	_, err := esphome.Assemble(cfg, testModules())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
/usr/local/go/bin/go test ./internal/esphome/... -v 2>&1
```

Expected: FAIL — package does not exist yet.

- [ ] **Step 3: Create assembler.go**

Create `internal/esphome/assembler.go`:

```go
package esphome

import (
	"fmt"
	"strings"

	"github.com/karthangar/matteresp32hub/internal/yamldef"
)

// ComponentConfig is one component in an ESPHome device.
type ComponentConfig struct {
	Type string            // module ID, e.g. "dht22"
	Name string            // user-facing label, e.g. "Room Temp"
	Pins map[string]string // pin role → GPIO, e.g. {"DATA": "GPIO4"}
}

// Config is the full set of parameters for assembling an ESPHome YAML.
type Config struct {
	Board         string            // e.g. "esp32-c3"
	DeviceName    string            // e.g. "Kitchen Sensor" → slug: "kitchen-sensor"
	DeviceID      string            // chip MAC, used in heartbeat URL
	WiFiSSID      string
	WiFiPassword  string
	HAIntegration bool
	APIKey        string // base64-encoded 32-byte key; required if HAIntegration == true
	OTAPassword   string // hex random bytes
	HubURL        string // e.g. "http://192.168.1.10:8080"
	Components    []ComponentConfig
}

// boardDef maps board IDs to ESPHome platform + board strings.
var boardDef = map[string][2]string{
	"esp32-c3": {"esp32", "esp32-c3-devkitm-1"},
	"esp32-h2": {"esp32", "esp32-h2-devkitm-1"},
	"esp32":    {"esp32", "esp32dev"},
	"esp32-s3": {"esp32", "esp32-s3-devkitc-1"},
}

func slug(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "-"))
}

// Assemble builds a complete ESPHome YAML string from cfg and the module library.
func Assemble(cfg Config, modules map[string]*yamldef.Module) (string, error) {
	bd, ok := boardDef[cfg.Board]
	if !ok {
		return "", fmt.Errorf("unsupported board: %q", cfg.Board)
	}
	platform, boardName := bd[0], bd[1]
	deviceSlug := slug(cfg.DeviceName)

	var sb strings.Builder

	// esphome header
	fmt.Fprintf(&sb, "esphome:\n  name: %s\n\n", deviceSlug)

	// platform block (esp32/esp8266)
	fmt.Fprintf(&sb, "%s:\n  board: %s\n  framework:\n    type: esp-idf\n\n", platform, boardName)

	// wifi
	fmt.Fprintf(&sb, "wifi:\n  ssid: %q\n  password: %q\n  ap:\n    ssid: %q\n    password: \"changeme\"\n\n",
		cfg.WiFiSSID, cfg.WiFiPassword, deviceSlug+"-fallback")

	// logger
	sb.WriteString("logger:\n\n")

	// ota
	fmt.Fprintf(&sb, "ota:\n  - platform: esphome\n    password: %q\n\n", cfg.OTAPassword)

	// api (HA integration only)
	if cfg.HAIntegration && cfg.APIKey != "" {
		fmt.Fprintf(&sb, "api:\n  encryption:\n    key: %q\n\n", cfg.APIKey)
	}

	// heartbeat
	fmt.Fprintf(&sb, "http_request:\n  useragent: MatterHub-ESPHome/1.0\n\n")
	fmt.Fprintf(&sb, "interval:\n  - interval: 60s\n    then:\n      - http_request.post:\n          url: %q\n\n",
		cfg.HubURL+"/api/devices/"+cfg.DeviceID+"/heartbeat")

	// components grouped by domain
	type entry struct{ domain, rendered string }
	var entries []entry
	domainSeen := map[string]bool{}
	var domainOrder []string

	for _, comp := range cfg.Components {
		mod, ok := modules[comp.Type]
		if !ok {
			return "", fmt.Errorf("module %q not found in library", comp.Type)
		}
		if mod.ESPHome == nil {
			return "", fmt.Errorf("module %q has no esphome: block", comp.Type)
		}
		for _, ec := range mod.ESPHome.Components {
			rendered := ec.Template
			rendered = strings.ReplaceAll(rendered, "{NAME}", comp.Name)
			for role, gpio := range comp.Pins {
				rendered = strings.ReplaceAll(rendered, "{"+role+"}", gpio)
			}
			entries = append(entries, entry{ec.Domain, rendered})
			if !domainSeen[ec.Domain] {
				domainSeen[ec.Domain] = true
				domainOrder = append(domainOrder, ec.Domain)
			}
		}
	}

	// group entries by domain preserving insertion order
	byDomain := map[string][]string{}
	for _, e := range entries {
		byDomain[e.domain] = append(byDomain[e.domain], e.rendered)
	}

	for _, domain := range domainOrder {
		sb.WriteString(domain + ":\n")
		for _, tmpl := range byDomain[domain] {
			lines := strings.Split(strings.TrimRight(tmpl, "\n"), "\n")
			fmt.Fprintf(&sb, "  - %s\n", lines[0])
			for _, line := range lines[1:] {
				if line == "" {
					sb.WriteByte('\n')
				} else {
					fmt.Fprintf(&sb, "    %s\n", line)
				}
			}
		}
		sb.WriteByte('\n')
	}

	return sb.String(), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
/usr/local/go/bin/go test ./internal/esphome/... -run "TestAssemble" -v 2>&1
```

Expected: all PASS

- [ ] **Step 5: Run all tests**

```bash
/usr/local/go/bin/go test ./... 2>&1
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/esphome/assembler.go internal/esphome/assembler_test.go
git commit -m "feat: ESPHome YAML assembler"
```

---

## Task 4: ESPHome Docker Build Client

**Files:**
- Modify: `go.mod` / `go.sum` (add Docker SDK)
- Create: `internal/esphome/builder.go`
- Create: `internal/esphome/builder_test.go`

- [ ] **Step 1: Add Docker Go SDK dependency**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
/usr/local/go/bin/go get github.com/docker/docker/client@latest
/usr/local/go/bin/go get github.com/docker/docker/api/types/container@latest
/usr/local/go/bin/go get github.com/docker/docker/api/types/mount@latest
/usr/local/go/bin/go mod tidy 2>&1
```

Expected: go.mod updated with `github.com/docker/docker` dependency.

- [ ] **Step 2: Write the failing test**

Create `internal/esphome/builder_test.go`:

```go
package esphome_test

import (
	"context"
	"os"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/esphome"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder_New(t *testing.T) {
	b, err := esphome.NewBuilder("/tmp/esphome-test-cache")
	require.NoError(t, err)
	assert.NotNil(t, b)
	b.Close()
}

func TestBuilder_Compile_Integration(t *testing.T) {
	if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
		t.Skip("Docker socket not available — skipping integration test")
	}
	b, err := esphome.NewBuilder(t.TempDir())
	require.NoError(t, err)
	defer b.Close()

	// Minimal valid ESPHome YAML for esp32-c3
	yaml := `
esphome:
  name: test-device

esp32:
  board: esp32-c3-devkitm-1
  framework:
    type: esp-idf

wifi:
  ssid: "TestNet"
  password: "testpass"
  ap:
    ssid: "test-fallback"
    password: "changeme"

logger:

ota:
  - platform: esphome
    password: "otapass"
`
	bin, err := b.Compile(context.Background(), "test-device", yaml, os.Stdout)
	require.NoError(t, err)
	assert.Greater(t, len(bin), 1000, "firmware binary should be > 1kB")
}
```

- [ ] **Step 3: Run the non-integration test to verify it fails**

```bash
/usr/local/go/bin/go test ./internal/esphome/... -run "TestBuilder_New" -v 2>&1
```

Expected: FAIL — `esphome.NewBuilder` does not exist yet.

- [ ] **Step 4: Create builder.go**

Create `internal/esphome/builder.go`:

```go
package esphome

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
)

const (
	esphomeImage   = "ghcr.io/esphome/esphome:latest"
	compileTimeout = 15 * time.Minute
)

// Builder compiles ESPHome YAML into firmware binaries using a one-shot Docker container.
type Builder struct {
	docker   *client.Client
	cacheDir string
}

// NewBuilder creates a Builder and connects to the local Docker daemon.
func NewBuilder(cacheDir string) (*Builder, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Builder{docker: cli, cacheDir: cacheDir}, nil
}

// Close releases the Docker client.
func (b *Builder) Close() { b.docker.Close() }

// Compile assembles the ESPHome YAML, compiles it in a Docker container, and returns
// the firmware-factory.bin bytes. Build logs are written to logWriter.
// deviceID is used as both the config directory name and the YAML device name.
func (b *Builder) Compile(ctx context.Context, deviceName string, yaml string, logWriter io.Writer) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, compileTimeout)
	defer cancel()

	devDir := filepath.Join(b.cacheDir, deviceName)
	if err := os.MkdirAll(devDir, 0755); err != nil {
		return nil, fmt.Errorf("create device dir: %w", err)
	}

	cfgPath := filepath.Join(devDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(yaml), 0644); err != nil {
		return nil, fmt.Errorf("write config: %w", err)
	}

	resp, err := b.docker.ContainerCreate(ctx,
		&container.Config{
			Image: esphomeImage,
			Cmd:   []string{"compile", "/config/" + deviceName + "/config.yaml"},
		},
		&container.HostConfig{
			Mounts: []mount.Mount{{
				Type:   mount.TypeBind,
				Source: b.cacheDir,
				Target: "/config",
			}},
			AutoRemove: true,
		},
		nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}

	if err := b.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("start container: %w", err)
	}

	// Stream logs
	logReader, err := b.docker.ContainerLogs(ctx, resp.ID, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Follow: true,
	})
	if err != nil {
		return nil, fmt.Errorf("attach logs: %w", err)
	}
	defer logReader.Close()
	io.Copy(logWriter, logReader) //nolint:errcheck

	// Wait for completion
	statusCh, errCh := b.docker.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return nil, fmt.Errorf("container wait: %w", err)
		}
	case status := <-statusCh:
		if status.StatusCode != 0 {
			return nil, fmt.Errorf("esphome compile failed (exit %d) — check build logs", status.StatusCode)
		}
	case <-ctx.Done():
		b.docker.ContainerStop(context.Background(), resp.ID, container.StopOptions{}) //nolint:errcheck
		return nil, fmt.Errorf("compile timed out after %s", compileTimeout)
	}

	// Read compiled binary
	binPath := filepath.Join(devDir, ".esphome", "build", deviceName, ".pioenvs", deviceName, "firmware-factory.bin")
	bin, err := os.ReadFile(binPath)
	if err != nil {
		return nil, fmt.Errorf("read firmware binary (%s): %w", binPath, err)
	}
	return bin, nil
}
```

- [ ] **Step 5: Run the non-integration test**

```bash
/usr/local/go/bin/go test ./internal/esphome/... -run "TestBuilder_New" -v 2>&1
```

Expected: PASS

- [ ] **Step 6: Build to verify compilation**

```bash
/usr/local/go/bin/go build ./... 2>&1
```

Expected: clean build.

- [ ] **Step 7: Commit**

```bash
git add internal/esphome/builder.go internal/esphome/builder_test.go go.mod go.sum
git commit -m "feat: ESPHome Docker build client"
```

---

## Task 5: ESPHome Flash Function + API Endpoint

**Files:**
- Create: `internal/flash/esphome.go`
- Modify: `internal/api/flash.go`
- Modify: `internal/api/router.go`
- Test: `internal/api/flash_test.go` (new file or add to existing)

- [ ] **Step 1: Write the failing test**

Create `internal/api/flash_test.go`:

```go
package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlash_ESPHomeEndpointExists(t *testing.T) {
	srv := newTestServer(t)
	body, _ := json.Marshal(map[string]interface{}{
		"port":           "/dev/ttyUSB0",
		"device_name":    "Test",
		"wifi_ssid":      "net",
		"wifi_password":  "pass",
		"hub_url":        "http://hub",
		"board":          "esp32-c3",
		"ha_integration": false,
		"components":     []interface{}{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/flash/esphome", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	// Port /dev/ttyUSB0 won't exist in test — expect 500 or 400, NOT 404
	assert.NotEqual(t, http.StatusNotFound, w.Code, "route must be registered")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
/usr/local/go/bin/go test ./internal/api/... -run "TestFlash_ESPHomeEndpointExists" -v 2>&1
```

Expected: FAIL — route returns 404 (not registered yet).

- [ ] **Step 3: Create internal/flash/esphome.go**

```go
package flash

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/karthangar/matteresp32hub/internal/esphome"
	"github.com/karthangar/matteresp32hub/internal/usb"
)

// ESPHomeRequest holds the parameters for an ESPHome flash operation.
type ESPHomeRequest struct {
	Port         string
	DeviceName   string
	WiFiSSID     string
	WiFiPassword string
	HubURL       string
	Board        string
	HAIntegration bool
	Components   []esphome.ComponentConfig
}

// FlashESPHomeDevice runs the full ESPHome flash sequence.
// Log lines from the ESPHome compiler are written to logWriter.
// modules is the map from module ID → *yamldef.Module, loaded from the library.
func FlashESPHomeDevice(database *db.Database, builder *esphome.Builder,
	modules map[string]*yamldef.Module, req ESPHomeRequest, logWriter io.Writer) Result {

	// 1. Read chip info
	chip, err := usb.GetChipInfo(req.Port)
	if err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("chip_id: %w", err)}
	}

	// 2. Generate credentials
	otaBuf := make([]byte, 16)
	if _, err := rand.Read(otaBuf); err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("generate OTA password: %w", err)}
	}
	otaPassword := hex.EncodeToString(otaBuf)

	var apiKey string
	if req.HAIntegration {
		keyBuf := make([]byte, 32)
		if _, err := rand.Read(keyBuf); err != nil {
			return Result{Name: req.DeviceName, Error: fmt.Errorf("generate API key: %w", err)}
		}
		apiKey = base64.StdEncoding.EncodeToString(keyBuf)
	}

	// 3. Assemble ESPHome YAML
	cfg := esphome.Config{
		Board:         req.Board,
		DeviceName:    req.DeviceName,
		DeviceID:      chip.DeviceID,
		WiFiSSID:      req.WiFiSSID,
		WiFiPassword:  req.WiFiPassword,
		HAIntegration: req.HAIntegration,
		APIKey:        apiKey,
		OTAPassword:   otaPassword,
		HubURL:        req.HubURL,
		Components:    req.Components,
	}
	yaml, err := esphome.Assemble(cfg, modules)
	if err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("assemble YAML: %w", err)}
	}

	// 4. Compile
	bin, err := builder.Compile(req.Context(), req.DeviceName, yaml, logWriter)
	if err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("compile: %w", err)}
	}

	// 5. Write binary to temp file and flash at 0x0
	tmpDir, err := os.MkdirTemp("", "esphome-flash-*")
	if err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("temp dir: %w", err)}
	}
	defer os.RemoveAll(tmpDir)

	binPath := filepath.Join(tmpDir, "firmware-factory.bin")
	if err := os.WriteFile(binPath, bin, 0644); err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("write firmware: %w", err)}
	}
	if err := usb.WriteFlash(req.Port, binPath, 0x0); err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("flash firmware: %w", err)}
	}

	// 6. Persist ESPHome config as JSON
	esphomeCfg := struct {
		Board         string                    `json:"board"`
		HAIntegration bool                      `json:"ha_integration"`
		OTAPassword   string                    `json:"ota_password"`
		HubURL        string                    `json:"hub_url"`
		Components    []esphome.ComponentConfig `json:"components"`
	}{
		Board: req.Board, HAIntegration: req.HAIntegration,
		OTAPassword: otaPassword, HubURL: req.HubURL, Components: req.Components,
	}
	cfgJSON, err := json.Marshal(esphomeCfg)
	if err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("marshal config: %w", err)}
	}

	// 7. Register device in DB
	if err := database.CreateDevice(db.Device{
		ID:            chip.DeviceID,
		Name:          req.DeviceName,
		FirmwareType:  "esphome",
		ESPHomeConfig: string(cfgJSON),
		ESPHomeAPIKey: apiKey,
		PSK:           []byte{},
	}); err != nil {
		return Result{Name: req.DeviceName, Error: fmt.Errorf("register device: %w", err)}
	}

	return Result{DeviceID: chip.DeviceID, Name: req.DeviceName}
}
```

Note: `ESPHomeRequest` needs a `Context()` method or a `ctx context.Context` field. Add `ctx context.Context` to the struct and update the call in the API handler accordingly. Simplest approach — add it as a field:

```go
type ESPHomeRequest struct {
	Ctx          context.Context // set by the API handler
	Port         string
	// ...
}
```

And replace `req.Context()` with `req.Ctx`. Add `"context"` to imports.

Also add `"github.com/karthangar/matteresp32hub/internal/yamldef"` to the imports since `modules map[string]*yamldef.Module` is used.

- [ ] **Step 4: Add POST /api/flash/esphome to flash.go**

Read `internal/api/flash.go` to find the `flashRouter` function. Add the new route and handler. The handler must:
1. Decode JSON request
2. Load modules from library
3. Initialize Builder with cache dir from `DATA_DIR` env
4. Call `flash.FlashESPHomeDevice` with a log writer that writes newline-delimited JSON to the response
5. Flush after each log line (chunked transfer)

Add to `internal/api/flash.go`:

```go
import (
	// add to existing imports:
	"bufio"
	"context"
	"strings"

	"github.com/karthangar/matteresp32hub/internal/esphome"
	"github.com/karthangar/matteresp32hub/internal/library"
	"github.com/karthangar/matteresp32hub/internal/yamldef"
)

// In flashRouter, add:
//   r.Post("/esphome", runESPHomeFlash(database))

func runESPHomeFlash(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Port          string                     `json:"port"`
			DeviceName    string                     `json:"device_name"`
			WiFiSSID      string                     `json:"wifi_ssid"`
			WiFiPassword  string                     `json:"wifi_password"`
			HubURL        string                     `json:"hub_url"`
			Board         string                     `json:"board"`
			HAIntegration bool                       `json:"ha_integration"`
			Components    []esphome.ComponentConfig   `json:"components"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Port == "" || req.DeviceName == "" || req.Board == "" || req.HubURL == "" {
			http.Error(w, "port, device_name, board, hub_url are required", http.StatusBadRequest)
			return
		}

		// Load module library
		mods, err := library.LoadModules()
		if err != nil {
			http.Error(w, "load modules: "+err.Error(), http.StatusInternalServerError)
			return
		}
		modMap := make(map[string]*yamldef.Module, len(mods))
		for _, m := range mods {
			modMap[m.ID] = m
		}

		dataDir := os.Getenv("DATA_DIR")
		if dataDir == "" {
			dataDir = "./data"
		}
		builder, err := esphome.NewBuilder(dataDir + "/esphome-cache")
		if err != nil {
			http.Error(w, "builder init: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer builder.Close()

		// Stream build logs as newline-delimited JSON
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Transfer-Encoding", "chunked")
		flusher, canFlush := w.(http.Flusher)

		// logWriter writes each log line as {"log":"..."}\n and flushes
		pr, pw := io.Pipe()
		done := make(chan flash.Result, 1)
		go func() {
			result := flash.FlashESPHomeDevice(database, builder, modMap, flash.ESPHomeRequest{
				Ctx: r.Context(), Port: req.Port, DeviceName: req.DeviceName,
				WiFiSSID: req.WiFiSSID, WiFiPassword: req.WiFiPassword,
				HubURL: req.HubURL, Board: req.Board,
				HAIntegration: req.HAIntegration, Components: req.Components,
			}, pw)
			pw.Close()
			done <- result
		}()

		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			json.NewEncoder(w).Encode(map[string]string{"log": line})
			if canFlush {
				flusher.Flush()
			}
		}

		result := <-done
		if result.Error != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": result.Error.Error()})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "device_id": result.DeviceID, "name": result.Name})
		}
		if canFlush {
			flusher.Flush()
		}
	}
}
```

Also add `r.Post("/esphome", runESPHomeFlash(database))` inside `flashRouter`.

- [ ] **Step 5: Run the test**

```bash
/usr/local/go/bin/go test ./internal/api/... -run "TestFlash_ESPHomeEndpointExists" -v 2>&1
```

Expected: PASS (returns non-404; likely 400 or 500 since no real USB port)

- [ ] **Step 6: Build to verify compilation**

```bash
/usr/local/go/bin/go build ./... 2>&1
```

Expected: clean build.

- [ ] **Step 7: Run all tests**

```bash
/usr/local/go/bin/go test ./... 2>&1
```

Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/flash/esphome.go internal/api/flash.go go.mod go.sum
git commit -m "feat: ESPHome flash orchestrator + POST /api/flash/esphome"
```

---

## Task 6: Heartbeat + ESPHome Key Endpoints

**Files:**
- Modify: `internal/api/devices.go`
- Modify: `internal/api/router.go`
- Modify: `internal/api/devices_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/api/devices_test.go`:

```go
func TestDevices_Heartbeat_ESPHome(t *testing.T) {
	srv := newTestServer(t)
	database := getDatabase(t, srv)

	require.NoError(t, database.CreateDevice(db.Device{
		ID: "dev-esph", Name: "Sensor", FirmwareType: "esphome", PSK: []byte{},
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/devices/dev-esph/heartbeat", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	got, err := database.GetDevice("dev-esph")
	require.NoError(t, err)
	assert.Equal(t, "online", got.Status)
}

func TestDevices_Heartbeat_MatterDeviceReturns404(t *testing.T) {
	srv := newTestServer(t)
	database := getDatabase(t, srv)

	require.NoError(t, database.CreateTemplate(db.TemplateRow{
		ID: "tpl-1", Name: "T1", Board: "esp32-c3", YAMLBody: "id: tpl-1\n",
	}))
	require.NoError(t, database.CreateDevice(db.Device{
		ID: "dev-matter", Name: "Light", TemplateID: "tpl-1",
		FirmwareType: "matter", PSK: make([]byte, 32),
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/devices/dev-matter/heartbeat", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDevices_Heartbeat_MissingDeviceReturns404(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/devices/missing/heartbeat", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDevices_ESPHomeKey(t *testing.T) {
	srv := newTestServer(t)
	database := getDatabase(t, srv)

	require.NoError(t, database.CreateDevice(db.Device{
		ID: "dev-esph", Name: "Sensor", FirmwareType: "esphome",
		ESPHomeAPIKey: "apikey123",
		ESPHomeConfig: `{"ota_password":"otapass"}`,
		PSK:           []byte{},
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices/dev-esph/esphome-key", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "apikey123", body["api_key"])
	assert.Equal(t, "otapass", body["ota_password"])
}

func TestDevices_ESPHomeKey_NoKey(t *testing.T) {
	srv := newTestServer(t)
	database := getDatabase(t, srv)

	require.NoError(t, database.CreateDevice(db.Device{
		ID: "dev-esph", Name: "Sensor", FirmwareType: "esphome",
		ESPHomeConfig: `{"ota_password":"otapass"}`,
		PSK:           []byte{},
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/devices/dev-esph/esphome-key", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
/usr/local/go/bin/go test ./internal/api/... -run "TestDevices_Heartbeat|TestDevices_ESPHomeKey" -v 2>&1
```

Expected: FAIL — routes not registered.

- [ ] **Step 3: Add handlers to devices.go**

Add to `internal/api/devices.go` (after `getPairingInfo`):

```go
func heartbeat(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		dev, err := database.GetDevice(id)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && dev.FirmwareType != "esphome") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ip := r.RemoteAddr
		if i := strings.LastIndex(ip, ":"); i >= 0 {
			ip = ip[:i]
		}
		if err := database.UpdateDeviceStatus(id, "online", ip); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func getESPHomeKey(database *db.Database) http.HandlerFunc {
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
		if dev.ESPHomeAPIKey == "" {
			http.Error(w, "no HA API key for this device", http.StatusNotFound)
			return
		}
		// Extract ota_password from esphome_config JSON
		var cfg struct {
			OTAPassword string `json:"ota_password"`
		}
		json.Unmarshal([]byte(dev.ESPHomeConfig), &cfg) //nolint:errcheck
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"api_key":      dev.ESPHomeAPIKey,
			"ota_password": cfg.OTAPassword,
		})
	}
}
```

Add `"strings"` to the import block in `devices.go` if not already present.

- [ ] **Step 4: Register routes in devicesRouter**

In `internal/api/devices.go`, update `devicesRouter`:

```go
func devicesRouter(database *db.Database) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/", listDevices(database))
		r.Get("/{id}/pairing", getPairingInfo(database))
		r.Get("/{id}/esphome-key", getESPHomeKey(database))
		r.Post("/{id}/heartbeat", heartbeat(database))
		r.Get("/{id}", getDevice(database))
	}
}
```

- [ ] **Step 5: Run new tests**

```bash
/usr/local/go/bin/go test ./internal/api/... -run "TestDevices_Heartbeat|TestDevices_ESPHomeKey" -v 2>&1
```

Expected: all PASS

- [ ] **Step 6: Run all tests**

```bash
/usr/local/go/bin/go test ./... 2>&1
```

Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add internal/api/devices.go internal/api/devices_test.go
git commit -m "feat: POST /api/devices/{id}/heartbeat + GET /api/devices/{id}/esphome-key"
```

---

## Task 7: Modules API ESPHome Filter + docker-compose.yml

**Files:**
- Modify: `internal/api/modules.go`
- Modify: `docker-compose.yml`

- [ ] **Step 1: Write the failing test**

Add to an existing or new `internal/api/modules_test.go`:

```go
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/karthangar/matteresp32hub/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModules_ESPHomeFilter(t *testing.T) {
	srv := newTestServer(t)
	database := getDatabase(t, srv)

	// Module with esphome block
	require.NoError(t, database.CreateModule(db.ModuleRow{
		ID: "dht22-test", Name: "DHT22", Category: "sensor",
		YAMLBody: `id: dht22-test
name: "DHT22"
version: "1.0"
category: sensor
io:
  - id: DATA
    type: digital_out
    label: "Data"
    constraints:
      digital: {active: high, initial_state: low}
matter:
  endpoint_type: temperature_sensor
  behaviors: [temperature_reporting]
esphome:
  components:
    - domain: sensor
      template: "platform: dht"
`,
	}))

	// Module without esphome block
	require.NoError(t, database.CreateModule(db.ModuleRow{
		ID: "no-esphome-test", Name: "No ESPHome", Category: "io",
		YAMLBody: `id: no-esphome-test
name: "No ESPHome"
version: "1.0"
category: io
io:
  - id: OUT
    type: digital_out
    label: "Out"
    constraints:
      digital: {active: high, initial_state: low}
matter:
  endpoint_type: on_off_light
  behaviors: [on_off]
`,
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/modules?esphome=true", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var mods []map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&mods))
	// Only ESPHome-capable modules returned (plus builtins from embedded FS that have esphome block)
	for _, m := range mods {
		assert.True(t, m["has_esphome"].(bool), "all returned modules must have esphome block")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
/usr/local/go/bin/go test ./internal/api/... -run "TestModules_ESPHomeFilter" -v 2>&1
```

Expected: FAIL — `?esphome=true` filter not implemented and `has_esphome` field absent.

- [ ] **Step 3: Update listModules handler in modules.go**

The `listModules` handler currently returns `[]db.ModuleRow`. We need to:
1. Parse each module's YAML to check if it has an ESPHome block
2. Filter when `?esphome=true`
3. Add a `has_esphome bool` field to the response

Replace `listModules`:

```go
func listModules(database *db.Database) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		esphomeOnly := r.URL.Query().Get("esphome") == "true"
		mods, err := database.ListModules()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		type moduleResp struct {
			db.ModuleRow
			HasESPHome bool `json:"has_esphome"`
		}
		var results []moduleResp
		for _, m := range mods {
			mod, err := yamldef.ParseModule([]byte(m.YAMLBody))
			hasESPHome := err == nil && mod.ESPHome != nil
			if esphomeOnly && !hasESPHome {
				continue
			}
			results = append(results, moduleResp{ModuleRow: m, HasESPHome: hasESPHome})
		}
		if results == nil {
			results = []moduleResp{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}
```

- [ ] **Step 4: Run the test**

```bash
/usr/local/go/bin/go test ./internal/api/... -run "TestModules_ESPHomeFilter" -v 2>&1
```

Expected: PASS

- [ ] **Step 5: Update docker-compose.yml**

Add the `esphome-cache` named volume and mount it in the hub service:

```yaml
services:
  matteresp32hub:
    # ... existing config ...
    volumes:
      # existing volumes...
      - esphome-cache:/data/esphome-cache   # add this line

volumes:
  esphome-cache:
```

- [ ] **Step 6: Run all tests**

```bash
/usr/local/go/bin/go test ./... 2>&1
```

Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add internal/api/modules.go internal/api/modules_test.go docker-compose.yml
git commit -m "feat: modules API esphome filter + esphome-cache Docker volume"
```

---

## Task 8: Frontend — Flash Wizard ESPHome Path

**Files:**
- Modify: `web/src/views/Flash.svelte`

Read `web/src/views/Flash.svelte` in full before editing. The existing server flash wizard uses variables `step` (1-5), `selectedTemplate`, `deviceNames`, `wifiSSID`, `wifiPassword`, `selectedPort`, `selectedFW`, `flashing`, `flashError`, `results`.

- [ ] **Step 1: Add ESPHome state variables**

In the `<script>` block, after the existing server flash state variables, add:

```js
// ESPHome flash path
let firmwareType = 'matter'; // 'matter' | 'esphome'
let espStep = 1;             // 1=type 2=board 3=components 4=config 5=flash 6=done
let espBoard = 'esp32-c3';
let espComponents = [];      // [{type, name, pins: {ROLE: 'GPIO4'}}]
let espDeviceName = '';
let espWifiSSID = '';
let espWifiPassword = '';
let espHubURL = window.location.origin;
let espHA = false;
let espFlashing = false;
let espFlashError = '';
let espLogs = [];
let espResult = null;        // {ok, device_id, name, error} | null
let espModules = [];         // loaded from /api/modules?esphome=true

async function loadESPHomeModules() {
  try {
    espModules = await api.get('/api/modules?esphome=true');
  } catch (e) {
    espFlashError = 'Failed to load modules: ' + e.message;
  }
}

function espAddComponent(moduleId) {
  const mod = espModules.find(m => m.id === moduleId);
  if (!mod) return;
  // Build empty pin map from module's io pins
  const pins = {};
  (mod.io || []).forEach(p => { pins[p.id] = ''; });
  espComponents = [...espComponents, { type: moduleId, name: mod.name, pins }];
}

function espRemoveComponent(i) {
  espComponents = espComponents.filter((_, idx) => idx !== i);
}

async function espDoFlash() {
  espFlashError = '';
  espFlashing = true;
  espLogs = [];
  espResult = null;
  try {
    const res = await fetch('/api/flash/esphome', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        port:           selectedPort,
        device_name:    espDeviceName,
        wifi_ssid:      espWifiSSID,
        wifi_password:  espWifiPassword,
        hub_url:        espHubURL,
        board:          espBoard,
        ha_integration: espHA,
        components:     espComponents,
      }),
    });
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop();
      for (const line of lines) {
        if (!line.trim()) continue;
        try {
          const obj = JSON.parse(line);
          if (obj.log !== undefined) espLogs = [...espLogs, obj.log];
          else espResult = obj;
        } catch {}
      }
    }
    espStep = 6;
  } catch (e) {
    espFlashError = e.message;
  } finally {
    espFlashing = false;
  }
}

function espReset() {
  espStep = 1; espBoard = 'esp32-c3'; espComponents = [];
  espDeviceName = ''; espWifiSSID = ''; espWifiPassword = '';
  espHA = false; espFlashing = false; espFlashError = '';
  espLogs = []; espResult = null; firmwareType = 'matter';
}
```

- [ ] **Step 2: Add firmware type selector to Step 1**

In the existing step 1 template block (where template selection happens), wrap it in a conditional so that when `firmwareType === 'matter'` the existing template picker shows, and add an ESPHome wizard when `firmwareType === 'esphome'`.

At the top of the server flash tab template (before the step content), add the type selector:

```html
{#if step === 1}
  <!-- Firmware type selector — shown at top of step 1 -->
  <div class="flex gap-2 mb-4">
    <button class="btn btn-sm {firmwareType === 'matter' ? 'btn-primary' : 'btn-ghost'}"
      on:click={() => { firmwareType = 'matter'; espStep = 1; }}>Matter</button>
    <button class="btn btn-sm {firmwareType === 'esphome' ? 'btn-primary' : 'btn-ghost'}"
      on:click={() => { firmwareType = 'esphome'; loadESPHomeModules(); }}>ESPHome</button>
  </div>
{/if}
```

- [ ] **Step 3: Add ESPHome wizard steps to the server flash tab**

After the existing `{#if step === 5}` done block and before the closing `</div>` of the server flash tab, add:

```html
<!-- ESPHome wizard steps (overlays the matter wizard when firmwareType === 'esphome') -->
{#if firmwareType === 'esphome'}

  {#if espStep === 1}
    <!-- Step 1: Board selection -->
    <div class="flex flex-col gap-3">
      <div class="text-sm font-semibold">Select board</div>
      {#each ['esp32-c3', 'esp32-h2', 'esp32', 'esp32-s3'] as board}
        <label class="flex items-center gap-2 cursor-pointer">
          <input type="radio" class="radio radio-sm" bind:group={espBoard} value={board} />
          <span class="text-sm font-mono">{board}</span>
        </label>
      {/each}
      <div class="flex gap-2 mt-2">
        <button class="btn btn-primary btn-sm" on:click={() => espStep = 2}>Next</button>
      </div>
    </div>

  {:else if espStep === 2}
    <!-- Step 2: Component builder -->
    <div class="flex flex-col gap-3">
      <div class="text-sm font-semibold">Add components</div>
      {#each espComponents as comp, i}
        <div class="border border-base-300 rounded p-3 flex flex-col gap-2">
          <div class="flex items-center justify-between">
            <span class="text-sm font-semibold">{comp.type}</span>
            <button class="btn btn-ghost btn-xs" on:click={() => espRemoveComponent(i)}>✕</button>
          </div>
          <input class="input input-bordered input-sm w-full" placeholder="Name (e.g. Room Temp)"
            bind:value={comp.name} />
          {#each Object.keys(comp.pins) as role}
            <label class="text-xs flex items-center gap-2">
              <span class="w-12 font-mono">{role}</span>
              <input class="input input-bordered input-xs flex-1" placeholder="GPIO4"
                bind:value={comp.pins[role]} />
            </label>
          {/each}
        </div>
      {/each}
      <select class="select select-bordered select-sm" on:change={e => { espAddComponent(e.target.value); e.target.value = ''; }}>
        <option value="">+ Add component…</option>
        {#each espModules as m}
          <option value={m.id}>{m.name}</option>
        {/each}
      </select>
      <div class="flex gap-2 mt-2">
        <button class="btn btn-ghost btn-sm" on:click={() => espStep = 1}>Back</button>
        <button class="btn btn-primary btn-sm" disabled={espComponents.length === 0}
          on:click={() => espStep = 3}>Next</button>
      </div>
    </div>

  {:else if espStep === 3}
    <!-- Step 3: Device config -->
    <div class="flex flex-col gap-3">
      <div class="text-sm font-semibold">Device configuration</div>
      <input class="input input-bordered input-sm" placeholder="Device name" bind:value={espDeviceName} />
      <input class="input input-bordered input-sm" placeholder="WiFi SSID" bind:value={espWifiSSID} />
      <input class="input input-bordered input-sm" type="password" placeholder="WiFi password" bind:value={espWifiPassword} />
      <input class="input input-bordered input-sm" placeholder="Hub URL (e.g. http://192.168.1.10:48060)" bind:value={espHubURL} />
      <label class="flex items-center gap-2 text-sm cursor-pointer">
        <input type="checkbox" class="checkbox checkbox-sm" bind:checked={espHA} />
        Home Assistant integration (generates API encryption key)
      </label>
      {#if flashError}<div class="alert alert-error text-xs">{flashError}</div>{/if}
      <div class="flex gap-2 mt-2">
        <button class="btn btn-ghost btn-sm" on:click={() => espStep = 2}>Back</button>
        <button class="btn btn-primary btn-sm"
          disabled={!espDeviceName || !espWifiSSID || !selectedPort}
          on:click={() => { espStep = 4; espDoFlash(); }}>Flash</button>
      </div>
      {#if !selectedPort}
        <div class="text-xs text-warning">Select a USB port above before flashing.</div>
      {/if}
    </div>

  {:else if espStep === 4}
    <!-- Step 4: Flashing + compile progress -->
    <div class="flex flex-col gap-3">
      <div class="text-sm font-semibold">
        {espFlashing ? 'Compiling + flashing… (may take several minutes on first flash)' : 'Done'}
      </div>
      <div class="bg-base-300 rounded p-2 h-48 overflow-y-auto font-mono text-xs">
        {#each espLogs as line}<div>{line}</div>{/each}
        {#if espFlashing}<div class="animate-pulse">▋</div>{/if}
      </div>
      {#if espFlashError}<div class="alert alert-error text-xs">{espFlashError}</div>{/if}
    </div>

  {:else if espStep === 5 || espStep === 6}
    <!-- Step 6: Done -->
    <div class="flex flex-col gap-3">
      {#if espResult?.ok}
        <div class="alert alert-success text-sm">Device flashed successfully — {espResult.name}</div>
        {#if espHA}
          <div class="text-xs">The ESPHome API encryption key has been stored. Use the <strong>ESPHome Key</strong> button in the Fleet view to retrieve it for Home Assistant pairing.</div>
        {/if}
      {:else if espResult}
        <div class="alert alert-error text-sm">{espResult.error}</div>
      {/if}
      <button class="btn btn-ghost btn-sm self-start" on:click={espReset}>Flash another device</button>
    </div>
  {/if}

{/if}
```

- [ ] **Step 4: Build frontend**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller/web
npm run build 2>&1 | tail -5
```

Expected: `✓ built in X.Xs` — no errors.

- [ ] **Step 5: Run all Go tests**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
/usr/local/go/bin/go test ./... 2>&1
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/views/Flash.svelte
git commit -m "feat: Flash wizard ESPHome path — board, component builder, compile + flash"
```

---

## Task 9: Frontend — Fleet View ESPHome Display

**Files:**
- Modify: `web/src/views/Fleet.svelte`

Read `web/src/views/Fleet.svelte` in full before editing.

- [ ] **Step 1: Add ESPHome state variables and handlers**

In the `<script>` block, after the existing `closePairModal` function, add:

```js
let espKeyModal = null; // { api_key, ota_password } | null
let espKeyError = '';

async function openESPHomeKey(device) {
  espKeyError = '';
  try {
    espKeyModal = await api.get(`/api/devices/${device.id}/esphome-key`);
  } catch (e) {
    espKeyError = e.message;
  }
}
function closeESPHomeKey() { espKeyModal = null; espKeyError = ''; }

function copyToClipboard(text) {
  navigator.clipboard.writeText(text).catch(() => {});
}
```

- [ ] **Step 2: Update device row actions**

Find the device row `<td>` that contains the "Pair" button. Replace it with a conditional based on `firmware_type`:

```html
<td>
  {#if d.firmware_type === 'esphome'}
    <button class="btn btn-xs btn-outline" on:click={() => openESPHomeKey(d)}>ESPHome Key</button>
  {:else}
    <button class="btn btn-xs btn-outline" on:click={() => openPairModal(d)}>Pair</button>
  {/if}
</td>
```

- [ ] **Step 3: Add ESPHome key modal**

After the existing `{#if pairModal}` block (the Matter QR modal), add:

```html
{#if espKeyModal}
  <button class="fixed inset-0 z-40 bg-black/60 cursor-default" on:click={closeESPHomeKey} />
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <div class="bg-base-200 rounded-xl shadow-xl w-full max-w-sm flex flex-col">
      <div class="flex items-center justify-between px-5 py-3 border-b border-base-300">
        <span class="font-semibold text-sm">ESPHome credentials</span>
        <button class="btn btn-ghost btn-xs" on:click={closeESPHomeKey}>✕</button>
      </div>
      <div class="flex flex-col gap-3 p-5">
        <p class="text-xs text-base-content/60">Use the API key in Home Assistant → Add Integration → ESPHome.</p>
        <div class="w-full text-xs font-mono bg-base-300 rounded p-3 space-y-2">
          <div class="flex items-center justify-between gap-2">
            <div>
              <div class="text-base-content/50">API Encryption Key</div>
              <div class="break-all">{espKeyModal.api_key}</div>
            </div>
            <button class="btn btn-ghost btn-xs shrink-0"
              on:click={() => copyToClipboard(espKeyModal.api_key)}>Copy</button>
          </div>
          <div class="flex items-center justify-between gap-2">
            <div>
              <div class="text-base-content/50">OTA Password</div>
              <div class="break-all">{espKeyModal.ota_password}</div>
            </div>
            <button class="btn btn-ghost btn-xs shrink-0"
              on:click={() => copyToClipboard(espKeyModal.ota_password)}>Copy</button>
          </div>
        </div>
      </div>
      <div class="px-5 pb-4 flex justify-end">
        <button class="btn btn-ghost btn-sm" on:click={closeESPHomeKey}>Close</button>
      </div>
    </div>
  </div>
{/if}

{#if espKeyError}
  <div class="toast toast-end"><div class="alert alert-error text-xs">{espKeyError}</div></div>
{/if}
```

- [ ] **Step 4: Build frontend**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller/web
npm run build 2>&1 | tail -5
```

Expected: `✓ built in X.Xs` — no errors.

- [ ] **Step 5: Run all Go tests**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
/usr/local/go/bin/go test ./... 2>&1
```

Expected: all PASS.

- [ ] **Step 6: Docker rebuild + redeploy**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
docker compose build && docker compose up -d && docker image prune -f
```

- [ ] **Step 7: Commit**

```bash
git add web/src/views/Fleet.svelte
git commit -m "feat: Fleet view ESPHome device display — key modal + conditional Pair/ESPHome-Key button"
```

---

## Verification

1. **All Go tests pass:** `go test ./...` — all green
2. **Frontend builds:** `cd web && npm run build` — no errors
3. **Go embed compiles:** `go build ./...` — no errors
4. **Manual test — ESPHome flash:**
   - Open Flash tab → select server flash → click ESPHome button
   - Step 1: select esp32-c3
   - Step 2: add DHT22 component, assign GPIO4 to DATA pin, name "Room Temp"
   - Step 3: fill device name, WiFi, hub URL; enable HA integration
   - Click Flash → live compile log appears → device flashes
   - Done screen confirms success
5. **Manual test — Fleet view:**
   - Flashed ESPHome device appears in Fleet
   - Row shows "ESPHome Key" button (not "Pair")
   - Click button → modal shows API key + OTA password with copy buttons
6. **Manual test — Heartbeat:**
   - After device boots, device calls `POST /api/devices/{id}/heartbeat` every 60s
   - Fleet shows status "online" and current IP
