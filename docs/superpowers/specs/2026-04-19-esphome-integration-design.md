# ESPHome Integration Design

**Date:** 2026-04-19
**Status:** Approved

---

## Goal

Allow a device to be flashed with either Matter firmware (existing path) or ESPHome firmware (new path). For ESPHome, the user assembles a device from existing modules via a component builder UI, the hub assembles an ESPHome YAML config from those modules, compiles it via a one-shot ESPHome Docker container, and flashes the resulting binary.

## Architecture

The firmware type is chosen at flash time. The existing module library is the single source of truth for both Matter and ESPHome devices — each module definition gains an `esphome:` block alongside its existing Matter configuration. The Flash wizard forks at step 1 (Matter vs ESPHome); the ESPHome path reuses the existing module+GPIO assignment UI. Effects remain attached to modules and can be extended with ESPHome automation equivalents in the future without changing the assembler.

**Tech Stack:** Go (Docker SDK, chi, modernc SQLite), Svelte 4, DaisyUI, `ghcr.io/esphome/esphome` Docker image

---

## Section 1: Data Model

### DB — three new columns on `devices`

| Column | Type | Default | Description |
|---|---|---|---|
| `firmware_type` | TEXT | `matter` | `matter` or `esphome` |
| `esphome_config` | TEXT | `''` | JSON blob — board + component list (see below) |
| `esphome_api_key` | TEXT | `''` | HA encryption key; empty if standalone mode |

Existing Matter devices are unaffected — `firmware_type` defaults to `matter`, other columns remain empty.

### `esphome_config` JSON shape

```json
{
  "board": "esp32-c3",
  "ha_integration": true,
  "ota_password": "hex...",
  "components": [
    { "type": "dht22",  "name": "Room Temp", "pins": {"DATA": "GPIO4"} },
    { "type": "relay",  "name": "Relay 1",   "pins": {"PIN":  "GPIO5"} },
    { "type": "neopixel", "name": "Strip",   "pins": {"DATA": "GPIO6"} }
  ]
}
```

### `Device` struct additions

```go
FirmwareType   string `json:"firmware_type"`
ESPHomeConfig  string `json:"-"` // JSON, never in list/get responses
ESPHomeAPIKey  string `json:"-"` // never in list/get responses
```

---

## Section 2: Module Extension + YAML Assembler

### Module YAML extension

Each module definition gains an `esphome:` block. Existing `matter:` side is unchanged. Pin role placeholders (`{PIN_ROLE}`) and name placeholder (`{NAME}`) are substituted by the assembler at compile time.

**DHT22 sensor example:**
```yaml
id: dht22
type: sensor
pins:
  DATA: { direction: output, description: "Data pin" }
esphome:
  components:
    - platform: dht
      model: DHT22
      pin: "{DATA}"
      temperature:
        name: "{NAME} Temperature"
      humidity:
        name: "{NAME} Humidity"
```

**GPIO relay example:**
```yaml
id: relay
type: switch
pins:
  PIN: { direction: output }
esphome:
  components:
    - platform: gpio
      domain: switch
      pin: "{PIN}"
      name: "{NAME}"
```

**NeoPixel light example:**
```yaml
id: neopixel
type: light
pins:
  DATA: { direction: output }
esphome:
  components:
    - platform: neopixelbus
      type: GRB
      pin: "{DATA}"
      name: "{NAME}"
```

**Binary input (button/door sensor) example:**
```yaml
id: binary_input
type: binary_sensor
pins:
  PIN: { direction: input }
esphome:
  components:
    - platform: gpio
      domain: binary_sensor
      pin:
        number: "{PIN}"
        mode: INPUT_PULLUP
        inverted: true
      name: "{NAME}"
```

**ADC sensor example:**
```yaml
id: adc
type: sensor
pins:
  PIN: { direction: input }
esphome:
  components:
    - platform: adc
      pin: "{PIN}"
      name: "{NAME}"
      update_interval: 10s
```

**BME280 example:**
```yaml
id: bme280
type: sensor
pins:
  SDA: { direction: output }
  SCL: { direction: output }
esphome:
  components:
    - platform: i2c
      sda: "{SDA}"
      scl: "{SCL}"
      id: i2c_bus
    - platform: bme280_i2c
      temperature:
        name: "{NAME} Temperature"
      humidity:
        name: "{NAME} Humidity"
      pressure:
        name: "{NAME} Pressure"
      i2c_id: i2c_bus
```

**BH1750 example:**
```yaml
id: bh1750
type: sensor
pins:
  SDA: { direction: output }
  SCL: { direction: output }
esphome:
  components:
    - platform: i2c
      sda: "{SDA}"
      scl: "{SCL}"
      id: i2c_bus
    - platform: bh1750
      name: "{NAME} Illuminance"
      i2c_id: i2c_bus
```

### Assembler — `internal/esphome/assembler.go`

```go
type ComponentConfig struct {
    Type  string            // module id (e.g. "dht22")
    Name  string            // user-facing name
    Pins  map[string]string // pin role → GPIO (e.g. {"DATA": "GPIO4"})
}

type Config struct {
    Board         string
    DeviceName    string
    WiFiSSID      string
    WiFiPassword  string
    HAIntegration bool
    APIKey        string // 32-byte hex, required if HAIntegration == true
    OTAPassword   string // random, generated at flash time
    Components    []ComponentConfig
}

func Assemble(cfg Config, modules map[string]yamldef.Module) (string, error)
// Returns complete ESPHome YAML string
```

