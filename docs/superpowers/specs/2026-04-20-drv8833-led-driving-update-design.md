# DRV8833 LED Driving Update — Design Spec

## Goal

Update the DRV8833 warm-white LED strip driver with two improvements shared across both module variants: perceptually smooth gamma-corrected dimming (eliminating visible stepping at low brightness), and a new MONO module that drives an anti-parallel single-color strip by alternating current direction at high frequency so both halves of the strip light simultaneously.

## Context

The DRV8833 H-bridge can drive an LED strip wired in anti-parallel across AOUT1/AOUT2. In CWWW mode the two halves are different color temperatures (warm/cold), controlled independently. In MONO mode both halves are the same color, so the strip must be driven alternately in both directions at high frequency — otherwise only one physical half lights at a time.

---

## Module 1 — `drv8833-led.yaml` (CWWW, updated)

### Changes

**Gamma correction**
Set `gamma_correct: 2.2` on the `cwww` light (was `1`). ESPHome applies this curve to both warm and cold channels before computing LEDC duty. No code change needed.

**Resolution and frequency**
Change `frequency: 25000Hz` → `15000Hz` and `resolution_bits: 11` → `12` on both LEDC outputs. This doubles the step count from 2048 to 4096 ticks, halving visible steps at low brightness. 15 kHz is fully inaudible and keeps the hpoint math clean (half-period = 2048 ticks at 12-bit).

**LEDC substitution variables**
Move the LEDC timer and channel IDs out of hardcoded values into YAML substitution variables so the module can be reused with other controllers without editing the module file itself.

Variables exposed (with C3 defaults):
- `${ledc_timer}` — default `0`
- `${ledc_channel_ain1}` — default `0`
- `${ledc_channel_ain2}` — default `1`

**Twinkle effect**
Unchanged. The existing lambda alternates `ain1`/`ain2` duty levels to produce a side-switching shimmer; this remains correct for the CWWW use case.

**Matter**
Unchanged: `color_temperature_light` with `on_off`, `level_control`, `color_temperature`.

---

## Module 2 — `drv8833-led-mono.yaml` (new)

### Purpose

Drive a single-color anti-parallel LED strip connected across AOUT1/AOUT2 so that both physical halves light at the same perceived brightness. AIN1 drives the forward half, AIN2 the reverse half, alternating at 15 kHz with a 180° phase offset enforced by the LEDC `hpoint` register.

### Driving principle

Both AIN1 and AIN2 are assigned to two LEDC channels on the **same timer**. AIN1 has `hpoint=0`; AIN2 has `hpoint=period/2` (2048 ticks at 12-bit). Each channel's duty is capped at `period/2`. Because AIN2 only goes HIGH starting at the midpoint of the period, and neither channel can exceed half the period, overlap is structurally impossible — no dead time register or shoot-through guard needed beyond the DRV8833's internal protection.

```
Period = 4096 ticks (15 kHz, 12-bit):

AIN1:  |██████░░░░░░░░░░|   hpoint=0,    duty = B×2048
AIN2:  |░░░░░░░░██████░░|   hpoint=2048, duty = B×2048

B=1.0 → each half at 50%, full strip perceived as 100%
B=0.5 → each half at 25%, full strip perceived as 50%
```

### Gamma correction

Gamma 2.2 is applied inside the C++ component: `float corrected = powf(state, 2.2f)`. The ESPHome `monochromatic` light YAML sets `gamma_correct: 1.0` to prevent double-correction.

### C++ custom component

A single class `Drv8833MonoOutput` extending `output::FloatOutput` and `Component`:

**Constructor parameters (set from YAML):**
- `ain1_pin` — GPIO number for AIN1
- `ain2_pin` — GPIO number for AIN2
- `ledc_timer` — LEDC timer index (0–3)
- `ledc_channel_a` — LEDC channel for AIN1 (0–5 on C3)
- `ledc_channel_b` — LEDC channel for AIN2 (0–5 on C3)
- `frequency_hz` — PWM frequency (default 15000)
- `resolution_bits` — LEDC resolution (default 12)

**`setup()`:**
1. Configure the shared LEDC timer (frequency, resolution, auto clock)
2. Configure AIN1 channel: `hpoint=0`, `duty=0`
3. Configure AIN2 channel: `hpoint=period/2`, `duty=0`

**`write_state(float state)`:**
1. Apply gamma: `float g = powf(state, 2.2f)`
2. Compute half-period: `uint32_t half = (1u << resolution_bits) / 2` (= 2048 at 12-bit)
3. Compute duty: `uint32_t duty = (uint32_t)(g * half)`
4. Set duty on both channels via `ledc_set_duty` + `ledc_update_duty`

Uses ESP-IDF LEDC API directly (`driver/ledc.h`). No FreeRTOS or interrupt dependencies.

### YAML substitution variables

```yaml
substitutions:
  ledc_timer:     "1"   # separate timer from CWWW default (0)
  ledc_channel_a: "2"
  ledc_channel_b: "3"
  frequency_hz:   "15000"
  resolution_bits: "12"
```

### Light platform

```yaml
light:
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
          // Three independent sparks summed and clamped to [0, 1]
          // Each spark cycles: IDLE → ATTACK → HOLD → DECAY → IDLE
          // state: 0=idle, 1=attack, 2=hold, 3=decay
          // (full lambda body defined in implementation plan)
```

### Twinkle effect (MONO)

Three independent sparks, each with a randomised phase offset, random peak brightness in [0.2, 0.6], fast attack (~4 update steps) and slow decay (~20 update steps). Output is the sum of all three sparks, clamped to 1.0. Updates every 50 ms via the lambda `update_interval`.

```
spark state machine (per spark):
  IDLE  → wait random 0–30 steps → ATTACK
  ATTACK → ramp up to peak over FADE_IN steps
  HOLD  → hold peak for 1–3 steps
  DECAY → ramp down to 0 over FADE_OUT steps → IDLE
```

Combined output drives the single `write()` call on the `monochromatic` output, which feeds the C++ component.

### Matter

`endpoint_type: dimmable_light` with behaviors `[on_off, level_control]`. No color temperature slider (single color).

---

## Template — `drv8833-mono-strip-c3.yaml` (new)

```yaml
id: drv8833-mono-strip-c3
board: esp32-c3
modules:
  - module: drv8833-led-mono
    endpoint_name: "Mono LED Strip"
    pins:
      AIN1: GPIO0
      AIN2: GPIO1
```

Provides default LEDC timer/channel assignments (timer=1, channels 2+3) that don't collide with the CWWW defaults (timer=0, channels 0+1).

---

## File Summary

| File | Action |
|------|--------|
| `data/modules/drv8833-led.yaml` | Modify — gamma 2.2, 15kHz/12-bit, LEDC substitution vars |
| `data/modules/drv8833-led-mono.yaml` | Create — C++ component, monochromatic light, MONO Twinkle |
| `data/templates/drv8833-mono-strip-c3.yaml` | Create — default C3 pin/LEDC assignments for mono module |

No changes to Go backend, API, or frontend.

---

## Testing

1. **CWWW**: Flash `drv8833-bicolor-strip-c3` template → verify smooth dimming from 1% to 100% with no visible steps; verify warm/cold color mixing; verify Twinkle side-switches correctly.
2. **MONO**: Flash `drv8833-mono-strip-c3` template → verify full strip lights (both halves); verify no flicker at any brightness; verify smooth dimming; verify Twinkle sparks are random and visually natural.
3. **LEDC collision check**: Flash both modules on same device (different channels/timers) → verify no interference.
