# DRV8833 Mono Strip — Dual-Side Effects Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add five dual-side parameterised lambda effects (Strobe, Breathing, Flicker, Flame, Twinkle) to the `drv8833-led-mono` module by extending the ESPHome assembler to support effect-param substitution.

**Architecture:** Add `EffectParams map[string]string` to `ComponentConfig`; the assembler substitutes `{KEY}` → value after the existing pin pass. The module YAML template uses uppercase param-name placeholders (`{SIDE}`, `{MIN_PCT}` etc.). Five new effect metadata YAMLs define params for future UI use. The existing built-in effects (strobe, pulse, flicker) remain untouched; the hardcoded Twinkle is replaced by the parameterised Dual Twinkle.

**Tech Stack:** Go 1.25, ESPHome lambda (C++/ESP-IDF LEDC), YAML, testify

---

## Key Context

- Hardware: 12-bit LEDC, max duty **2048** (50 % of 4096) because of bipolar drive.
- `LEDC_CHAN_A` (AIN1, hpoint = 0) = **left** side of strip.
- `LEDC_CHAN_B` (AIN2, hpoint = 2048) = **right** side of strip.
- Effects that set sides independently call `ledc_set_duty` / `ledc_update_duty` directly — calling `id({ID}_mono_out).set_level()` always sets both channels equally.
- Gamma correction (γ = 2.2) must be applied manually inside any lambda that calls `ledc_set_duty`: `duty = (uint32_t)(powf(b, 2.2f) * 2048.0f)`.
- **Side enum** (shared across all effects): 0 = left, 1 = right, 2 = both, 3 = alternating, 4 = random. Flame omits 4; Twinkle omits 3.
- **Template substitution key** = param `id` uppercased. Example: param `id: side` → placeholder `{SIDE}` in the module YAML.
- `ComponentConfig` is defined in `internal/esphome/assembler.go` (not queue.go). It is the JSON request shape for `POST /api/jobs`.

---

## File Structure

| File | Change |
|------|--------|
| `internal/esphome/assembler.go` | Add `EffectParams map[string]string` to `ComponentConfig`; substitute after pin pass |
| `internal/esphome/assembler_test.go` | Add two EffectParams substitution tests |
| `data/effects/strobe-dual-effect.yaml` | New — effect metadata |
| `data/effects/breathing-dual-effect.yaml` | New — effect metadata |
| `data/effects/flicker-dual-effect.yaml` | New — effect metadata |
| `data/effects/flame-dual-effect.yaml` | New — effect metadata |
| `data/effects/twinkle-dual-effect.yaml` | New — effect metadata |
| `data/modules/drv8833-led-mono.yaml` | Add 4 lambda blocks + replace existing Twinkle with Dual Twinkle |

---

## Task 1: Assembler — EffectParams substitution

**Files:**
- Modify: `internal/esphome/assembler.go`
- Test: `internal/esphome/assembler_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/esphome/assembler_test.go` (after `TestAssemble_MonoConfigPinSubstitution`):

