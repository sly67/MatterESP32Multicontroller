# DRV8833 Mono Strip — Brightness Cutoff & Gamma Configuration Design

## Goal

Add three per-job configurable io config params to `drv8833-led-mono`:

1. **`GAMMA`** — the gamma exponent (replaces hardcoded `2.2f` everywhere)
2. **`CUTOFF_PCT`** — minimum non-zero brightness floor; any non-zero request below this value is clamped up; a 0% request still goes to 0%
3. **`CUTOFF_AFTER_GAMMA`** — selects where the cutoff is applied: before gamma (brightness space) or after gamma (duty space)

Applies to: normal dimming, all built-in ESPHome effects (strobe, pulse, flicker), the legacy Twinkle effect, and all five custom dual-side lambda effects.

---

## Behaviour

```
cutoff_before_gamma (CUTOFF_AFTER_GAMMA = 0):
  level = (state > 0 && state < CUTOFF_PCT) ? CUTOFF_PCT : state
  duty  = powf(level, GAMMA) * 2048

cutoff_after_gamma (CUTOFF_AFTER_GAMMA = 1):
  duty     = powf(state, GAMMA) * 2048
  min_duty = CUTOFF_PCT * 2048
  duty     = (duty > 0 && duty < min_duty) ? min_duty : duty
```

`state = 0` always produces `duty = 0` in both modes.

**Interpretation of CUTOFF_PCT:**
- Mode 0 (before): CUTOFF_PCT is a linear brightness fraction. `0.20` means "clamp to 20% brightness" → after gamma 2.2 that becomes ~3.3% duty.
- Mode 1 (after): CUTOFF_PCT is a physical duty fraction. `0.20` means "minimum 20% of max hardware output" regardless of gamma.

Since `{CUTOFF_AFTER_GAMMA}` is a compile-time constant after substitution (`0` or `1`), the compiler dead-code-eliminates the unused branch — no runtime cost.

---

## Architecture

Single file change: `data/modules/drv8833-led-mono.yaml`. No Go code changes.

All three params are `type: config` io entries (same pattern as `LEDC_TIMER`, `LEDC_CHAN_A`, `LEDC_CHAN_B`). They are substituted by the existing pin pass in the assembler. Defaults make the feature a no-op when unset: gamma stays 2.2, cutoff 0.0 (disabled), order 0 (before gamma).

---

## Changes to `data/modules/drv8833-led-mono.yaml`

### 1. New io entries (three)

```yaml
- id: GAMMA
  type: config
  label: "Gamma correction exponent"
  default: "2.2"

- id: CUTOFF_PCT
  type: config
  label: "Minimum non-zero brightness (0.0 = disabled)"
  default: "0.0"

- id: CUTOFF_AFTER_GAMMA
  type: config
  label: "Apply cutoff after gamma? (0 = brightness clamp, 1 = duty clamp)"
  default: "0"
```

### 2. Output component `write_action` lambda

Replace the gamma/duty block with:

```cpp
float level = state;
if ({CUTOFF_AFTER_GAMMA} == 0 && level > 0.0f && level < {CUTOFF_PCT}) level = {CUTOFF_PCT};
float g = powf(level, {GAMMA});
uint32_t duty = (uint32_t)(g * 2048.0f);
if (duty > 2048) duty = 2048;
if ({CUTOFF_AFTER_GAMMA} == 1) {
  const uint32_t min_d = (uint32_t)({CUTOFF_PCT} * 2048.0f);
  if (duty > 0 && duty < min_d) duty = min_d;
}
ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_A}, duty);
ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_A});
ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_B}, duty);
ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_B});
```

Covers: normal brightness control + built-in strobe/pulse/flicker + legacy Twinkle (all go through `set_level()` → `write_action`).

### 3. `duty_of` helper in all five custom lambda effects

Replace the existing `duty_of` closure in each of: Dual Strobe, Dual Breathing, Dual Flicker, Dual Flame, Dual Twinkle:

