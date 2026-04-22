# DRV8833 Mono Strip — Minimum Brightness Cutoff Design

## Goal

Add a per-job configurable brightness floor to `drv8833-led-mono` so that any non-zero brightness request is clamped up to a minimum level (e.g. 20%), while an explicit 0% request still turns the strip fully off. Applies to normal dimming, all built-in ESPHome effects (strobe, pulse, flicker), and all five custom dual-side lambda effects.

---

## Behaviour

```
output(requested) =
  0               if requested == 0
  CUTOFF_PCT      if 0 < requested < CUTOFF_PCT
  requested       if requested >= CUTOFF_PCT
```

Applied **before** gamma correction (in linear brightness space).

---

## Architecture

Single file change: `data/modules/drv8833-led-mono.yaml`. No Go code changes.

`CUTOFF_PCT` is added as a `type: config` io entry (same pattern as `LEDC_TIMER`, `LEDC_CHAN_A`, `LEDC_CHAN_B`). It is substituted by the existing pin pass in the assembler — no new substitution mechanism required. The user passes it in the `pins` map when creating a job; if omitted the default `0.0` disables the feature.

---

## Changes to `data/modules/drv8833-led-mono.yaml`

### 1. New io entry

```yaml
- id: CUTOFF_PCT
  type: config
  label: "Minimum non-zero brightness (0.0 = disabled)"
  default: "0.0"
```

### 2. Output component `write_action` lambda

Replace the brightness-to-duty block with:

```cpp
float level = state;
if (level > 0.0f && level < {CUTOFF_PCT}) level = {CUTOFF_PCT};
float g = powf(level, 2.2f);
uint32_t duty = (uint32_t)(g * 2048.0f);
if (duty > 2048) duty = 2048;
ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_A}, duty);
ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_A});
ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_B}, duty);
ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_B});
```

This covers: normal brightness control, built-in strobe, pulse, and flicker effects, and the legacy Twinkle effect (which calls `set_level()` → `write_action`).

### 3. `duty_of` helper in all five custom lambda effects

Each lambda defines its own `duty_of` closure. Add the clamp-up line as the first line inside:

```cpp
auto duty_of = [](float b) -> uint32_t {
  if (b > 0.0f && b < {CUTOFF_PCT}) b = {CUTOFF_PCT};
  if (b < 0.0f) b = 0.0f; if (b > 1.0f) b = 1.0f;
  uint32_t d = (uint32_t)(powf(b, 2.2f) * 2048.0f);
  return d > 2048u ? 2048u : d;
};
```

Affects: Dual Strobe, Dual Breathing, Dual Flicker, Dual Flame, Dual Twinkle.

---

## API Usage

No API changes. Pass `CUTOFF_PCT` in the `pins` map of the component config:

```json
{
  "type": "drv8833-led-mono",
  "pins": {
    "AIN1": "GPIO0",
    "AIN2": "GPIO1",
    "LEDC_TIMER": "1",
    "LEDC_CHAN_A": "2",
    "LEDC_CHAN_B": "3",
    "CUTOFF_PCT": "0.20"
  }
}
```

Omitting `CUTOFF_PCT` defaults to `0.0` (no floor applied).

---

## Out of Scope

- Applying the cutoff to other modules.
- A separate maximum brightness cap.
- UI for setting the cutoff.