```go
func TestAssemble_EffectParamSubstitution(t *testing.T) {
	mods := map[string]*yamldef.Module{
		"dual-strip": {
			ESPHome: &yamldef.ESPHomeDef{
				Components: []yamldef.ESPHomeComponent{
					{Domain: "light", Template: "platform: monochromatic\n  name: \"{NAME}\"\n  effects:\n    - lambda:\n        name: Dual Strobe\n        lambda: |-\n          const int SIDE = {SIDE};\n          const float MIN = {MIN_PCT};"},
				},
			},
		},
	}
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "Strip", DeviceID: "dev1",
		WiFiSSID: "n", WiFiPassword: "p", OTAPassword: "o",
		Components: []esphome.ComponentConfig{
			{
				Type: "dual-strip",
				Name: "LED Strip",
				Pins: map[string]string{},
				EffectParams: map[string]string{
					"SIDE":    "4",
					"MIN_PCT": "0.1",
				},
			},
		},
	}
	out, err := esphome.Assemble(cfg, mods)
	require.NoError(t, err)
	assert.Contains(t, out, "SIDE = 4")
	assert.Contains(t, out, "MIN = 0.1")
	assert.NotContains(t, out, "{SIDE}")
	assert.NotContains(t, out, "{MIN_PCT}")
}

func TestAssemble_EffectParamsAndPinsCoexist(t *testing.T) {
	mods := map[string]*yamldef.Module{
		"m": {
			ESPHome: &yamldef.ESPHomeDef{
				Components: []yamldef.ESPHomeComponent{
					{Domain: "output", Template: "pin: {GPIO}\n  side: {SIDE}"},
				},
			},
		},
	}
	cfg := esphome.Config{
		Board: "esp32-c3", DeviceName: "x", DeviceID: "y",
		WiFiSSID: "n", WiFiPassword: "p", OTAPassword: "o",
		Components: []esphome.ComponentConfig{
			{
				Type:         "m",
				Name:         "out",
				Pins:         map[string]string{"GPIO": "GPIO4"},
				EffectParams: map[string]string{"SIDE": "2"},
			},
		},
	}
	out, err := esphome.Assemble(cfg, mods)
	require.NoError(t, err)
	assert.Contains(t, out, "GPIO4")
	assert.Contains(t, out, "side: 2")
	assert.NotContains(t, out, "{GPIO}")
	assert.NotContains(t, out, "{SIDE}")
}
```

- [ ] **Step 2: Run tests — verify FAIL**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
go test ./internal/esphome/... -run TestAssemble_EffectParam -v
```

Expected: FAIL — `unknown field EffectParams` on `ComponentConfig`.

- [ ] **Step 3: Add EffectParams to ComponentConfig**

In `internal/esphome/assembler.go`, replace the `ComponentConfig` struct:

```go
// ComponentConfig is one component in an ESPHome device.
type ComponentConfig struct {
	Type         string            `json:"type"`
	Name         string            `json:"name"`
	Pins         map[string]string `json:"pins"`
	EffectParams map[string]string `json:"effect_params,omitempty"`
}
```

- [ ] **Step 4: Add substitution pass in Assemble**

In `internal/esphome/assembler.go`, inside the component loop (after `for role, gpio := range comp.Pins`), add:

```go
			for key, val := range comp.EffectParams {
				rendered = strings.ReplaceAll(rendered, "{"+key+"}", val)
			}
```

The full inner loop (lines 117–130 of the current file) becomes:

```go
		for _, ec := range mod.ESPHome.Components {
			rendered := ec.Template
			rendered = strings.ReplaceAll(rendered, "{NAME}", comp.Name)
			rendered = strings.ReplaceAll(rendered, "{ID}", idSlug(comp.Name))
			for role, gpio := range comp.Pins {
				rendered = strings.ReplaceAll(rendered, "{"+role+"}", gpio)
			}
			for key, val := range comp.EffectParams {
				rendered = strings.ReplaceAll(rendered, "{"+key+"}", val)
			}
			entries = append(entries, entry{ec.Domain, rendered})
			if !domainSeen[ec.Domain] {
				domainSeen[ec.Domain] = true
				domainOrder = append(domainOrder, ec.Domain)
			}
		}
```

- [ ] **Step 5: Run the new tests — verify PASS**

```bash
go test ./internal/esphome/... -run TestAssemble_EffectParam -v
```

Expected: both PASS.

- [ ] **Step 6: Run all assembler tests**

```bash
go test ./internal/esphome/... -v
```

Expected: all PASS, no regressions.

- [ ] **Step 7: Commit**

```bash
git add internal/esphome/assembler.go internal/esphome/assembler_test.go
git commit -m "feat: effect param substitution in ESPHome assembler"
```

---

## Task 2: Effect metadata YAML files

**Files:**
- Create: `data/effects/strobe-dual-effect.yaml`
- Create: `data/effects/breathing-dual-effect.yaml`
- Create: `data/effects/flicker-dual-effect.yaml`
- Create: `data/effects/flame-dual-effect.yaml`
- Create: `data/effects/twinkle-dual-effect.yaml`

These are pure metadata — they define params for UI display and document defaults. The `id` of each param is lowercase; its uppercase form is the template substitution key used in the module YAML.

- [ ] **Step 1: Create `data/effects/strobe-dual-effect.yaml`**

```yaml
id: strobe-dual
name: "Dual Strobe"
version: "1.0"
compatible_with: [drv8833-led-mono]
params:
  - {id: SIDE,         type: int,     label: "Side (0=L 1=R 2=Both 3=Alt 4=Rand)", default: 4,   min: 0, max: 4}
  - {id: MIN_PCT,      type: percent, label: "Min brightness",                      default: 0.0}
  - {id: MAX_PCT,      type: percent, label: "Max brightness",                      default: 1.0}
  - {id: FLASH_MS,     type: int,     label: "Flash duration (ms)",                 default: 80,  min: 10,  max: 2000}
  - {id: PAUSE_MIN_MS, type: int,     label: "Min pause (ms)",                      default: 100, min: 0,   max: 5000}
  - {id: PAUSE_MAX_MS, type: int,     label: "Max pause (ms)",                      default: 600, min: 0,   max: 5000}