```cpp
auto duty_of = [](float b) -> uint32_t {
  if ({CUTOFF_AFTER_GAMMA} == 0 && b > 0.0f && b < {CUTOFF_PCT}) b = {CUTOFF_PCT};
  if (b < 0.0f) b = 0.0f; if (b > 1.0f) b = 1.0f;
  uint32_t d = (uint32_t)(powf(b, {GAMMA}) * 2048.0f);
  if (d > 2048u) d = 2048u;
  if ({CUTOFF_AFTER_GAMMA} == 1) {
    const uint32_t min_d = (uint32_t)({CUTOFF_PCT} * 2048.0f);
    if (d > 0 && d < min_d) d = min_d;
  }
  return d;
};
```

---

## API Usage

Pass the params in the `pins` map of the component config:

```json
{
  "type": "drv8833-led-mono",
  "pins": {
    "AIN1": "GPIO0",
    "AIN2": "GPIO1",
    "LEDC_TIMER": "1",
    "LEDC_CHAN_A": "2",
    "LEDC_CHAN_B": "3",
    "GAMMA": "2.2",
    "CUTOFF_PCT": "0.20",
    "CUTOFF_AFTER_GAMMA": "0"
  }
}
```

Omitting any of the three params uses the default (gamma 2.2, cutoff disabled, before-gamma order).

---

## UI Changes

### New io types in `internal/yamldef/types.go`

Two new io type constants (alongside existing `IOTypeConfig`):

```go
IOTypeFloat  = "float"   // number input with step/min/max hints
IOTypeSelect = "select"  // dropdown with labeled options list
```

### Extended `IOPin` struct in `internal/yamldef/types.go`

Add optional fields (used as UI hints only — assembler ignores them):

```go
type IOPin struct {
    ID      string     `yaml:"id"              json:"id"`
    Type    string     `yaml:"type"            json:"type"`
    Label   string     `yaml:"label"           json:"label"`
    Default string     `yaml:"default,omitempty" json:"default,omitempty"`
    Step    float64    `yaml:"step,omitempty"  json:"step,omitempty"`
    Min     *float64   `yaml:"min,omitempty"   json:"min,omitempty"`
    Max     *float64   `yaml:"max,omitempty"   json:"max,omitempty"`
    Options []IOOption `yaml:"options,omitempty" json:"options,omitempty"`
    Constraints Constraints `yaml:"constraints,omitempty" json:"constraints,omitempty"`
}

type IOOption struct {
    Value string `yaml:"value" json:"value"`
    Label string `yaml:"label" json:"label"`
}
```

### Validator in `internal/yamldef/module.go`

Add `IOTypeFloat` and `IOTypeSelect` to `validIOTypes`.

### Flash.svelte — both wizards (browser flash step 2 + server flash step 2)

The pin rendering block currently has two branches (`config` → text input, else → GPIO dropdown). Replace with four branches:

```svelte
<span class="w-24 font-mono truncate">{pinDef?.label ?? role}</span>
{#if pinDef?.type === 'config'}
  <input class="input input-bordered input-xs flex-1" type="text"
    placeholder="value" bind:value={comp.pins[role]} />
{:else if pinDef?.type === 'float'}
  <input class="input input-bordered input-xs flex-1" type="number"
    step={pinDef.step || 'any'}
    min={pinDef.min ?? ''}
    max={pinDef.max ?? ''}
    bind:value={comp.pins[role]} />
{:else if pinDef?.type === 'select'}
  <select class="select select-bordered select-xs flex-1" bind:value={comp.pins[role]}>
    {#each (pinDef.options || []) as opt}
      <option value={opt.value}>{opt.label}</option>
    {/each}
  </select>
{:else}
  <select class="select select-bordered select-xs flex-1" bind:value={comp.pins[role]}>
    <option value="">Select GPIO…</option>
    {#each boardGpios(board) as gpio}
      <option value={gpio}>{gpio}</option>
    {/each}
  </select>
{/if}
```

Note: the label span now shows `pinDef?.label ?? role` so GAMMA shows "Gamma correction" instead of the raw key. The `boardGpios` call uses the correct board variable (`bfEspBoard` in browser wizard, `espBoard` in server wizard).

---

## Out of Scope

- Applying these params to other modules.
- A separate maximum brightness cap.
- Per-effect gamma overrides.
