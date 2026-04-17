# MatterESP32Multicontroller — Design Spec
**Date:** 2026-04-16
**Status:** Approved

---

## 1. Overview

A two-part system:

1. **Universal ESP32 firmware** — a single pre-built binary that acts as a Matter Bridge, dynamically configuring its endpoints at boot from NVS. Built on ESP-IDF + ESP Matter SDK.
2. **Web management platform** — a Go + Svelte + DaisyUI application running in Docker on a Raspberry Pi (or any host), providing device flashing, hardware template management, fleet management, and OTA updates.

---

## 2. System Architecture

```
[ Docker Container (Go + Svelte) ]
       |                    |
  USB (N ports)        WiFi OTA (PSK-auth HTTPS)
  flash + config            |
       |                    |
[ ESP32-C3 / H2 / ... ]  (unlimited deployed devices)
  Matter Bridge
  NVS config
  HTTP OTA client
       |
[ Matter Ecosystem ]
  Home Assistant / Apple Home / Google Home
```

---

## 3. ESP32 Universal Firmware

### 3.1 Stack
```
[ Application layer  ]  NVS config reader, driver logic interpreter,
                         effect engine, measurement routines, HTTP OTA
[ ESP Matter SDK     ]  Bridge device, endpoints, commissioning, clusters
[ ESP-IDF            ]  GPIO, PWM, ADC, I2C, WiFi, NVS, esp_https_ota
[ ESP32-C3 / H2      ]  Hardware
```

### 3.2 Boot Sequence
1. Read NVS config
2. If no config → sit in USB config mode (serial ready, no Matter stack started)
3. If config present → instantiate endpoint objects from NVS module/endpoint definitions
4. Start Matter Bridge stack
5. Connect WiFi
6. Subscribe OTA task to task watchdog
7. Begin polling OTA server on schedule

### 3.3 NVS Schema
```
nvs/
  wifi/       ssid, password
  matter/     discriminator, passcode, PAI cert reference
  security/   psk (32 bytes)
  hw/         board_id (esp32c3 | esp32h2 | esp32s3 | custom)
  modules/    count, module_0..N {type, pins{}, constraints{}, compiled_truth_table{}}
  endpoints/  count, ep_0..N {matter_type, name, module_ref, effect_ref, effect_params{}}
  effects/    effect_0..N {id, params{}}
  routines/   routine_0..N {ops[], max_duration_ms, on_timeout}
```

### 3.4 FreeRTOS Task Layout
| Task | Purpose | WDT |
|---|---|---|
| Matter task | Matter stack, cluster messaging | subscribed |
| OTA task | Polls OTA server, downloads, applies | subscribed |
| Driver task | Runs effect engine, PWM output | subscribed |
| Sensor task | Runs measurement routines per sensor | subscribed (per routine) |
| Config task | USB serial config mode (pre-Matter) | subscribed |

---

## 4. Driver Logic System

Modules declare a truth table mapping named states to pin values. The firmware interpreter drives pins according to state names — no driver-specific code in firmware.

### 4.1 Pin Types
| Type | Description |
|---|---|
| `digital_pwm_out` | Digital output with PWM capability |
| `digital_out` | Digital output only |
| `digital_in` | Digital input |
| `adc_in` | ADC input channel |
| `i2c_data` | I2C SDA |
| `i2c_clock` | I2C SCL |

### 4.2 Per-Pin Constraints
```yaml
constraints:
  pwm:
    frequency_hz: 20000
    frequency_range: [100, 20000]
    duty_min: 0.0
    duty_max: 1.0
    resolution_bits: 10
    invert: false
    dead_time_us: 10
  adc:
    attenuation: 11db       # 0db / 2.5db / 6db / 11db
    resolution_bits: 12
    sample_rate_hz: 100
    filter: moving_average  # none / moving_average / median
    filter_samples: 8
  i2c:
    speed: 400000           # 100k / 400k / 1M
    pullup: internal        # internal / external / none
  digital:
    active: high            # high / low
    initial_state: low
```

