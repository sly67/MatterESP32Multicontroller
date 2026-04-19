#include <stdio.h>
#include <string.h>
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/event_groups.h"
#include "esp_log.h"
#include "esp_system.h"
#include "esp_ota_ops.h"
#include "esp_wifi.h"
#include "esp_event.h"
#include "esp_netif.h"
#include "nvs_flash.h"
#include "nvs.h"

#include "nvs_config.h"
#include "drv8833.h"
#include "matter_app.h"
#include "console_cmd.h"

static const char *TAG = "app";

#define WIFI_CONNECTED_BIT  BIT0
#define WIFI_FAIL_BIT       BIT1
#define WIFI_MAX_RETRIES    10

static EventGroupHandle_t s_wifi_events;
static int s_wifi_retries = 0;

static void wifi_event_handler(void *arg, esp_event_base_t base,
                                int32_t id, void *data)
{
    if (base == WIFI_EVENT && id == WIFI_EVENT_STA_START) {
        esp_wifi_connect();
    } else if (base == WIFI_EVENT && id == WIFI_EVENT_STA_CONNECTED) {
        ESP_LOGI(TAG, "associated — starting DHCP");
        esp_netif_t *netif = esp_netif_get_default_netif();
        if (netif) esp_netif_dhcpc_start(netif);
    } else if (base == WIFI_EVENT && id == WIFI_EVENT_STA_DISCONNECTED) {
        wifi_event_sta_disconnected_t *disc = (wifi_event_sta_disconnected_t *)data;
        ESP_LOGW(TAG, "disconnected — reason=%d (0x%02x) rssi=%d",
                 disc->reason, disc->reason, disc->rssi);
        if (s_wifi_retries < WIFI_MAX_RETRIES) {
            esp_wifi_connect();
            s_wifi_retries++;
            ESP_LOGW(TAG, "WiFi retry %d/%d", s_wifi_retries, WIFI_MAX_RETRIES);
        } else {
            xEventGroupSetBits(s_wifi_events, WIFI_FAIL_BIT);
        }
    } else if (base == IP_EVENT && id == IP_EVENT_STA_GOT_IP) {
        ip_event_got_ip_t *ev = (ip_event_got_ip_t *)data;
        ESP_LOGI(TAG, "got IP: " IPSTR, IP2STR(&ev->ip_info.ip));
        s_wifi_retries = 0;
        xEventGroupSetBits(s_wifi_events, WIFI_CONNECTED_BIT);
    }
}

static esp_err_t wifi_connect(const char *ssid, const char *password)
{
    s_wifi_retries = 0;   // reset before every connection attempt
    s_wifi_events = xEventGroupCreate();

    esp_netif_create_default_wifi_sta();

    wifi_init_config_t cfg = WIFI_INIT_CONFIG_DEFAULT();
    ESP_ERROR_CHECK(esp_wifi_init(&cfg));

    esp_event_handler_instance_t inst_wifi, inst_ip;
    ESP_ERROR_CHECK(esp_event_handler_instance_register(
        WIFI_EVENT, ESP_EVENT_ANY_ID, &wifi_event_handler, NULL, &inst_wifi));
    ESP_ERROR_CHECK(esp_event_handler_instance_register(
        IP_EVENT, IP_EVENT_STA_GOT_IP, &wifi_event_handler, NULL, &inst_ip));

    wifi_config_t wifi_cfg = {};
    strncpy((char *)wifi_cfg.sta.ssid,     ssid,     sizeof(wifi_cfg.sta.ssid) - 1);
    strncpy((char *)wifi_cfg.sta.password, password, sizeof(wifi_cfg.sta.password) - 1);
    wifi_cfg.sta.threshold.authmode = WIFI_AUTH_WPA2_PSK;
    wifi_cfg.sta.pmf_cfg.capable  = true;
    wifi_cfg.sta.pmf_cfg.required = false;

    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_STA));
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_STA, &wifi_cfg));
    ESP_ERROR_CHECK(esp_wifi_start());

    ESP_LOGI(TAG, "connecting to SSID: %s  pass_len=%d  authmode=WPA2", ssid, (int)strlen(password));

    EventBits_t bits = xEventGroupWaitBits(s_wifi_events,
        WIFI_CONNECTED_BIT | WIFI_FAIL_BIT, pdFALSE, pdFALSE,
        pdMS_TO_TICKS(60000));

    if (bits & WIFI_CONNECTED_BIT) return ESP_OK;
    ESP_LOGE(TAG, "WiFi connection failed");
    return ESP_FAIL;
}

static esp_err_t init_module(const device_cfg_t *cfg)
{
    if (cfg->module_count == 0) {
        ESP_LOGW(TAG, "no modules configured");
        return ESP_OK;
    }

    const module_cfg_t *mod = &cfg->modules[0];
    ESP_LOGI(TAG, "module[0] type=%s ep=%s", mod->type, mod->ep_name);

    if (strcmp(mod->type, "drv8833") == 0) {
        drv8833_cfg_t drv = {
            .gpio_ain1 = module_gpio_for_pin(mod, "AIN1"),
            .gpio_ain2 = module_gpio_for_pin(mod, "AIN2"),
        };
        ESP_LOGI(TAG, "drv8833 AIN1=GPIO%d AIN2=GPIO%d", drv.gpio_ain1, drv.gpio_ain2);
        return drv8833_init(&drv);
    }

    ESP_LOGW(TAG, "unknown module type: %s", mod->type);
    return ESP_OK;
}

extern "C" void app_main(void)
{
    const esp_app_desc_t *app = esp_app_get_description();
    ESP_LOGI(TAG, "MatterHub firmware %s", app->version);

    // Init NVS — esp-matter also uses NVS, so init before anything else
    esp_err_t err = nvs_flash_init();
    if (err == ESP_ERR_NVS_NO_FREE_PAGES || err == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        // Only erase if NVS is corrupt (should not happen with our flashed partition)
        ESP_LOGW(TAG, "NVS corrupt, erasing");
        ESP_ERROR_CHECK(nvs_flash_erase());
        err = nvs_flash_init();
    }
    ESP_ERROR_CHECK(err);

    // Load our device config from NVS
    device_cfg_t cfg;
    ESP_ERROR_CHECK(nvs_config_load(&cfg));

    // Init hardware module (DRV8833 PWM)
    ESP_ERROR_CHECK(init_module(&cfg));

    // Console starts immediately — usable without WiFi or Matter.
    console_start();

    if (cfg.wifi_ssid[0] != '\0') {
        ESP_ERROR_CHECK(esp_netif_init());
        ESP_ERROR_CHECK(esp_event_loop_create_default());
        esp_err_t wifi_err = wifi_connect(cfg.wifi_ssid, cfg.wifi_pass);
        if (wifi_err == ESP_OK) {
            esp_err_t matter_err = matter_app_start(&cfg);
            if (matter_err != ESP_OK)
                ESP_LOGE(TAG, "Matter failed (%s)", esp_err_to_name(matter_err));
        } else {
            ESP_LOGW(TAG, "WiFi unavailable (%s) — Matter disabled, console ready",
                     esp_err_to_name(wifi_err));
        }
    } else {
        ESP_LOGW(TAG, "no WiFi SSID provisioned — console only");
    }

    vTaskSuspend(NULL); // console task keeps running
}
