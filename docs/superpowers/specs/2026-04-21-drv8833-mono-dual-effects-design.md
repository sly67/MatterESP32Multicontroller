# DRV8833 Mono Strip — Dual-Side Effects Design

## Goal

Add four parameterised effects to the `drv8833-led-mono` module that control the left side (LEDC_CHAN_A / AIN1) and right side (LEDC_CHAN_B / AIN2) independently: **Dual Strobe**, **Dual Breathing**, **Dual Flicker**, and **Dual Flame**. Extend the ESPHome assembler to support effect-param substitution so each effect's behaviour can be tuned at compile time via the existing job-creation API.

---

## Background: Hardware Constraints

- 12-bit LEDC resolution, max duty **2048** (50 % of 4096) due to bipolar drive.
- LEDC_CHAN_A (AIN1, hpoint = 0) → left side of strip.
- LEDC_CHAN_B (AIN2, hpoint = 2048) → right side of strip.
- Effects must call `ledc_set_duty` / `ledc_update_duty` directly on each channel to set sides independently; calling `id({ID}_mono_out).set_level()` always sets both channels to the same value.
- Gamma correction (2.2) must be applied manually in any lambda that calls `ledc_set_duty` directly: `uint32_t duty = (uint32_t)(powf(brightness, 2.2f) * 2048.0f)`.
- Max duty clamp: `if (duty > 2048) duty = 2048;`

---

## Architecture

### 1. Assembler Extension — Effect Param Substitution

**Current:** `ComponentConfig` carries `Type`, `Name`, `Pins`. The assembler substitutes `{NAME}`, `{ID}`, and `{ROLE}` from Pins.

**New:** Add `EffectParams map[string]string` to `ComponentConfig`. After pin substitution, the assembler substitutes every key in `EffectParams` as `{KEY}` → value in the module template.

Effect params use **SCREAMING_SNAKE_CASE** to avoid collisions with pin roles. Each effect's params are namespaced by effect: `STROBE_MIN_PCT`, `BREATHING_SIDE`, `FLAME_WIND`, etc.

**Default resolution:** The effect YAML files define param defaults. When the API creates a `ComponentConfig`, it fills in defaults for any unspecified params from the effect YAML. The assembler performs straight substitution only — it does not read effect files.

**Side enum encoding** (shared across all four effects):

| Value | Meaning |
|-------|---------|
| 0 | left only |
| 1 | right only |
| 2 | both (same value each tick) |
| 3 | alternating (anti-phase) |
| 4 | random (re-rolled each cycle; not available for Flame) |

### 2. Effect Metadata YAML Files

Four new files in `data/effects/`:

- `strobe-dual-effect.yaml`
- `breathing-dual-effect.yaml`
- `flicker-dual-effect.yaml`
- `flame-dual-effect.yaml`

Each declares `compatible_with: [drv8833-led-mono]` and lists params with IDs, types, defaults, and ranges. These are the source of truth for defaults used by the API layer.

### 3. Lambda Effects in drv8833-led-mono.yaml

Four new `lambda:` blocks added to the `effects:` list in the light component template. All run at a fixed `update_interval: 10ms` (internal step counters handle timing). Param placeholders (`{STROBE_MIN_PCT}` etc.) are resolved by the assembler at build time.

---

## Effect Specifications

### Common Helpers (appear in every lambda)

```cpp
// Apply gamma and clamp, returns duty in [0, 2048]
auto duty_of = [](float b) -> uint32_t {
  uint32_t d = (uint32_t)(powf(b < 0.0f ? 0.0f : b, 2.2f) * 2048.0f);
  return d > 2048 ? 2048 : d;
};
// Write duties to both channels
auto set_sides = [&](uint32_t da, uint32_t db) {
  ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_A}, da);
  ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_A});
  ledc_set_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_B}, db);
  ledc_update_duty(LEDC_LOW_SPEED_MODE, (ledc_channel_t){LEDC_CHAN_B});
};
```

---

### Dual Strobe

**Effect file:** `data/effects/strobe-dual-effect.yaml`