### 4.3 Truth Table (example: DRV8833)
```yaml
truth_table:
  coast:   {in1: 0,   in2: 0}
  reverse: {in1: 0,   in2: 1}
  forward: {in1: 1,   in2: 0}
  brake:   {in1: 1,   in2: 1}

pwm_modes:
  forward_fast:  {in1: PWM, in2: 0}
  forward_slow:  {in1: 1,   in2: PWM}
  reverse_fast:  {in1: 0,   in2: PWM}
  reverse_slow:  {in1: PWM, in2: 1}
```

### 4.4 Pin Groups
```yaml
pin_groups:
  - id: bridge_A
    pins: [AIN1, AIN2]
    mode: complementary
    dead_time_us: 10
  - id: bridge_B
    pins: [BIN1, BIN2]
    mode: complementary
    dead_time_us: 10
```

---

## 5. Effect / Behavior System

Effects are YAML-defined, compatibility-checked against module type, and stored in NVS as parameter sets. Built-in library + user-importable.

### 5.1 Parameter Types
| Type | Description |
|---|---|
| `float` | Decimal with min/max/step |
| `int` | Integer with min/max/step |
| `bool` | Toggle |
| `percent` | 0.0–1.0 slider |
| `duration` | Time value + unit (ms/s) |
| `speed` | Frequency value + unit (hz) |
| `color_rgb` | RGB color picker |
| `color_wrgb` | WRGB color picker |
| `easing` | Curve selector (linear/sine/ease-in/ease-out/bounce) |
| `select` | Labeled enum, can reference `capability.*` |

### 5.2 Effect YAML Schema
```yaml
id: firefly-effect
name: "Firefly Blink"
compatible_with: [drv8833]
params:
  - {id: channel_mode, type: select,   options_from: capability.channel_mode}
  - {id: decay_mode,   type: select,   options_from: capability.decay_mode}
  - {id: speed,        type: speed,    default: 1.0,  unit: hz, min: 0.1, max: 10.0}
  - {id: intensity,    type: percent,  default: 0.8}
  - {id: fade_in,      type: duration, default: 200,  unit: ms}
  - {id: fade_out,     type: duration, default: 400,  unit: ms}
  - {id: easing,       type: easing,   default: sine}
  - {id: randomize,    type: bool,     default: true}
```

---

## 6. Sensor Measurement Routine System

Custom sensor routines are declarative YAML op sequences interpreted by the sensor task. All blocking ops require explicit timeouts. The interpreter feeds the watchdog before each op and enforces a per-routine `max_duration_ms` hard abort.

### 6.1 Available Ops
| Op | Purpose |
|---|---|
| `set` | Drive a pin high/low, optional duration_us |
| `wait_edge` | Wait for pin edge, mandatory timeout_us |
| `measure_pulse` | Time a pulse, mandatory timeout_us, store result |
| `read_adc` | Read ADC channel, store raw value |
| `read_i2c` | Read N bytes from I2C address, store |
| `compute` | Math expression over stored variables |
| `repeat` | Loop N times with taskYIELD() between iterations |
| `average` | Average collected values |
| `map_curve` | Lookup table interpolation for calibration |
| `clamp` | Clamp value to min/max |
| `report` | Emit value to Matter cluster |

### 6.2 Watchdog Safety Rules
- Every op calls `esp_task_wdt_reset()` before executing
- `wait_edge` and `measure_pulse` require `timeout_us` — YAML validator rejects missing timeouts
- `repeat` calls `taskYIELD()` between iterations
- `max_duration_ms` is a hard abort ceiling per routine
- `on_timeout: last_value | zero | error` defines abort behavior
- Sensor task is a dedicated FreeRTOS task, WDT-subscribed independently

