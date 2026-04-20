#pragma once
#include <stdint.h>
#include <stdbool.h>
#include "esp_err.h"

// DRV8833 H-bridge driver for alternate-polarity LED strip.
//
// Bridge A channel:
//   AIN1 high → forward direction LEDs light up
//   AIN2 high → reverse direction LEDs light up
//
// Both pins are PWM-capable; brightness is controlled via duty cycle.
// AIN1 and AIN2 are mutually exclusive — driving both brakes the motor/LED.

typedef struct {
    int gpio_ain1;  // -1 if not wired
    int gpio_ain2;  // -1 if not wired
} drv8833_cfg_t;

// Initialize LEDC PWM for the given GPIO pins.
esp_err_t drv8833_init(const drv8833_cfg_t *cfg);

// Apply Matter light state to the H-bridge.
//
//   on_off  : false → coast (both outputs 0)
//   level   : 0-254 Matter brightness
//   color_temperature_mireds : 153-500
//               < 326 (cool) → AIN2 side active
//               ≥ 326 (warm) → AIN1 side active
void drv8833_set(bool on_off, uint8_t level, uint16_t color_temp_mireds);

// Set raw LEDC duty on both channels (0-1023).  Bypasses the Matter model.
// Both channels can be driven simultaneously: overlap produces "brake" (0V across
// load) so no shoot-through risk; disjoint windows produce warm+cool mix.
void drv8833_set_duty(uint32_t ain1, uint32_t ain2);
