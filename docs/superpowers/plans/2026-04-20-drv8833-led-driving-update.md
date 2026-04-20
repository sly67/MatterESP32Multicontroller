# DRV8833 LED Driving Update — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Update the CWWW `drv8833-led` module with gamma 2.2 and configurable LEDC channels, and create a new `drv8833-led-mono` module that drives an anti-parallel single-color LED strip by phase-offsetting two LEDC channels at 180° so both halves light simultaneously.

**Architecture:** Three YAML data files change (two modules, one template). One new module uses an ESPHome `custom` float output with an inline C++ class that uses ESP-IDF LEDC API directly — `hpoint` on channel B set to `period/2` (2048 ticks at 12-bit) enforces non-overlapping drive. Gamma 2.2 is applied inside `write_state`. A new `config` IO pin type is added to the Go yamldef package so that LEDC timer/channel numbers can be declared as module io pins and substituted into ESPHome templates at assemble time.

**Tech Stack:** Go (yamldef, library), ESPHome YAML, ESP-IDF LEDC API (C++), YAML module/template files

---

## File Structure

| File | Action |
|------|--------|
| `internal/yamldef/types.go` | Add `IOTypeConfig = "config"` constant |
| `internal/yamldef/module.go` | Add `"config"` to `validIOTypes` map |
| `internal/yamldef/module_test.go` | Add test for `config` io type |
| `data/modules/drv8833-led.yaml` | Update: gamma 2.2, 15kHz/12-bit, LEDC channel config pins |
| `data/templates/drv8833-bicolor-strip-c3.yaml` | Add LEDC channel defaults |
| `data/modules/drv8833-led-mono.yaml` | **Create**: MONO module with custom C++ output |
| `data/templates/drv8833-mono-strip-c3.yaml` | **Create**: MONO template with default pin/LEDC assignments |
| `internal/library/library_test.go` | Update module count 10→11, assert drv8833-led-mono |
| `internal/esphome/assembler_test.go` | Add tests for MONO module assembly |

---

## Task 1: Add `config` IO pin type

**Files:**
- Modify: `internal/yamldef/types.go`
- Modify: `internal/yamldef/module.go`
- Modify: `internal/yamldef/module_test.go`

This new type allows modules to declare non-GPIO configuration values (LEDC timer, channel numbers) as io pins so they participate in the assembler's `{ROLE}` substitution mechanism.

- [ ] **Step 1: Write the failing test**

Add to `internal/yamldef/module_test.go` after the existing tests:

```go
func TestParseModule_ConfigTypePin(t *testing.T) {
	yaml := []byte(`
id: test-config
name: "Test Config"
version: "1.0"
category: driver
io:
  - id: GPIO_A
    type: digital_pwm_out
    label: "Output GPIO"
    constraints:
      pwm: {frequency_hz: 15000, duty_min: 0.0, duty_max: 1.0, resolution_bits: 12}
  - id: LEDC_CHAN
    type: config
    label: "LEDC Channel (0-5)"
matter:
  endpoint_type: dimmable_light
  behaviors: [on_off, level_control]
`)
	mod, err := yamldef.ParseModule(yaml)
	require.NoError(t, err)
	assert.Equal(t, "config", mod.IO[1].Type)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
go test ./internal/yamldef/... -run TestParseModule_ConfigTypePin -v
```

Expected: FAIL — `"config"` is not in `validIOTypes`, error contains "io type".

- [ ] **Step 3: Add the constant to types.go**

In `internal/yamldef/types.go`, add after the existing `IOType*` constants:

```go
IOTypeConfig = "config" // non-GPIO configuration value (e.g. LEDC channel, timer index)
```

- [ ] **Step 4: Add to validIOTypes in module.go**

In `internal/yamldef/module.go`, add to the `validIOTypes` map:

```go
var validIOTypes = map[string]bool{
	IOTypePWMOut:        true,
	IOTypeDigitalPWMOut: true,
	IOTypeDigitalOut:    true,
	IOTypeDigitalIn:     true,
	IOTypeADCIn:         true,
	IOTypeI2CData:       true,
	IOTypeI2CClock:      true,
	IOTypeConfig:        true, // ← add this line
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/yamldef/... -run TestParseModule_ConfigTypePin -v
```