### 6.3 Routine Example (HC-SR04)
```yaml
measurement:
  trigger_interval_ms: 100
  max_duration_ms: 200
  on_timeout: last_value
  routine:
    - {op: set,           pin: TRIG, value: 0,    duration_us: 2}
    - {op: set,           pin: TRIG, value: 1,    duration_us: 10}
    - {op: set,           pin: TRIG, value: 0}
    - {op: wait_edge,     pin: ECHO, edge: rising,  timeout_us: 30000}
    - {op: measure_pulse, pin: ECHO, edge: falling, timeout_us: 25000, store: pulse_us}
    - {op: compute,       expr: "pulse_us / 58.0",  store: distance_cm}
    - {op: clamp,         value: distance_cm, min: 2.0, max: 400.0, store: distance_cm}
    - {op: report,        value: distance_cm, unit: cm}
```

---

## 7. Analog Scaling

Applied as a post-routine step or as a standalone sensor routine op.

```yaml
params:
  raw_min: 0          # X — ADC floor
  raw_max: 4095       # Y — ADC ceiling
  scale_min: 0.0      # Z min — output floor
  scale_max: 100.0    # Z max — output ceiling
  unit: "%"           # displayed in dashboard and Matter
  curve: linear       # linear | logarithmic
  samples: 4          # ADC reads averaged per report
  invert: false
  threshold_low: 10.0
  threshold_high: 90.0
```

---

## 8. Hardware Template & Module System

### 8.1 Module YAML Schema
```yaml
id: drv8833
name: "DRV8833 Dual H-Bridge Motor Driver"
version: "1.0"
category: driver           # driver | sensor | io
io: [...]                  # pin definitions with types and constraints
channels: [...]            # logical channel groupings
pin_groups: [...]          # complementary/relationship declarations
truth_table: {...}         # state name → pin values
pwm_modes: {...}           # PWM mode → pin assignments
capabilities: [...]        # select options exposed to effects
matter:
  endpoint_type: extended_color_light
  behaviors: [firefly_effect, brightness]
```

### 8.2 Template YAML Schema
```yaml
id: firefly-hub-v1
board: esp32-c3
modules:
  - module: drv8833
    pins: {AIN1: GPIO4, AIN2: GPIO5, BIN1: GPIO6, BIN2: GPIO7}
    endpoint_name: "Firefly Lights"
    effect: firefly-effect
    effect_params: {speed: 1.2, channel_mode: alternating, decay_mode: fast}
  - module: wrgb-led
    pins: {R: GPIO8, G: GPIO9, B: GPIO10, W: GPIO11}
    endpoint_name: "WRGB Light"
    effect: breathing-effect
    effect_params: {period_s: 4.0}
  - module: bh1750
    pins: {SDA: GPIO18, SCL: GPIO19}
    endpoint_name: "Light Sensor"
    params: {poll_interval_s: 5}
  - module: analog-in
    pins: {SIG: GPIO1}
    endpoint_name: "Analog Sensor"
    params: {raw_min: 0, raw_max: 4095, scale_min: 0, scale_max: 100, unit: "%"}
```

### 8.3 Template Builder Wizard (5 steps)
1. Select base hardware (ESP32-C3 / H2 / S3 / custom / import YAML)
2. Add modules from library or import YAML
3. Assign GPIO pins to each module signal (conflict detection)
4. Map modules to Matter endpoint types, name them, assign effects
5. Review, name devices sequentially (1/Bedroom, 2/Attic…), flash

---

## 9. Web Platform

### 9.1 Tech Stack
- **Backend:** Go — single binary serving REST API + embedded Svelte frontend
- **Frontend:** Svelte + DaisyUI (`night` theme)
- **Database:** SQLite (device registry, templates, modules, firmware versions)
- **Containerization:** Docker via Portainer

### 9.2 Application Sections
| Section | Purpose |
|---|---|
| Fleet | All deployed devices, status, firmware version, filter/search |
| Flash Devices | Template picker, sequential naming, WiFi creds, USB flash |
| Templates | Create/edit/clone/export hardware templates |
| Module Library | Browse/filter built-in + imported modules, view YAML, export |
| OTA Updates | Pending updates, bulk push, auto-update schedule |
| Firmware | Firmware version list, upload new binary |
| Settings | USB ports, OTA server config, default WiFi, DB info |