```

- [ ] **Step 2: Create `data/effects/breathing-dual-effect.yaml`**

```yaml
id: breathing-dual
name: "Dual Breathing"
version: "1.0"
compatible_with: [drv8833-led-mono]
params:
  - {id: SIDE,         type: int,     label: "Side (0=L 1=R 2=Both 3=Alt 4=Rand)", default: 2,    min: 0, max: 4}
  - {id: MIN_PCT,      type: percent, label: "Min brightness",                      default: 0.0}
  - {id: MAX_PCT,      type: percent, label: "Max brightness",                      default: 1.0}
  - {id: PERIOD_MS,    type: int,     label: "Cycle period (ms)",                   default: 3000, min: 200,  max: 20000}
  - {id: PAUSE_MIN_MS, type: int,     label: "Min pause (ms)",                      default: 0,    min: 0,    max: 5000}
  - {id: PAUSE_MAX_MS, type: int,     label: "Max pause (ms)",                      default: 500,  min: 0,    max: 5000}
```

- [ ] **Step 3: Create `data/effects/flicker-dual-effect.yaml`**

```yaml
id: flicker-dual
name: "Dual Flicker"
version: "1.0"
compatible_with: [drv8833-led-mono]
params:
  - {id: SIDE,      type: int,     label: "Side (0=L 1=R 2=Both 3=Alt 4=Rand)", default: 2,    min: 0, max: 4}
  - {id: MIN_PCT,   type: percent, label: "Min brightness",                      default: 0.1}
  - {id: MAX_PCT,   type: percent, label: "Max brightness",                      default: 1.0}
  - {id: SPEED_HZ,  type: float,   label: "Update rate (Hz)",                    default: 20.0, min: 1.0, max: 50.0}
  - {id: SMOOTHING, type: float,   label: "Smoothing (0=instant 0.95=slow)",     default: 0.3,  min: 0.0, max: 0.95}
```

- [ ] **Step 4: Create `data/effects/flame-dual-effect.yaml`**

```yaml
id: flame-dual
name: "Dual Flame"
version: "1.0"
compatible_with: [drv8833-led-mono]
params:
  - {id: SIDE,          type: int,     label: "Side (0=L 1=R 2=Both 3=Alt)", default: 2,   min: 0, max: 3}
  - {id: MIN_PCT,       type: percent, label: "Ember floor",                  default: 0.15}
  - {id: MAX_PCT,       type: percent, label: "Peak flare",                   default: 1.0}
  - {id: SPEED,         type: float,   label: "Speed multiplier",             default: 1.0, min: 0.1, max: 5.0}
  - {id: FLARE_RATE_HZ, type: float,   label: "Flares per second",            default: 2.0, min: 0.1, max: 10.0}
  - {id: WIND,          type: float,   label: "Wind bias (-1=left +1=right)", default: 0.0, min: -1.0, max: 1.0}
```

- [ ] **Step 5: Create `data/effects/twinkle-dual-effect.yaml`**

```yaml
id: twinkle-dual
name: "Dual Twinkle"
version: "1.0"
compatible_with: [drv8833-led-mono]
params:
  - {id: SIDE,    type: int,     label: "Side (0=L 1=R 2=Both 4=Rand)", default: 2,   min: 0, max: 4}
  - {id: MIN_PCT, type: percent, label: "Floor brightness",              default: 0.0}
  - {id: MAX_PCT, type: percent, label: "Max spark peak",                default: 0.6}
  - {id: SPEED,   type: float,   label: "Speed multiplier",             default: 1.0, min: 0.1, max: 5.0}
  - {id: DENSITY, type: int,     label: "Sparks per active side",       default: 3,   min: 1,   max: 6}