Expected: PASS

- [ ] **Step 6: Run all yamldef tests**

```bash
go test ./internal/yamldef/... -v
```

Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add internal/yamldef/types.go internal/yamldef/module.go internal/yamldef/module_test.go
git commit -m "feat: add 'config' io pin type for non-GPIO module parameters"
```

---

## Task 2: Update CWWW module (drv8833-led.yaml)

**Files:**
- Modify: `data/modules/drv8833-led.yaml`
- Modify: `data/templates/drv8833-bicolor-strip-c3.yaml`

Changes:
- `gamma_correct: 1` → `gamma_correct: 2.2`
- `frequency: 25000Hz` → `frequency: 15000Hz`, `resolution_bits: 11` → `12`
- Add `LEDC_CHAN_AIN1` and `LEDC_CHAN_AIN2` as `config` io pins
- Add `channel: {LEDC_CHAN_AIN1}` / `channel: {LEDC_CHAN_AIN2}` to the `ledc` output components
- Update template with default channel values

- [ ] **Step 1: Replace data/modules/drv8833-led.yaml**

Full file content:

```yaml
id: drv8833-led
name: "DRV8833 Warm White LED Strip (A Channel)"
version: "2.0"
category: driver
io:
  - id: AIN1
    type: digital_pwm_out
    label: "Bridge A Input 1 → AOUT1 (A+) — Side 1"
    constraints:
      pwm: {frequency_hz: 15000, duty_min: 0.0, duty_max: 1.0, resolution_bits: 12}
  - id: AIN2
    type: digital_pwm_out
    label: "Bridge A Input 2 → AOUT2 (A-) — Side 2"
    constraints:
      pwm: {frequency_hz: 15000, duty_min: 0.0, duty_max: 1.0, resolution_bits: 12}
  - id: LEDC_CHAN_AIN1
    type: config
    label: "LEDC Channel for AIN1 (0–5)"
  - id: LEDC_CHAN_AIN2
    type: config
    label: "LEDC Channel for AIN2 (0–5)"
pin_groups:
  - {id: bridge_A, pins: [AIN1, AIN2], mode: complementary, dead_time_us: 1}
matter:
  endpoint_type: color_temperature_light
  behaviors: [on_off, level_control, color_temperature]
esphome:
  components:
    - domain: output
      template: |
        platform: ledc
        pin: "{AIN1}"
        id: {ID}_ain1
        frequency: 15000Hz
        channel: {LEDC_CHAN_AIN1}
    - domain: output
      template: |
        platform: ledc
        pin: "{AIN2}"
        id: {ID}_ain2
        frequency: 15000Hz
        channel: {LEDC_CHAN_AIN2}
    - domain: light
      template: |
        platform: cwww
        name: "{NAME}"
        warm_white: {ID}_ain1
        cold_white: {ID}_ain2
        warm_white_color_temperature: 2700K
        cold_white_color_temperature: 6500K
        gamma_correct: 2.2
        effects:
          - strobe:
          - pulse:
          - flicker:
          - lambda:
              name: Twinkle
              update_interval: 50ms
              lambda: |-
                static uint8_t step = 0;
                static bool side = false;
                const uint8_t FADE = 20, HOLD = 10, PAUSE = 20;
                const uint8_t CYCLE = FADE * 2 + HOLD + PAUSE;
                float duty = 0.0f;
                if (step < FADE)
                  duty = 0.5f * step / FADE;
                else if (step < FADE + HOLD)
                  duty = 0.5f;
                else if (step < FADE * 2 + HOLD)
                  duty = 0.5f * (FADE * 2 + HOLD - step) / FADE;
                if (side) {
                  id({ID}_ain1).set_level(0.0f);
                  id({ID}_ain2).set_level(duty);
                } else {
                  id({ID}_ain1).set_level(duty);
                  id({ID}_ain2).set_level(0.0f);
                }
                if (++step >= CYCLE) { step = 0; side = !side; }
```

- [ ] **Step 2: Update drv8833-bicolor-strip-c3.yaml**

Full file content:

```yaml
id: drv8833-bicolor-strip-c3
board: esp32-c3
modules:
  - module: drv8833-led
    endpoint_name: "Bicolor LED Strip"
    pins:
      AIN1: GPIO0
      AIN2: GPIO1
      LEDC_CHAN_AIN1: "0"
      LEDC_CHAN_AIN2: "1"