| Param ID | Type | Default | Min | Max | Description |
|----------|------|---------|-----|-----|-------------|
| STROBE_SIDE | enum(int) | 4 | — | — | 0=left 1=right 2=both 3=alternating 4=random |
| STROBE_MIN_PCT | float | 0.0 | 0.0 | 1.0 | Floor brightness |
| STROBE_MAX_PCT | float | 1.0 | 0.0 | 1.0 | Peak brightness |
| STROBE_FLASH_MS | int | 80 | 10 | 2000 | Flash ON duration (ms) |
| STROBE_PAUSE_MIN_MS | int | 100 | 0 | 5000 | Min dark interval (ms) |
| STROBE_PAUSE_MAX_MS | int | 600 | 0 | 5000 | Max dark interval (ms) |

**Lambda logic (10 ms tick):**

```
State: flash_on (bool), current_side (0-2), step, pause_ticks_remaining
flash_steps  = STROBE_FLASH_MS / 10
pause_range  = STROBE_PAUSE_MAX_MS - STROBE_PAUSE_MIN_MS (clamped >= 0)

Each tick:
  if in pause:
    set both sides to duty_of(STROBE_MIN_PCT)
    decrement pause_ticks_remaining
    if pause_ticks_remaining == 0: enter flash phase, roll side if SIDE==4
  else if in flash:
    left  = (current_side==0||current_side==2) ? STROBE_MAX_PCT : STROBE_MIN_PCT
    right = (current_side==1||current_side==2) ? STROBE_MAX_PCT : STROBE_MIN_PCT
    set_sides(duty_of(left), duty_of(right))
    if ++step >= flash_steps:
      enter pause phase
      pause_ticks_remaining = (STROBE_PAUSE_MIN_MS + rand()%pause_range) / 10

Side roll (random): current_side = rand() % 3  → 0, 1, or 2
Alternating: current_side toggles between 0 and 1 each flash
```

---

### Dual Breathing

**Effect file:** `data/effects/breathing-dual-effect.yaml`

| Param ID | Type | Default | Min | Max | Description |
|----------|------|---------|-----|-----|-------------|
| BREATHING_SIDE | enum(int) | 2 | — | — | 0=left 1=right 2=both 3=alternating 4=random |
| BREATHING_MIN_PCT | float | 0.0 | 0.0 | 1.0 | Breath floor |
| BREATHING_MAX_PCT | float | 1.0 | 0.0 | 1.0 | Breath peak |
| BREATHING_PERIOD_MS | int | 3000 | 200 | 20000 | Full in+out cycle (ms) |
| BREATHING_PAUSE_MIN_MS | int | 0 | 0 | 5000 | Min hold at floor between breaths (ms) |
| BREATHING_PAUSE_MAX_MS | int | 500 | 0 | 5000 | Max hold at floor between breaths (ms) |

**Lambda logic (10 ms tick):**

```
State: step, pause_remaining, current_side
half_steps  = BREATHING_PERIOD_MS / 2 / 10   (steps for fade-in, same for fade-out)
pause_range = BREATHING_PAUSE_MAX_MS - BREATHING_PAUSE_MIN_MS

Phase 0 — fade in  (step: 0 → half_steps)
Phase 1 — fade out (step: 0 → half_steps)
Phase 2 — pause    (step: 0 → pause_remaining)

Brightness of active side at phase 0, tick t:
  t_norm = t / (float)half_steps           // 0→1
  b = MIN + (MAX - MIN) * sinf(t_norm * M_PI / 2.0f)  // quarter-sine ease in

At phase 1:
  b = MIN + (MAX - MIN) * sinf((1.0f - t_norm) * M_PI / 2.0f)  // ease out

Alternating: inactive side = MIN + (MAX - MIN) - active + MIN  (mirror of active)
Random: re-roll current_side (0/1/2) at start of each new breath cycle
```

---

### Dual Flicker

**Effect file:** `data/effects/flicker-dual-effect.yaml`

| Param ID | Type | Default | Min | Max | Description |
|----------|------|---------|-----|-----|-------------|
| FLICKER_SIDE | enum(int) | 2 | — | — | 0=left 1=right 2=both 3=alternating 4=random |
| FLICKER_MIN_PCT | float | 0.1 | 0.0 | 1.0 | Floor brightness |
| FLICKER_MAX_PCT | float | 1.0 | 0.0 | 1.0 | Peak brightness |
| FLICKER_SPEED_HZ | float | 20.0 | 1.0 | 50.0 | Update rate (Hz) |
| FLICKER_SMOOTHING | float | 0.3 | 0.0 | 0.95 | EMA coefficient — 0=instant 0.95=very slow |

**Lambda logic (10 ms tick):**