```

- [ ] **Step 6: Verify no test regressions**

```bash
go test ./... 2>&1 | grep -E "FAIL|ok"
```

Expected: all `ok`, no `FAIL`.

- [ ] **Step 7: Commit**

```bash
git add data/effects/strobe-dual-effect.yaml data/effects/breathing-dual-effect.yaml \
        data/effects/flicker-dual-effect.yaml data/effects/flame-dual-effect.yaml \
        data/effects/twinkle-dual-effect.yaml
git commit -m "feat: effect metadata YAMLs for five dual-side drv8833-mono effects"
```

---

## Task 3: Dual Strobe lambda

**Files:**
- Modify: `data/modules/drv8833-led-mono.yaml`

Add the Dual Strobe lambda **after** `- flicker:` and **before** the existing Twinkle lambda inside the `effects:` block of the light component template.

The light component template currently ends with `- flicker:` followed by `- lambda:\n    name: Twinkle`. Insert between them.

- [ ] **Step 1: Add the lambda block**

In `data/modules/drv8833-led-mono.yaml`, after the line `          - flicker:`, insert:

```yaml
          - lambda:
              name: Dual Strobe
              update_interval: 10ms
              lambda: |-
                const int     FLASH_TICKS   = ({FLASH_MS}) / 10;
                const int     PAUSE_MIN_T   = ({PAUSE_MIN_MS}) / 10;
                const int     PAUSE_RANGE_T = (({PAUSE_MAX_MS}) - ({PAUSE_MIN_MS})) / 10;
                const int     SIDE_PARAM    = {SIDE};
                auto duty_of = [](float b) -> uint32_t {
                  if (b < 0.0f) b = 0.0f; if (b > 1.0f) b = 1.0f;
                  uint32_t d = (uint32_t)(powf(b, 2.2f) * 2048.0f);
                  return d > 2048u ? 2048u : d;
                };
                const uint32_t MIN_D = duty_of({MIN_PCT});
                const uint32_t MAX_D = duty_of({MAX_PCT});
                auto set_sides = [&](uint32_t da, uint32_t db) {
                  ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_A}, da);
                  ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_A});
                  ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_B}, db);
                  ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_B});
                };
                static int8_t  phase      = 1;
                static int32_t ticks_left = 0;
                static uint8_t cur_side   = 2;
                static uint8_t alt_toggle = 0;
                if (--ticks_left > 0) {
                  if (phase == 0) {
                    uint32_t la = (cur_side == 0 || cur_side == 2) ? MAX_D : MIN_D;
                    uint32_t lb = (cur_side == 1 || cur_side == 2) ? MAX_D : MIN_D;
                    set_sides(la, lb);
                  } else {
                    set_sides(MIN_D, MIN_D);
                  }
                  return;
                }
                if (phase == 1) {
                  phase = 0;
                  ticks_left = FLASH_TICKS > 0 ? FLASH_TICKS : 1;
                  if      (SIDE_PARAM == 3) { cur_side = alt_toggle; alt_toggle ^= 1; }
                  else if (SIDE_PARAM == 4) { cur_side = (uint8_t)(rand() % 3); }
                  else                      { cur_side = (uint8_t)SIDE_PARAM; }
                  uint32_t la = (cur_side == 0 || cur_side == 2) ? MAX_D : MIN_D;
                  uint32_t lb = (cur_side == 1 || cur_side == 2) ? MAX_D : MIN_D;
                  set_sides(la, lb);
                } else {
                  phase = 1;
                  int range = PAUSE_RANGE_T > 0 ? PAUSE_RANGE_T : 1;
                  ticks_left = PAUSE_MIN_T + rand() % range;
                  if (ticks_left <= 0) ticks_left = 1;
                  set_sides(MIN_D, MIN_D);
                }