```

- [ ] **Step 3: Verify the module parses cleanly**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
go test ./internal/library/... -v
```

Expected: PASS (count still 10 — no new module yet; existing modules load)

- [ ] **Step 4: Build to verify no compilation errors**

```bash
/usr/local/go/bin/go build ./...
```

Expected: clean build.

- [ ] **Step 5: Commit**

```bash
git add data/modules/drv8833-led.yaml data/templates/drv8833-bicolor-strip-c3.yaml
git commit -m "feat: drv8833-led CWWW — gamma 2.2, 15kHz/12-bit, LEDC channel config pins"
```

---

## Task 3: Create MONO module (drv8833-led-mono.yaml)

**Files:**
- Create: `data/modules/drv8833-led-mono.yaml`

The module uses ESPHome's `custom` float output platform with an inline C++ struct. The struct's `setup()` configures two LEDC channels on the same timer: channel A with `hpoint=0`, channel B with `hpoint=2048` (half of the 4096-tick 12-bit period). This ensures the two channels never overlap. `write_state()` applies gamma 2.2 and calls `ledc_set_duty` on both channels with the same duty. The three-spark Twinkle calls `id({ID}_mono_out).set_level()` directly to bypass the light component.

- [ ] **Step 1: Write the assembler test first**

Add to `internal/esphome/assembler_test.go`:

```go
func TestAssemble_MonoCustomOutput(t *testing.T) {
	mods := map[string]*yamldef.Module{
		"drv8833-led-mono": {
			ESPHome: &yamldef.ESPHomeDef{
				Components: []yamldef.ESPHomeComponent{
					{Domain: "output", Template: "platform: custom\ntype: float\nlambda: |-\n  return {};\noutputs:\n  - id: {ID}_mono_out"},
					{Domain: "light", Template: "platform: monochromatic\nname: \"{NAME}\"\noutput: {ID}_mono_out\ngamma_correct: 1.0"},
				},
			},
		},
	}
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "Mono Strip", DeviceID: "aabb",
		WiFiSSID: "net", WiFiPassword: "pass", OTAPassword: "otp",
		Components: []esphome.ComponentConfig{
			{Type: "drv8833-led-mono", Name: "Strip", Pins: map[string]string{
				"AIN1": "GPIO0", "AIN2": "GPIO1",
				"LEDC_TIMER": "1", "LEDC_CHAN_A": "2", "LEDC_CHAN_B": "3",
			}},
		},
	}
	out, err := esphome.Assemble(cfg, mods)
	require.NoError(t, err)
	assert.Contains(t, out, "platform: custom")
	assert.Contains(t, out, "platform: monochromatic")
	assert.Contains(t, out, "strip_mono_out")
	assert.NotContains(t, out, "{ID}")
	assert.NotContains(t, out, "{NAME}")
	assert.NotContains(t, out, "{AIN1}")
	assert.NotContains(t, out, "{LEDC_TIMER}")
}
```

- [ ] **Step 2: Run test to verify it passes (assembler already handles this)**

```bash
go test ./internal/esphome/... -run TestAssemble_MonoCustomOutput -v
```

Expected: PASS — the assembler substitutes all placeholders generically.

- [ ] **Step 3: Create data/modules/drv8833-led-mono.yaml**

