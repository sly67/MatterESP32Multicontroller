#include "matter_app.h"
#include "drv8833.h"
#include "nvs_config.h"

#include <esp_log.h>
#include <esp_matter.h>
#include <esp_matter_endpoint.h>
#include <nvs.h>
#include <string.h>

#include <app/clusters/on-off-server/on-off-server.h>
#include <app/clusters/level-control/level-control.h>
#include <app/clusters/color-control-server/color-control-server.h>

using namespace esp_matter;
using namespace esp_matter::attribute;
using namespace esp_matter::endpoint;
using namespace chip::app::Clusters;

static const char *TAG = "matter_app";

// Cached light state — updated by attribute callback, applied to driver
static bool     s_on_off   = false;
static uint8_t  s_level    = 64;
static uint16_t s_ct_mireds = 370;  // warm default

static void apply_state()
{
    drv8833_set(s_on_off, s_level, s_ct_mireds);
}

// Called by esp-matter for every attribute write before it is committed.
static esp_err_t attribute_update_cb(attribute::callback_type_t type,
                                     uint16_t endpoint_id,
                                     uint32_t cluster_id,
                                     uint32_t attribute_id,
                                     esp_matter_attr_val_t *val,
                                     void * /*priv_data*/)
{
    if (type != PRE_UPDATE) return ESP_OK;

    if (cluster_id == OnOff::Id) {
        if (attribute_id == OnOff::Attributes::OnOff::Id) {
            s_on_off = val->val.b;
            ESP_LOGI(TAG, "OnOff → %s", s_on_off ? "ON" : "OFF");
        }
    } else if (cluster_id == LevelControl::Id) {
        if (attribute_id == LevelControl::Attributes::CurrentLevel::Id) {
            s_level = val->val.u8;
            ESP_LOGI(TAG, "Level → %d", s_level);
        }
    } else if (cluster_id == ColorControl::Id) {
        if (attribute_id == ColorControl::Attributes::ColorTemperatureMireds::Id) {
            s_ct_mireds = val->val.u16;
            ESP_LOGI(TAG, "CT → %d mireds (%s side)", s_ct_mireds,
                     s_ct_mireds >= 326 ? "warm/AIN1" : "cool/AIN2");
        }
    }

    apply_state();
    return ESP_OK;
}

static esp_err_t identification_cb(identification::callback_type_t type,
                                   uint16_t endpoint_id,
                                   uint8_t effect_id,
                                   uint8_t effect_variant,
                                   void * /*priv_data*/)
{
    // Blink both sides alternately during identify
    ESP_LOGI(TAG, "identify endpoint %d effect %d", endpoint_id, effect_id);
    return ESP_OK;
}

// Persist discriminator and passcode to CHIP's own NVS namespace so
// it uses our pre-provisioned values during commissioning.
static void store_matter_credentials(uint16_t discriminator, uint32_t passcode)
{
    nvs_handle_t h;
    if (nvs_open("chip-config", NVS_READWRITE, &h) != ESP_OK) return;
    nvs_set_u16(h, "discriminator", discriminator);
    nvs_set_u32(h, "pin-code",      passcode);
    nvs_commit(h);
    nvs_close(h);
    ESP_LOGI(TAG, "stored disc=%u passcode=%lu", discriminator, (unsigned long)passcode);
}

esp_err_t matter_app_start(const device_cfg_t *cfg)
{
    store_matter_credentials(cfg->discriminator, cfg->passcode);

    // Create root Matter node
    node::config_t node_cfg;
    node_t *node = node::create(&node_cfg, attribute_update_cb, identification_cb);
    if (!node) {
        ESP_LOGE(TAG, "failed to create Matter node");
        return ESP_FAIL;
    }

    // Create color_temperature_light endpoint
    // Warm CT (≥326 mireds) → AIN1 forward; cool CT (<326) → AIN2 reverse
    endpoint::color_temperature_light::config_t light_cfg;
    light_cfg.on_off.on_off                              = false;
    light_cfg.level_control.current_level                = 64;
    light_cfg.level_control.lighting.start_up_current_level = 64;
    light_cfg.color_control.color_mode                   = 2;  // Matter spec ColorMode: 2 = ColorTemperatureMireds
    light_cfg.color_control.enhanced_color_mode          = 2;  // EnhancedColorMode: 2 = ColorTemperatureMireds
    light_cfg.color_control.color_temperature.startup_color_temperature_mireds = 370;

    endpoint_t *ep = endpoint::color_temperature_light::create(
        node, &light_cfg, ENDPOINT_FLAG_NONE, NULL);
    if (!ep) {
        ESP_LOGE(TAG, "failed to create light endpoint");
        return ESP_FAIL;
    }

    ESP_LOGI(TAG, "starting Matter stack — disc=%u passcode=%lu",
             cfg->discriminator, (unsigned long)cfg->passcode);
    return esp_matter::start(NULL);
}