The assembler produces:
1. `esphome:` header block (name, board)
2. `wifi:` block (SSID, password, AP fallback)
3. `logger:` block
4. `ota:` block (with generated password)
5. `api:` block (with encryption key if HA mode; omitted if standalone)
6. Per-component blocks (pin + name substituted from module's `esphome.components`)

---

## Section 3: ESPHome Build Pipeline

### `internal/esphome/builder.go`

```go
type Builder struct {
    DockerClient *client.Client
    CacheDir     string // e.g. /var/lib/matterhub/esphome-cache
}

// Compile assembles and compiles ESPHome YAML into a firmware-factory.bin.
// Streams build logs to logWriter. Timeout: 15 minutes.
func (b *Builder) Compile(ctx context.Context, deviceID string, yaml string, logWriter io.Writer) ([]byte, error)
```

**Compile steps:**
1. Write YAML to `{CacheDir}/{deviceID}/config.yaml`
2. Run `docker run --rm -v {CacheDir}:/config ghcr.io/esphome/esphome compile /config/{deviceID}/config.yaml`
3. Stream container stdout/stderr to `logWriter`
4. Read `{CacheDir}/{deviceID}/.esphome/build/{deviceID}/.pioenvs/{deviceID}/firmware-factory.bin`
5. Return binary bytes

**docker-compose.yml additions:**
```yaml
volumes:
  esphome-cache:

services:
  matteresp32hub:
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock  # already present
      - esphome-cache:/var/lib/matterhub/esphome-cache
```

**Build timeout:** 15 minutes (context deadline). First compile per board type: 5–10 min (PlatformIO toolchain download). Subsequent compiles: ~30s (cached).

### Flash orchestrator integration — `internal/flash/orchestrator.go`

When `FirmwareType == "esphome"`:
1. Skip PSK generation, NVS compilation, NVS flash
2. Generate random OTA password (hex, 16 bytes)
3. If HA integration: generate random API key (hex, 32 bytes)
4. Call `esphome.Assemble()` then `builder.Compile()`
5. Flash resulting binary at offset `0x0` via existing `usb.WriteFlash`
6. Register device in DB with `firmware_type = "esphome"`, `esphome_config` JSON, `esphome_api_key`

---

## Section 4: API Endpoints

Three new endpoints:

### `GET /api/modules?esphome=true`
Returns modules that have an `esphome:` block. Used by the component builder UI to populate the module picker.

Response:
```json
[
  { "id": "dht22", "type": "sensor", "pins": {"DATA": {...}}, "esphome": true },
  { "id": "relay", "type": "switch", "pins": {"PIN":  {...}}, "esphome": true }
]
```

### `GET /api/devices/{id}/esphome-key`
Returns the ESPHome API encryption key and OTA password for HA pairing. 404 if device is not ESPHome or has no API key. `api_key` comes from the `esphome_api_key` column; `ota_password` comes from the `ota_password` field inside `esphome_config` JSON.

Response: `{ "api_key": "hex...", "ota_password": "hex..." }`

### `POST /api/flash/esphome`
Server-flash ESPHome path. Accepts:
```json
{
  "port": "/dev/ttyUSB0",
  "device_name": "Kitchen Sensor",
  "wifi_ssid": "MyNetwork",
  "wifi_password": "secret",
  "ha_integration": true,
  "board": "esp32-c3",
  "components": [
    { "type": "dht22", "name": "Kitchen Temp", "pins": {"DATA": "GPIO4"} }
  ]
}
```

Streams build log lines as newline-delimited JSON (`{"log": "..."}`) via chunked response, then returns final result.

---

## Section 5: UI Changes

### Flash wizard — `web/src/views/Flash.svelte`

**Matter path** (existing, unchanged): template → name → WiFi → flash → done

**ESPHome path** (new):

| Step | Content |
|---|---|
| 1 | **Type** — Matter / ESPHome toggle |
| 2 | **Board** — esp32-c3, esp32-h2, etc. |
| 3 | **Components** — module picker (ESPHome-capable modules only) + GPIO pin assignment per module; same picker UI as existing template module assignment |
| 4 | **Config** — device name + WiFi credentials + HA toggle |
| 5 | **Flash** — live build log (streamed from Docker container stdout) + flash progress |
| 6 | **Done** — success message + ESPHome API key (if HA mode) with copy button |

### Fleet view — `web/src/views/Fleet.svelte`

Each device row checks `firmware_type`:
- `matter` → **Pair** button (existing Matter QR modal)
- `esphome` → **ESPHome Key** button (copies API key to clipboard) + **Reconfigure** button (opens component builder pre-filled from stored `esphome_config`, triggers recompile + reflash)

No changes to status display, OTA history, last-seen, or IP columns.

---

## Out of Scope (this iteration)

- ESPHome OTA via hub OTA server (ESPHome uses its own OTA mechanism)
- Background/pre-compile (compile triggered at flash time, not on config save)
- Browser Flash (WebSerial) ESPHome path — server flash only for v1
- Effects → ESPHome automations translation
- ESPHome device log streaming in the hub UI