```
State: target_a, target_b, current_a, current_b, tick_count
update_every = (uint32_t)(1000.0f / FLICKER_SPEED_HZ / 10.0f)  // ticks between updates

Each tick:
  current_a += (target_a - current_a) * (1.0f - FLICKER_SMOOTHING)
  current_b += (target_b - current_b) * (1.0f - FLICKER_SMOOTHING)
  if ++tick_count >= update_every:
    tick_count = 0
    range = FLICKER_MAX_PCT - FLICKER_MIN_PCT
    new_a = FLICKER_MIN_PCT + (rand() % 1000) / 1000.0f * range
    if side == 2 (both):     new_b = FLICKER_MIN_PCT + (rand()%1000)/1000.0f * range  (independent)
    if side == 3 (alternating): new_b = FLICKER_MAX_PCT - new_a + FLICKER_MIN_PCT
    if side == 4 (random): roll which channel(s) to update
    if side == 0: only update target_a; target_b = FLICKER_MIN_PCT
    if side == 1: only update target_b; target_a = FLICKER_MIN_PCT
  set_sides(duty_of(current_a), duty_of(current_b))
```

---

### Dual Flame

**Effect file:** `data/effects/flame-dual-effect.yaml`

No `random` side mode (flame is continuous and directional by nature; use `wind` for asymmetry instead).

| Param ID | Type | Default | Min | Max | Description |
|----------|------|---------|-----|-----|-------------|
| FLAME_SIDE | enum(int) | 2 | — | — | 0=left 1=right 2=both 3=alternating |
| FLAME_MIN_PCT | float | 0.15 | 0.0 | 1.0 | Ember floor |
| FLAME_MAX_PCT | float | 1.0 | 0.0 | 1.0 | Peak flare brightness |
| FLAME_SPEED | float | 1.0 | 0.1 | 5.0 | Base flame movement speed multiplier |
| FLAME_FLARE_RATE_HZ | float | 2.0 | 0.1 | 10.0 | Flares per second |
| FLAME_WIND | float | 0.0 | -1.0 | 1.0 | Brightness bias: negative=left heavier, positive=right |

**Lambda logic (50 ms tick — matches existing Twinkle pattern):**

```
State per side: base (slow drift), noise (fast random), flare (peak → decay)
flare_chance_per_tick = FLAME_FLARE_RATE_HZ * 0.05f  // probability per 50ms tick

Each tick, per active side:
  base  += (rand_centered() * 0.04f * FLAME_SPEED)   // slow random walk
  base   = clamp(base, FLAME_MIN_PCT, FLAME_MAX_PCT)
  noise  = rand_float(0.0f, 0.08f)                   // fast small perturbation
  if rand_float() < flare_chance_per_tick:
    flare = rand_float(0.3f, 0.6f)                   // launch a flare
  flare *= 0.75f                                     // exponential decay each tick
  brightness = clamp(base + noise + flare, FLAME_MIN_PCT, FLAME_MAX_PCT)

Wind bias: left_brightness  *= (1.0f - max(0, FLAME_WIND))
           right_brightness *= (1.0f + min(0, FLAME_WIND))
           (normalised so total output is unchanged at ±1 wind)

Alternating: right uses same state machine but with phase offset of ~3 ticks
```

---

## Files Changed

| File | Change |
|------|--------|
| `internal/esphome/assembler.go` | Add `EffectParams map[string]string` substitution after pin pass |
| `internal/esphome/queue.go` | `ComponentConfig` gains `EffectParams map[string]string` |
| `internal/api/jobs.go` | Pass `EffectParams` through from request JSON |
| `data/effects/strobe-dual-effect.yaml` | New — effect metadata + param defaults |
| `data/effects/breathing-dual-effect.yaml` | New — effect metadata + param defaults |
| `data/effects/flicker-dual-effect.yaml` | New — effect metadata + param defaults |
| `data/effects/flame-dual-effect.yaml` | New — effect metadata + param defaults |
| `data/modules/drv8833-led-mono.yaml` | Add 4 lambda effect blocks with `{PARAM}` placeholders |
| `internal/esphome/assembler_test.go` | Tests for effect param substitution |

---

## Out of Scope

- Web UI for selecting effects and tuning params (future feature).
- Applying multiple effects simultaneously.
- Runtime effect switching without recompile.
- Param substitution for modules other than drv8833-led-mono (generalised, but tested only here).