```

- [ ] **Step 2: Build to verify no Go errors**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
go build ./...
```

Expected: clean (YAML content errors appear only at ESPHome compile time, not here).

- [ ] **Step 3: Run tests**

```bash
go test ./internal/esphome/... -v
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add data/modules/drv8833-led-mono.yaml
git commit -m "feat: Dual Strobe lambda in drv8833-led-mono"
```

---

## Task 4: Dual Breathing lambda

**Files:**
- Modify: `data/modules/drv8833-led-mono.yaml`

Add after Dual Strobe, before the existing Twinkle lambda.

- [ ] **Step 1: Add the lambda block**

In `data/modules/drv8833-led-mono.yaml`, after the closing line of the Dual Strobe lambda block, insert:

```yaml
          - lambda:
              name: Dual Breathing
              update_interval: 10ms
              lambda: |-
                const int   HALF_TICKS    = ({PERIOD_MS}) / 2 / 10;
                const int   PAUSE_MIN_T   = ({PAUSE_MIN_MS}) / 10;
                const int   PAUSE_RANGE_T = (({PAUSE_MAX_MS}) - ({PAUSE_MIN_MS})) / 10;
                const int   SIDE_PARAM    = {SIDE};
                const float BRT_MIN       = {MIN_PCT};
                const float BRT_MAX       = {MAX_PCT};
                const float BRT_RANGE     = BRT_MAX - BRT_MIN;
                auto duty_of = [](float b) -> uint32_t {
                  if (b < 0.0f) b = 0.0f; if (b > 1.0f) b = 1.0f;
                  uint32_t d = (uint32_t)(powf(b, 2.2f) * 2048.0f);
                  return d > 2048u ? 2048u : d;
                };
                auto set_sides = [&](uint32_t da, uint32_t db) {
                  ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_A}, da);
                  ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_A});
                  ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_B}, db);
                  ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_B});
                };
                static int8_t  phase      = 2;
                static int32_t step       = 0;
                static int32_t ticks_left = 1;
                static uint8_t cur_side   = (uint8_t)(SIDE_PARAM < 4 ? SIDE_PARAM : 2);
                const int ht = HALF_TICKS > 0 ? HALF_TICKS : 1;
                if (phase == 2) {
                  set_sides(duty_of(BRT_MIN), duty_of(BRT_MIN));
                  if (--ticks_left <= 0) {
                    phase = 0; step = 0;
                    if (SIDE_PARAM == 4) cur_side = (uint8_t)(rand() % 3);
                  }
                  return;
                }
                float t      = (float)step / (float)ht;
                float sine_t = (phase == 0) ? sinf(t * (float)M_PI / 2.0f)
                                            : sinf((1.0f - t) * (float)M_PI / 2.0f);
                float active = BRT_MIN + BRT_RANGE * sine_t;
                float mirror = BRT_MAX - (active - BRT_MIN);
                float la, lb;
                if      (cur_side == 0) { la = active; lb = BRT_MIN; }
                else if (cur_side == 1) { la = BRT_MIN; lb = active; }
                else if (cur_side == 3) { la = active;  lb = mirror; }
                else                    { la = active;  lb = active; }
                set_sides(duty_of(la), duty_of(lb));
                if (++step > ht) {
                  step = 0;
                  if (phase == 0) {
                    phase = 1;
                  } else {
                    phase = 2;
                    int range = PAUSE_RANGE_T > 0 ? PAUSE_RANGE_T : 1;
                    ticks_left = PAUSE_MIN_T + rand() % range;
                    if (ticks_left <= 0) ticks_left = 1;
                  }
                }
```

- [ ] **Step 2: Build + test + commit**

```bash
go build ./...
go test ./internal/esphome/... -v
git add data/modules/drv8833-led-mono.yaml
git commit -m "feat: Dual Breathing lambda in drv8833-led-mono"
```

---

## Task 5: Dual Flicker lambda

**Files:**
- Modify: `data/modules/drv8833-led-mono.yaml`

Add after Dual Breathing, before existing Twinkle.

- [ ] **Step 1: Add the lambda block**

