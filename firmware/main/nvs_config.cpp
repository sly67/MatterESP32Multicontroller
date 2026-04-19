#include "nvs_config.h"
#include "nvs.h"
#include "nvs_flash.h"
#include "esp_log.h"
#include <string.h>
#include <stdio.h>
#include <stdlib.h>

static const char *TAG = "nvs_cfg";

static esp_err_t read_str(nvs_handle_t h, const char *key, char *buf, size_t len)
{
    size_t sz = len;
    esp_err_t err = nvs_get_str(h, key, buf, &sz);
    if (err != ESP_OK) {
        ESP_LOGW(TAG, "key '%s' not found (%s)", key, esp_err_to_name(err));
        buf[0] = '\0';
    }
    return err;
}

// Parse "GPIO4" → 4, or plain "4" → 4. Returns -1 on failure.
static int parse_gpio_num(const char *s)
{
    if (!s || s[0] == '\0') return -1;
    if (strncmp(s, "GPIO", 4) == 0) return atoi(s + 4);
    return atoi(s);
}

static esp_err_t load_module(uint8_t idx, module_cfg_t *mod)
{
    char ns[16];
    snprintf(ns, sizeof(ns), "mod_%d", idx);

    nvs_handle_t h;
    esp_err_t err = nvs_open(ns, NVS_READONLY, &h);
    if (err != ESP_OK) {
        ESP_LOGW(TAG, "namespace '%s' not found", ns);
        return err;
    }

    read_str(h, "type",    mod->type,    sizeof(mod->type));
    read_str(h, "ep_name", mod->ep_name, sizeof(mod->ep_name));
    read_str(h, "effect",  mod->effect,  sizeof(mod->effect));

    // Iterate NVS entries to find pin assignments (keys starting with "p_")
    mod->pin_count = 0;
    nvs_iterator_t it = NULL;
    err = nvs_entry_find(NVS_DEFAULT_PART_NAME, ns, NVS_TYPE_STR, &it);
    while (err == ESP_OK && it != NULL) {
        nvs_entry_info_t info;
        nvs_entry_info(it, &info);
        if (strncmp(info.key, "p_", 2) == 0 && mod->pin_count < MAX_PINS) {
            pin_assignment_t *pa = &mod->pins[mod->pin_count];
            // Strip the "p_" prefix to get the pin ID
            strncpy(pa->id, info.key + 2, sizeof(pa->id) - 1);
            pa->id[sizeof(pa->id) - 1] = '\0';
            size_t sz = sizeof(pa->gpio);
            nvs_get_str(h, info.key, pa->gpio, &sz);
            ESP_LOGI(TAG, "mod_%d pin %s → %s", idx, pa->id, pa->gpio);
            mod->pin_count++;
        }
        err = nvs_entry_next(&it);
    }
    nvs_release_iterator(it);
    nvs_close(h);
    return ESP_OK;
}

esp_err_t nvs_config_load(device_cfg_t *out)
{
    memset(out, 0, sizeof(*out));
    nvs_handle_t h;
    esp_err_t err;

    // wifi
    err = nvs_open("wifi", NVS_READONLY, &h);
    if (err == ESP_OK) {
        read_str(h, "ssid", out->wifi_ssid, sizeof(out->wifi_ssid));
        read_str(h, "pass", out->wifi_pass, sizeof(out->wifi_pass));
        nvs_close(h);
    }

    // hw
    err = nvs_open("hw", NVS_READONLY, &h);
    if (err == ESP_OK) {
        read_str(h, "board", out->board_id, sizeof(out->board_id));
        nvs_close(h);
    }

    // device
    err = nvs_open("device", NVS_READONLY, &h);
    if (err == ESP_OK) {
        read_str(h, "name", out->device_name, sizeof(out->device_name));
        nvs_close(h);
    }

    // matter — discriminator and passcode
    err = nvs_open("matter", NVS_READONLY, &h);
    if (err == ESP_OK) {
        nvs_get_u16(h, "disc",     &out->discriminator);
        nvs_get_u32(h, "passcode", &out->passcode);
        nvs_close(h);
        ESP_LOGI(TAG, "matter disc=%u passcode=%lu", out->discriminator, (unsigned long)out->passcode);
    }

    // modules
    err = nvs_open("modules_cfg", NVS_READONLY, &h);
    if (err == ESP_OK) {
        nvs_get_u8(h, "count", &out->module_count);
        nvs_close(h);
    }

    for (uint8_t i = 0; i < out->module_count && i < MAX_MODULES; i++) {
        load_module(i, &out->modules[i]);
    }

    ESP_LOGI(TAG, "loaded: ssid='%s' pass_len=%d board=%s modules=%d",
             out->wifi_ssid, (int)strlen(out->wifi_pass), out->board_id, out->module_count);
    return ESP_OK;
}

int module_gpio_for_pin(const module_cfg_t *mod, const char *pin_id)
{
    for (int i = 0; i < mod->pin_count; i++) {
        if (strcmp(mod->pins[i].id, pin_id) == 0) {
            return parse_gpio_num(mod->pins[i].gpio);
        }
    }
    return -1;
}