### 9.3 Go Backend Structure
```
cmd/server/          main entry, HTTP server
internal/
  usb/               serial port manager, esptool wrapper, NVS pusher
  flash/             flash orchestrator (detect → flash FW → push NVS)
  nvs/               NVS blob compiler (template + device values → binary)
  ota/               OTA HTTP server, PSK HMAC verifier, update scheduler
  registry/          SQLite: devices, templates, modules, firmware
  templates/         template YAML parser/validator
  modules/           module library (built-in + user-imported)
  effects/           effect library (built-in + user-imported)
  api/               REST handlers
web/                 embedded Svelte build (go:embed)
data/
  firmware/          bundled .bin files per board variant
  modules/           built-in module YAML definitions
  effects/           built-in effect YAML definitions
```

---

## 10. Security Model

### 10.1 Web UI — TLS
HTTPS on port 48060. Self-signed cert auto-generated on first boot into `/data/certs`. Replaceable with a proper cert at any time.

### 10.2 Flash-time PSK
- 32-byte random PSK generated per device at flash time
- Stored in SQLite registry and pushed to device NVS
- Never displayed in UI
- Used exclusively for OTA authentication

### 10.3 OTA Authentication
```
GET /ota/check
Headers:
  X-Device-ID: esp-a1b2
  X-Signature: HMAC-SHA256(device-id + timestamp, PSK)
  X-Timestamp: 1776339331
```
Server validates: known device → fetch PSK → verify HMAC → timestamp within ±60s (replay protection). Responds with current firmware version. Device downloads only if newer.

---

## 11. Docker / Portainer Setup

### 11.1 Ports
| Port | Purpose |
|---|---|
| 48060 | Web UI (HTTPS) |
| 48061 | OTA server (HTTPS, PSK-authenticated) |

### 11.2 Persistent Volumes
```yaml
volumes:
  - /Portainer/MatterESP32/db:/data/db               # SQLite
  - /Portainer/MatterESP32/firmware:/data/firmware    # uploaded .bin files
  - /Portainer/MatterESP32/modules:/data/modules      # user module YAMLs
  - /Portainer/MatterESP32/templates:/data/templates  # user templates
  - /Portainer/MatterESP32/config:/data/config        # app.yaml, wifi.yaml, usb.yaml
  - /Portainer/MatterESP32/logs:/data/logs            # flash + OTA + system logs
  - /Portainer/MatterESP32/certs:/data/certs          # TLS certs
  - /Portainer/MatterESP32/matter:/data/matter        # PAI cert, CD blob
```

### 11.3 Config Files
```
config/
  app.yaml          ports, OTA settings, auto-update policy
  wifi.yaml         default WiFi credentials
  usb.yaml          declared USB port list
  psk-policy.yaml   PSK length, rotation policy
```

### 11.4 First Boot Behaviour
- Self-signed TLS cert generated if `/data/certs` is empty
- Default `app.yaml` written if `/data/config` is empty
- Built-in modules and effects available immediately (embedded in binary)
- User volumes start empty — populated as user imports/creates

---

## 12. Matter PAI / Device Attestation

The `/data/matter` volume holds the deployment's Product Attestation Intermediate (PAI) certificate and Certification Declaration. These are embedded into each device at flash time via the NVS config push, providing a consistent attestation chain across all deployed devices.

---

## 13. Sequential Device Naming

At flash time, devices are assigned names in flash order from a user-defined list:
```
1 → 1/Bedroom
2 → 2/Attic
3 → 3/Cupboard
...
N → N/<custom>
```
Names are stored in the device registry and displayed throughout the fleet dashboard and OTA views.

---

## 14. Fleet Management

- Unlimited device count
- Per-device: name, ID, template, firmware version, status, last seen, IP
- Bulk OTA push with per-device status tracking
- Auto-update policy: push to online devices, queue for offline (apply on reconnect)
- Filter by template, status, firmware version
- Logs per device: flash history, OTA history