```yaml
          - lambda:
              name: Dual Flicker
              update_interval: 10ms
              lambda: |-
                const int      SIDE_PARAM  = {SIDE};
                const float    F_MIN       = {MIN_PCT};
                const float    F_MAX       = {MAX_PCT};
                const float    F_RANGE     = F_MAX - F_MIN;
                const float    SMOOTH      = {SMOOTHING};
                const float    ONE_MINUS_S = 1.0f - SMOOTH;
                const uint32_t UPD_EVERY   = (uint32_t)(1000.0f / ({SPEED_HZ}) / 10.0f + 0.5f);
                auto duty_of = [](float b) -> uint32_t {
                  if (b < 0.0f) b = 0.0f; if (b > 1.0f) b = 1.0f;
                  uint32_t d = (uint32_t)(powf(b, 2.2f) * 2048.0f);
                  return d > 2048u ? 2048u : d;
                };
                static float    cur_a = {MIN_PCT};
                static float    cur_b = {MIN_PCT};
                static float    tgt_a = {MIN_PCT};
                static float    tgt_b = {MIN_PCT};
                static uint32_t tick  = 0;
                cur_a += (tgt_a - cur_a) * ONE_MINUS_S;
                cur_b += (tgt_b - cur_b) * ONE_MINUS_S;
                if (++tick >= UPD_EVERY) {
                  tick = 0;
                  float rnd_a = F_MIN + (rand() % 1000) * 0.001f * F_RANGE;
                  float rnd_b = F_MIN + (rand() % 1000) * 0.001f * F_RANGE;
                  if      (SIDE_PARAM == 0) { tgt_a = rnd_a; tgt_b = F_MIN; }
                  else if (SIDE_PARAM == 1) { tgt_a = F_MIN; tgt_b = rnd_b; }
                  else if (SIDE_PARAM == 2) { tgt_a = rnd_a; tgt_b = rnd_b; }
                  else if (SIDE_PARAM == 3) { tgt_a = rnd_a; tgt_b = F_MAX - (rnd_a - F_MIN); }
                  else { if (rand() % 2) tgt_a = rnd_a; if (rand() % 2) tgt_b = rnd_b; }
                }
                ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_A}, duty_of(cur_a));
                ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_A});
                ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_B}, duty_of(cur_b));
                ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_B});
```

- [ ] **Step 2: Build + test + commit**

```bash
go build ./...
go test ./internal/esphome/... -v
git add data/modules/drv8833-led-mono.yaml
git commit -m "feat: Dual Flicker lambda in drv8833-led-mono"
```

---

## Task 6: Dual Flame lambda

**Files:**
- Modify: `data/modules/drv8833-led-mono.yaml`

Add after Dual Flicker, before existing Twinkle. Uses `update_interval: 50ms` for an organic, slower feel.

- [ ] **Step 1: Add the lambda block**

```yaml
          - lambda:
              name: Dual Flame
              update_interval: 50ms
              lambda: |-
                const int   SIDE_PARAM = {SIDE};
                const float F_MIN      = {MIN_PCT};
                const float F_MAX      = {MAX_PCT};
                const float F_RANGE    = F_MAX - F_MIN;
                const float SPD        = {SPEED};
                const float FLARE_PROB = ({FLARE_RATE_HZ}) * 0.05f;
                const float WIND       = {WIND};
                auto fr  = []() -> float { return (rand() % 1000) * 0.001f; };
                auto frc = []() -> float { return (rand() % 1000) * 0.001f - 0.5f; };
                auto clp = [](float v, float lo, float hi) -> float {
                  return v < lo ? lo : (v > hi ? hi : v);
                };
                auto duty_of = [](float b) -> uint32_t {
                  if (b < 0.0f) b = 0.0f; if (b > 1.0f) b = 1.0f;
                  uint32_t d = (uint32_t)(powf(b, 2.2f) * 2048.0f);
                  return d > 2048u ? 2048u : d;
                };
                static float base_a  = {MIN_PCT};
                static float base_b  = {MIN_PCT};
                static float flare_a = 0.0f;
                static float flare_b = 0.0f;
                const float center = F_MIN + F_RANGE * 0.4f;
                base_a += frc() * 0.06f * SPD;
                base_a += (center - base_a) * 0.05f;
                base_a = clp(base_a, F_MIN, F_MAX);
                base_b += frc() * 0.06f * SPD;
                base_b += (center - base_b) * 0.05f;
                base_b = clp(base_b, F_MIN, F_MAX);
                if (fr() < FLARE_PROB) flare_a = fr() * F_RANGE * 0.6f;
                if (fr() < FLARE_PROB) flare_b = fr() * F_RANGE * 0.6f;
                flare_a *= 0.7f;
                flare_b *= 0.7f;
                float brt_a = clp(base_a + flare_a, F_MIN, F_MAX);
                float brt_b = clp(base_b + flare_b, F_MIN, F_MAX);
                brt_a = clp(brt_a * (1.0f - WIND * 0.5f), F_MIN, F_MAX);
                brt_b = clp(brt_b * (1.0f + WIND * 0.5f), F_MIN, F_MAX);
                uint32_t da, db;
                if      (SIDE_PARAM == 0) { da = duty_of(brt_a); db = duty_of(F_MIN); }
                else if (SIDE_PARAM == 1) { da = duty_of(F_MIN);  db = duty_of(brt_b); }
                else if (SIDE_PARAM == 3) { da = duty_of(brt_a);  db = duty_of(clp(F_MAX - (brt_a - F_MIN), F_MIN, F_MAX)); }
                else                      { da = duty_of(brt_a);  db = duty_of(brt_b); }
                ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_A}, da);
                ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_A});
                ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_B}, db);
                ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_B});
```