```yaml
id: drv8833-led-mono
name: "DRV8833 Mono LED Strip (A Channel, bipolar drive)"
version: "1.0"
category: driver
io:
  - id: AIN1
    type: digital_pwm_out
    label: "Bridge A Input 1 → AOUT1 (half A)"
    constraints:
      pwm: {frequency_hz: 15000, duty_min: 0.0, duty_max: 0.5, resolution_bits: 12}
  - id: AIN2
    type: digital_pwm_out
    label: "Bridge A Input 2 → AOUT2 (half B)"
    constraints:
      pwm: {frequency_hz: 15000, duty_min: 0.0, duty_max: 0.5, resolution_bits: 12}
  - id: LEDC_TIMER
    type: config
    label: "LEDC Timer index (0–3)"
  - id: LEDC_CHAN_A
    type: config
    label: "LEDC Channel for AIN1 (0–5)"
  - id: LEDC_CHAN_B
    type: config
    label: "LEDC Channel for AIN2 (0–5)"
pin_groups:
  - {id: bridge_A, pins: [AIN1, AIN2], mode: complementary}
matter:
  endpoint_type: dimmable_light
  behaviors: [on_off, level_control]
esphome:
  components:
    - domain: output
      template: |
        platform: custom
        type: float
        lambda: |-
          #include "driver/ledc.h"
          #include <cmath>
          struct Drv8833Mono : public Component, public FloatOutput {
            int gpio_a_, gpio_b_;
            ledc_channel_t ch_a_, ch_b_;
            ledc_timer_t timer_;
            Drv8833Mono(int ga, int gb, int t, int ca, int cb)
              : gpio_a_(ga), gpio_b_(gb), timer_((ledc_timer_t)t),
                ch_a_((ledc_channel_t)ca), ch_b_((ledc_channel_t)cb) {}
            void setup() override {
              ledc_timer_config_t tc = {};
              tc.speed_mode = LEDC_LOW_SPEED_MODE;
              tc.duty_resolution = LEDC_TIMER_12_BIT;
              tc.timer_num = timer_;
              tc.freq_hz = 15000;
              tc.clk_cfg = LEDC_AUTO_CLK;
              ledc_timer_config(&tc);
              ledc_channel_config_t cc = {};
              cc.speed_mode = LEDC_LOW_SPEED_MODE;
              cc.timer_sel = timer_;
              cc.intr_type = LEDC_INTR_DISABLE;
              cc.duty = 0;
              cc.gpio_num = gpio_a_;
              cc.channel = ch_a_;
              cc.hpoint = 0;
              ledc_channel_config(&cc);
              cc.gpio_num = gpio_b_;
              cc.channel = ch_b_;
              cc.hpoint = 2048;
              ledc_channel_config(&cc);
            }
            void write_state(float state) override {
              float g = powf(state, 2.2f);
              uint32_t duty = (uint32_t)(g * 2048.0f);
              ledc_set_duty(LEDC_LOW_SPEED_MODE, ch_a_, duty);
              ledc_update_duty(LEDC_LOW_SPEED_MODE, ch_a_);
              ledc_set_duty(LEDC_LOW_SPEED_MODE, ch_b_, duty);
              ledc_update_duty(LEDC_LOW_SPEED_MODE, ch_b_);
            }
          };
          auto parseG = [](const char *s) -> int {
            return s[0] == 'G' ? atoi(s + 4) : atoi(s);
          };
          auto c = new Drv8833Mono(
            parseG("{AIN1}"), parseG("{AIN2}"),
            {LEDC_TIMER}, {LEDC_CHAN_A}, {LEDC_CHAN_B});
          App.register_component(c);
          return {c};
        outputs:
          - id: {ID}_mono_out
    - domain: light
      template: |
        platform: monochromatic
        name: "{NAME}"
        output: {ID}_mono_out
        gamma_correct: 1.0
        effects:
          - strobe:
          - pulse:
          - flicker:
          - lambda:
              name: Twinkle
              update_interval: 50ms
              lambda: |-
                struct Spark {
                  uint8_t state, step, wait;
                  float peak;
                };
                static Spark sp[3] = {};
                static bool init = false;
                if (!init) {
                  init = true;
                  sp[0].wait = 3; sp[1].wait = 12; sp[2].wait = 22;
                }
                const uint8_t FADE_IN = 4, HOLD = 2, FADE_OUT = 18;
                float total = 0.0f;
                for (int i = 0; i < 3; i++) {
                  float b = 0.0f;
                  auto &s = sp[i];
                  switch (s.state) {
                    case 0:
                      if (++s.step >= s.wait) {
                        s.state = 1; s.step = 0;
                        s.peak = 0.2f + (rand() % 40) * 0.01f;
                        s.wait = 5 + rand() % 28;
                      }
                      break;
                    case 1:
                      b = s.peak * s.step / (float)FADE_IN;
                      if (++s.step > FADE_IN) { s.state = 2; s.step = 0; }
                      break;
                    case 2:
                      b = s.peak;
                      if (++s.step > HOLD) { s.state = 3; s.step = 0; }
                      break;
                    case 3:
                      b = s.peak * (1.0f - s.step / (float)FADE_OUT);
                      if (++s.step > FADE_OUT) { s.state = 0; s.step = 0; }
                      break;
                  }
                  total += b;
                }
                if (total > 1.0f) total = 1.0f;
                id({ID}_mono_out).set_level(total);
```

