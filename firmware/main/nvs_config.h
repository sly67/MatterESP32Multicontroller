#pragma once
#include <stdint.h>
#include "esp_err.h"

#define MAX_MODULES  8
#define MAX_PINS     8
#define CFG_STR_LEN  64

typedef struct {
    char id[16];    // e.g. "AIN1"
    char gpio[16];  // e.g. "GPIO0"
} pin_assignment_t;

typedef struct {
    char type[32];
    char ep_name[CFG_STR_LEN];
    char effect[32];
    pin_assignment_t pins[MAX_PINS];
    int  pin_count;
} module_cfg_t;

typedef struct {
    char wifi_ssid[CFG_STR_LEN];
    char wifi_pass[CFG_STR_LEN];
    char board_id[CFG_STR_LEN];
    char device_name[CFG_STR_LEN];
    uint16_t discriminator;
    uint32_t passcode;
    uint8_t  module_count;
    module_cfg_t modules[MAX_MODULES];
} device_cfg_t;

// Loads all config from our custom NVS namespaces.
// Call after nvs_flash_init().
esp_err_t nvs_config_load(device_cfg_t *out);

// Returns the GPIO number for a given pin ID within a module config.
// Returns -1 if not found.
int module_gpio_for_pin(const module_cfg_t *mod, const char *pin_id);