- [ ] **Step 2: Build + test + commit**

```bash
go build ./...
go test ./internal/esphome/... -v
git add data/modules/drv8833-led-mono.yaml
git commit -m "feat: Dual Flame lambda in drv8833-led-mono"
```

---

## Task 7: Replace hardcoded Twinkle with Dual Twinkle

**Files:**
- Modify: `data/modules/drv8833-led-mono.yaml`

The existing Twinkle lambda (starting at the `- lambda:\n    name: Twinkle` line, ending with `id({ID}_mono_out).set_level(total);`) must be replaced entirely.

The old block to remove (exact text from the file — the full lambda including the closing `set_level` call and the blank line after the template block's last line):

```
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
                        s.peak = 0.2f + (rand() % 41) * 0.01f;
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

Replace with:

```yaml
          - lambda:
              name: Dual Twinkle
              update_interval: 50ms
              lambda: |-
                const int   SIDE_PARAM = {SIDE};
                const float T_MIN      = {MIN_PCT};
                const float T_MAX      = {MAX_PCT};
                const float SPD        = {SPEED};
                const int   DENS       = {DENSITY};
                const uint8_t FI = (uint8_t)(4.0f/SPD  < 1.0f ? 1 : (int)(4.0f/SPD));
                const uint8_t HO = (uint8_t)(2.0f/SPD  < 1.0f ? 1 : (int)(2.0f/SPD));
                const uint8_t FO = (uint8_t)(18.0f/SPD < 1.0f ? 1 : (int)(18.0f/SPD));
                const int MAX_WAIT = (int)(28.0f/SPD) + 1;
                struct Spark { uint8_t state, step, wait; float peak; };
                static Spark sa[6] = {}, sb[6] = {};
                static bool inited = false;
                if (!inited) {
                  inited = true;
                  for (int i = 0; i < 6; i++) {
                    sa[i].wait = (uint8_t)(3 + i * 7);
                    sb[i].wait = (uint8_t)(5 + i * 8);
                  }
                }
                auto run = [&](Spark* sp, int n) -> float {
                  float tot = T_MIN;
                  for (int i = 0; i < n && i < 6; i++) {
                    float b = 0.0f;
                    Spark& s = sp[i];
                    switch (s.state) {
                      case 0:
                        if (++s.step >= s.wait) {
                          s.state = 1; s.step = 0;
                          s.peak = T_MIN + (T_MAX - T_MIN) * (0.3f + (rand()%700)*0.001f);
                          s.wait = (uint8_t)(5 + rand() % (MAX_WAIT > 1 ? MAX_WAIT : 2));
                        }
                        break;
                      case 1:
                        b = s.peak * s.step / (float)FI;
                        if (++s.step > FI) { s.state = 2; s.step = 0; }
                        break;
                      case 2:
                        b = s.peak;
                        if (++s.step > HO) { s.state = 3; s.step = 0; }
                        break;
                      case 3:
                        b = s.peak * (1.0f - s.step / (float)FO);
                        if (++s.step > FO) { s.state = 0; s.step = 0; }
                        break;
                    }
                    tot += b;
                  }
                  return tot > T_MAX ? T_MAX : tot;
                };
                auto duty_of = [](float b) -> uint32_t {
                  if (b < 0.0f) b = 0.0f; if (b > 1.0f) b = 1.0f;
                  uint32_t d = (uint32_t)(powf(b, 2.2f) * 2048.0f);
                  return d > 2048u ? 2048u : d;
                };
                float brt_a = T_MIN, brt_b = T_MIN;
                if      (SIDE_PARAM == 0) { brt_a = run(sa, DENS); }
                else if (SIDE_PARAM == 1) { brt_b = run(sb, DENS); }
                else                      { brt_a = run(sa, DENS); brt_b = run(sb, DENS); }
                ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_A}, duty_of(brt_a));
                ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_A});
                ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_B}, duty_of(brt_b));
                ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_B});