- [ ] **Step 4: Verify the module parses**

```bash
go test ./internal/library/... -v
```

Expected: FAIL — count is now 11 but test expects 10.

- [ ] **Step 5: Commit**

```bash
git add data/modules/drv8833-led-mono.yaml
git commit -m "feat: drv8833-led-mono — bipolar hpoint drive, gamma 2.2 C++, three-spark Twinkle"
```

---

## Task 4: Create MONO template

**Files:**
- Create: `data/templates/drv8833-mono-strip-c3.yaml`

Uses LEDC timer 1 and channels 2+3 to avoid colliding with the CWWW template defaults (timer auto / channels 0+1).

- [ ] **Step 1: Create the file**

```yaml
id: drv8833-mono-strip-c3
board: esp32-c3
modules:
  - module: drv8833-led-mono
    endpoint_name: "Mono LED Strip"
    pins:
      AIN1: GPIO0
      AIN2: GPIO1
      LEDC_TIMER: "1"
      LEDC_CHAN_A: "2"
      LEDC_CHAN_B: "3"
```

- [ ] **Step 2: Commit**

```bash
git add data/templates/drv8833-mono-strip-c3.yaml
git commit -m "feat: drv8833-mono-strip-c3 template — default LEDC timer 1, channels 2+3"
```

---

## Task 5: Fix library test module count

**Files:**
- Modify: `internal/library/library_test.go`

- [ ] **Step 1: Update the test**

In `TestLoadModules_ReturnsAll`, change `assert.Len(t, mods, 10)` → `assert.Len(t, mods, 11)` and add:

```go
assert.True(t, ids["drv8833-led-mono"])
```

Full updated function:

```go
func TestLoadModules_ReturnsAll(t *testing.T) {
	mods, err := library.LoadModules()
	require.NoError(t, err)
	assert.Len(t, mods, 11)
	ids := make(map[string]bool)
	for _, m := range mods {
		ids[m.ID] = true
	}
	assert.True(t, ids["drv8833"])
	assert.True(t, ids["drv8833-led"])
	assert.True(t, ids["drv8833-led-mono"])
	assert.True(t, ids["wrgb-led"])
	assert.True(t, ids["bh1750"])
	assert.True(t, ids["analog-in"])
	assert.True(t, ids["gpio-switch"])
	assert.True(t, ids["dht22"])
	assert.True(t, ids["bme280"])
	assert.True(t, ids["neopixel"])
	assert.True(t, ids["binary-input"])
}
```

- [ ] **Step 2: Run all tests**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
go test ./... 2>&1
```

Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/library/library_test.go internal/esphome/assembler_test.go
git commit -m "test: update module count + add MONO assembler test"
```

---

## Task 6: Rebuild and deploy

- [ ] **Step 1: Build frontend (unchanged — no Svelte changes)**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller/web && npm run build
```

Expected: clean build.

- [ ] **Step 2: Build Go binary**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
/usr/local/go/bin/go build ./...
```

Expected: clean build.

- [ ] **Step 3: Rebuild Docker image and restart**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
docker build -t matteresp32hub . && docker compose up -d --no-deps matteresp32hub
```

Expected: image builds, container restarts.

---

## Verification

1. **Go tests pass:** `go test ./...` — all green
2. **CWWW flash test:**
   - Flash `drv8833-bicolor-strip-c3` template
   - Dim from 1% to 100% — no visible steps, smooth curve
   - Color temp slider works — warm and cold sides blend
   - Twinkle effect — side-alternating shimmer visible
3. **MONO flash test:**
   - Flash `drv8833-mono-strip-c3` template
   - Full strip lights (both physical halves)
   - Dim from 1% to 100% — smooth, no flicker
   - Twinkle effect — random sparkle visible on full strip
4. **LEDC collision check:** Both CWWW and MONO modules on same device (different timer/channel) — no interference
