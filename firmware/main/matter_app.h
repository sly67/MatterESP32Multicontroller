#pragma once
#include "nvs_config.h"
#include "esp_err.h"

// Initialize and start the Matter stack.
// Stores discriminator/passcode in chip-config NVS namespace so
// the CHIP stack picks them up before commissioning.
esp_err_t matter_app_start(const device_cfg_t *cfg);