```

Notes:
- `random` (SIDE_PARAM 4) falls through to the `else` branch: both `sa` and `sb` arrays run independently. Because the two arrays have different staggered initial wait values, they fire at different times, giving an unpredictable organic mix across both sides.
- `density` is clamped to 6 by the `i < 6` guard even if the user sets `DENSITY > 6`.

- [ ] **Step 1: Replace the Twinkle lambda block**

Open `data/modules/drv8833-led-mono.yaml` and replace the entire old Twinkle block with the Dual Twinkle block above.

- [ ] **Step 2: Verify no raw `{` placeholders remain from the old lambda**

```bash
grep "id({ID}" data/modules/drv8833-led-mono.yaml
```

Expected: no output (the `id({ID}_mono_out).set_level()` call is gone).

- [ ] **Step 3: Build + test**

```bash
go build ./...
go test ./internal/esphome/... -v
```

Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add data/modules/drv8833-led-mono.yaml
git commit -m "feat: replace hardcoded Twinkle with Dual Twinkle (parameterized, dual-side)"
```

---

## Task 8: Full verification

- [ ] **Step 1: Run all tests**

```bash
cd /home/Karthangar/Projets/MatterESP32Multicontroller
go test ./... 2>&1
```

Expected: all pass, zero failures.

- [ ] **Step 2: Audit placeholders in the module file**

```bash
grep -oP '\{[A-Z_]+\}' data/modules/drv8833-led-mono.yaml | sort -u
```

Expected set — every placeholder must match a pin/config role or an effect param ID (uppercased):

```
{AIN1}
{AIN2}
{DENSITY}
{FLARE_RATE_HZ}
{FLASH_MS}
{ID}
{LEDC_CHAN_A}
{LEDC_CHAN_B}
{LEDC_TIMER}
{MAX_PCT}
{MIN_PCT}
{NAME}
{PAUSE_MAX_MS}
{PAUSE_MIN_MS}
{PERIOD_MS}
{SIDE}
{SMOOTHING}
{SPEED}
{SPEED_HZ}
{WIND}
```

If any unexpected placeholder appears, it is a typo — fix it in the module YAML before proceeding.

- [ ] **Step 3: Docker build**

```bash
docker compose up --build -d 2>&1 | tail -20
```

Expected: clean build, both containers started.

- [ ] **Step 4: Commit any fixes from Step 2**

If Step 2 found and you fixed typos:

```bash
git add data/modules/drv8833-led-mono.yaml
git commit -m "fix: typo in drv8833-led-mono effect placeholder"
```

---

## Verification Checklist

1. `go test ./...` — all green.
2. `go build ./...` — clean.
3. Docker containers start without error.
4. Audit grep finds only the expected placeholder set.
5. A job created with `effect_params: {"SIDE": "3", "MIN_PCT": "0.05", "MAX_PCT": "0.9", "FLASH_MS": "60", "PAUSE_MIN_MS": "50", "PAUSE_MAX_MS": "400"}` compiles via ESPHome without C++ errors (requires a real ESPHome compile test against a device).
