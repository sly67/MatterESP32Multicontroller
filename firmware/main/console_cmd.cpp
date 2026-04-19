#include "console_cmd.h"
#include "drv8833.h"
#include "nvs_config.h"

#include "esp_console.h"
#include "esp_log.h"
#include "esp_chip_info.h"
#include "esp_flash.h"
#include "esp_timer.h"
#include "esp_heap_caps.h"
#include "esp_wifi.h"
#include "esp_netif.h"
#include "esp_system.h"
#include "driver/gpio.h"
#include "driver/uart.h"
#include "driver/usb_serial_jtag.h"
#include "nvs_flash.h"
#include "argtable3/argtable3.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

#include <string.h>
#include <stdlib.h>

static const char *TAG = "console";

// ── Shared LED state ──────────────────────────────────────────────────────────
static bool     s_on  = false;
static uint8_t  s_lvl = 128;
static uint16_t s_ct  = 370;

static void led_apply(void) { drv8833_set(s_on, s_lvl, s_ct); }

// ═════════════════════════════════════════════════════════════════════════════
//  LED COMMANDS
// ═════════════════════════════════════════════════════════════════════════════

// ── led_on ────────────────────────────────────────────────────────────────────
static int cmd_led_on(int argc, char **argv)
{
    s_on = true; led_apply();
    printf("LED on  level=%d ct=%d mireds (%s)\n",
           s_lvl, s_ct, s_ct >= 326 ? "warm/AIN1" : "cool/AIN2");
    return 0;
}

// ── led_off ───────────────────────────────────────────────────────────────────
static int cmd_led_off(int argc, char **argv)
{
    s_on = false; led_apply();
    printf("LED off\n");
    return 0;
}

// ── led_level <0-254> ─────────────────────────────────────────────────────────
static struct { struct arg_int *v; struct arg_end *end; } level_args;

static int cmd_led_level(int argc, char **argv)
{
    if (arg_parse(argc, argv, (void **)&level_args)) {
        arg_print_errors(stdout, level_args.end, argv[0]); return 1;
    }
    int v = level_args.v->ival[0];
    if (v < 0 || v > 254) { printf("level 0-254\n"); return 1; }
    s_lvl = (uint8_t)v; led_apply();
    printf("level=%d\n", s_lvl);
    return 0;
}

// ── led_ct <153-500 mireds> ───────────────────────────────────────────────────
static struct { struct arg_int *v; struct arg_end *end; } ct_args;

static int cmd_led_ct(int argc, char **argv)
{
    if (arg_parse(argc, argv, (void **)&ct_args)) {
        arg_print_errors(stdout, ct_args.end, argv[0]); return 1;
    }
    int v = ct_args.v->ival[0];
    if (v < 153 || v > 500) { printf("ct 153-500 mireds\n"); return 1; }
    s_ct = (uint16_t)v; led_apply();
    printf("ct=%d mireds (%s)\n", s_ct, s_ct >= 326 ? "warm/AIN1" : "cool/AIN2");
    return 0;
}

// ── led_status ────────────────────────────────────────────────────────────────
static int cmd_led_status(int argc, char **argv)
{
    printf("on=%d  level=%d  ct=%d mireds (%s)\n",
           s_on, s_lvl, s_ct, s_ct >= 326 ? "warm/AIN1" : "cool/AIN2");
    return 0;
}

// ── led_fade <level> [duration_ms=1000] ──────────────────────────────────────
static struct {
    struct arg_int *level;
    struct arg_int *duration;
    struct arg_end *end;
} fade_args;

static int cmd_led_fade(int argc, char **argv)
{
    if (arg_parse(argc, argv, (void **)&fade_args)) {
        arg_print_errors(stdout, fade_args.end, argv[0]); return 1;
    }
    int target = fade_args.level->ival[0];
    int dur    = fade_args.duration->count ? fade_args.duration->ival[0] : 1000;
    if (target < 0 || target > 254)  { printf("level 0-254\n"); return 1; }
    if (dur < 100 || dur > 10000)    { printf("duration 100-10000 ms\n"); return 1; }
    int steps    = target ? target : 1;
    int delay_ms = dur / steps;
    if (delay_ms < 1) delay_ms = 1;
    printf("fade → %d over %d ms\n", target, dur);
    for (int i = 0; i <= target; i++) {
        drv8833_set(true, (uint8_t)i, s_ct);
        vTaskDelay(pdMS_TO_TICKS(delay_ms));
    }
    s_on = true; s_lvl = (uint8_t)target;
    printf("done\n");
    return 0;
}

// ── led_blink <count> [on_ms=500] [off_ms=500] ───────────────────────────────
static struct {
    struct arg_int *count;
    struct arg_int *on_ms;
    struct arg_int *off_ms;
    struct arg_end *end;
} blink_args;

static int cmd_led_blink(int argc, char **argv)
{
    if (arg_parse(argc, argv, (void **)&blink_args)) {
        arg_print_errors(stdout, blink_args.end, argv[0]); return 1;
    }
    int n      = blink_args.count->ival[0];
    int on_ms  = blink_args.on_ms->count  ? blink_args.on_ms->ival[0]  : 500;
    int off_ms = blink_args.off_ms->count ? blink_args.off_ms->ival[0] : 500;
    if (n < 1 || n > 100)             { printf("count 1-100\n"); return 1; }
    if (on_ms  < 50 || on_ms  > 5000) { printf("on_ms 50-5000\n"); return 1; }
    if (off_ms < 50 || off_ms > 5000) { printf("off_ms 50-5000\n"); return 1; }
    printf("blink %d × (%dms on / %dms off)\n", n, on_ms, off_ms);
    for (int i = 0; i < n; i++) {
        drv8833_set(true,  s_lvl, s_ct); vTaskDelay(pdMS_TO_TICKS(on_ms));
        drv8833_set(false, s_lvl, s_ct); vTaskDelay(pdMS_TO_TICKS(off_ms));
    }
    led_apply(); printf("done\n");
    return 0;
}

// ── led_pulse <cycles> [period_ms=2000] ──────────────────────────────────────
static struct {
    struct arg_int *cycles;
    struct arg_int *period;
    struct arg_end *end;
} pulse_args;

static int cmd_led_pulse(int argc, char **argv)
{
    if (arg_parse(argc, argv, (void **)&pulse_args)) {
        arg_print_errors(stdout, pulse_args.end, argv[0]); return 1;
    }
    int cycles = pulse_args.cycles->ival[0];
    int period = pulse_args.period->count ? pulse_args.period->ival[0] : 2000;
    if (cycles < 1 || cycles > 20)      { printf("cycles 1-20\n"); return 1; }
    if (period < 200 || period > 10000) { printf("period 200-10000 ms\n"); return 1; }
    const int STEPS = 50;
    int step_ms = (period / 2) / STEPS;
    if (step_ms < 1) step_ms = 1;
    printf("pulse %d cycle(s), period=%d ms\n", cycles, period);
    for (int c = 0; c < cycles; c++) {
        for (int i = 0; i <= STEPS; i++) {
            drv8833_set(true, (uint8_t)((i * s_lvl) / STEPS), s_ct);
            vTaskDelay(pdMS_TO_TICKS(step_ms));
        }
        for (int i = STEPS; i >= 0; i--) {
            drv8833_set(true, (uint8_t)((i * s_lvl) / STEPS), s_ct);
            vTaskDelay(pdMS_TO_TICKS(step_ms));
        }
    }
    led_apply(); printf("done\n");
    return 0;
}

// ── led_sweep <cycles> [period_ms=4000] ──────────────────────────────────────
// Sweeps CT from warm (450 mireds/AIN1) to cool (200 mireds/AIN2) and back.
#define SWEEP_WARM 450
#define SWEEP_COOL 200

static struct {
    struct arg_int *cycles;
    struct arg_int *period;
    struct arg_end *end;
} sweep_args;

static int cmd_led_sweep(int argc, char **argv)
{
    if (arg_parse(argc, argv, (void **)&sweep_args)) {
        arg_print_errors(stdout, sweep_args.end, argv[0]); return 1;
    }
    int cycles = sweep_args.cycles->ival[0];
    int period = sweep_args.period->count ? sweep_args.period->ival[0] : 4000;
    if (cycles < 1 || cycles > 20)       { printf("cycles 1-20\n"); return 1; }
    if (period < 400 || period > 20000)  { printf("period 400-20000 ms\n"); return 1; }
    int range   = SWEEP_WARM - SWEEP_COOL;
    int step_ms = (period / 2) / range;
    if (step_ms < 1) step_ms = 1;
    printf("sweep %d cycle(s), period=%d ms  warm(%d)→cool(%d)→warm\n",
           cycles, period, SWEEP_WARM, SWEEP_COOL);
    for (int c = 0; c < cycles; c++) {
        for (int ct = SWEEP_WARM; ct >= SWEEP_COOL; ct--) {
            drv8833_set(true, s_lvl, (uint16_t)ct); vTaskDelay(pdMS_TO_TICKS(step_ms));
        }
        for (int ct = SWEEP_COOL; ct <= SWEEP_WARM; ct++) {
            drv8833_set(true, s_lvl, (uint16_t)ct); vTaskDelay(pdMS_TO_TICKS(step_ms));
        }
    }
    led_apply(); printf("done\n");
    return 0;
}

// ── led_raw <ain1_0-1023> <ain2_0-1023> ──────────────────────────────────────
static struct {
    struct arg_int *ain1;
    struct arg_int *ain2;
    struct arg_end *end;
} raw_args;

static int cmd_led_raw(int argc, char **argv)
{
    if (arg_parse(argc, argv, (void **)&raw_args)) {
        arg_print_errors(stdout, raw_args.end, argv[0]); return 1;
    }
    int a1 = raw_args.ain1->ival[0];
    int a2 = raw_args.ain2->ival[0];
    if (a1 < 0 || a1 > 1023 || a2 < 0 || a2 > 1023) { printf("duty 0-1023\n"); return 1; }
    drv8833_set_duty((uint32_t)a1, (uint32_t)a2);
    printf("AIN1=%d  AIN2=%d\n", a1, a2);
    return 0;
}

// ═════════════════════════════════════════════════════════════════════════════
//  SYSTEM COMMANDS
// ═════════════════════════════════════════════════════════════════════════════

// ── sys_info ──────────────────────────────────────────────────────────────────
static int cmd_sys_info(int argc, char **argv)
{
    esp_chip_info_t chip;
    esp_chip_info(&chip);

    uint32_t flash_size = 0;
    esp_flash_get_size(NULL, &flash_size);

    int64_t uptime_s  = esp_timer_get_time() / 1000000LL;
    size_t  free_heap = heap_caps_get_free_size(MALLOC_CAP_DEFAULT);
    size_t  min_heap  = heap_caps_get_minimum_free_size(MALLOC_CAP_DEFAULT);

    const char *model = "unknown";
    switch ((int)chip.model) {
#ifdef CONFIG_IDF_TARGET_ESP32C3
        case CHIP_ESP32C3: model = "ESP32-C3"; break;
#endif
        default: break;
    }

    printf("Chip:       %s  rev%d  %d core(s)\n", model, chip.revision, chip.cores);
    printf("Flash:      %lu KB\n", (unsigned long)(flash_size / 1024));
    printf("Heap free:  %u B  (min ever: %u B)\n", (unsigned)free_heap, (unsigned)min_heap);
    printf("Uptime:     %lld s  (%lld h %lld m %lld s)\n",
           uptime_s, uptime_s/3600, (uptime_s%3600)/60, uptime_s%60);
    return 0;
}

// ── wifi_status ───────────────────────────────────────────────────────────────
static int cmd_wifi_status(int argc, char **argv)
{
    wifi_ap_record_t ap;
    esp_err_t err = esp_wifi_sta_get_ap_info(&ap);
    if (err != ESP_OK) {
        printf("Not associated (err=%s)\n", esp_err_to_name(err));
        return 0;
    }

    uint8_t mac[6];
    esp_wifi_get_mac(WIFI_IF_STA, mac);

    printf("SSID:   %s\n",   (char *)ap.ssid);
    printf("RSSI:   %d dBm\n", ap.rssi);
    printf("BSSID:  %02x:%02x:%02x:%02x:%02x:%02x\n",
           ap.bssid[0], ap.bssid[1], ap.bssid[2],
           ap.bssid[3], ap.bssid[4], ap.bssid[5]);
    printf("MAC:    %02x:%02x:%02x:%02x:%02x:%02x\n",
           mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]);
    printf("Chan:   %d\n", ap.primary);

    esp_netif_t *netif = esp_netif_get_default_netif();
    if (netif) {
        esp_netif_ip_info_t ip;
        esp_netif_get_ip_info(netif, &ip);
        printf("IP:     " IPSTR "\n", IP2STR(&ip.ip));
        printf("GW:     " IPSTR "\n", IP2STR(&ip.gw));
        printf("Mask:   " IPSTR "\n", IP2STR(&ip.netmask));
    }
    return 0;
}

// ── nvs_dump ──────────────────────────────────────────────────────────────────
static int cmd_nvs_dump(int argc, char **argv)
{
    device_cfg_t cfg;
    if (nvs_config_load(&cfg) != ESP_OK) {
        printf("ERROR: failed to load NVS config\n");
        return 1;
    }
    printf("WiFi SSID:     %s\n", cfg.wifi_ssid);
    int plen = (int)strlen(cfg.wifi_pass);
    if (plen == 0) {
        printf("WiFi pass:     (empty)\n");
    } else {
        // Show first 4 chars to detect corruption without exposing full password
        char preview[5] = {};
        strncpy(preview, cfg.wifi_pass, 4);
        printf("WiFi pass:     %s... (len=%d)\n", preview, plen);
    }
    printf("Board ID:      %s\n", cfg.board_id);
    printf("Device name:   %s\n", cfg.device_name);
    printf("Discriminator: %u\n", cfg.discriminator);
    printf("Passcode:      %lu\n", (unsigned long)cfg.passcode);
    printf("Modules:       %d\n", cfg.module_count);
    for (int i = 0; i < cfg.module_count; i++) {
        const module_cfg_t *m = &cfg.modules[i];
        printf("  [%d] type=%-12s ep=%-20s effect=%s\n",
               i, m->type, m->ep_name, m->effect);
        for (int j = 0; j < m->pin_count; j++) {
            printf("      %-6s → %s\n", m->pins[j].id, m->pins[j].gpio);
        }
    }
    return 0;
}

// ── wifi_scan ─────────────────────────────────────────────────────────────────
// Scans for nearby APs and prints SSID, channel, RSSI, auth mode.
static const char *auth_name(wifi_auth_mode_t m) {
    switch (m) {
    case WIFI_AUTH_OPEN:          return "OPEN";
    case WIFI_AUTH_WEP:           return "WEP";
    case WIFI_AUTH_WPA_PSK:       return "WPA";
    case WIFI_AUTH_WPA2_PSK:      return "WPA2";
    case WIFI_AUTH_WPA_WPA2_PSK:  return "WPA/WPA2";
    case WIFI_AUTH_WPA2_ENTERPRISE: return "WPA2-ENT";
    case WIFI_AUTH_WPA3_PSK:      return "WPA3";
    case WIFI_AUTH_WPA2_WPA3_PSK: return "WPA2/WPA3";
    default:                      return "OTHER";
    }
}

static int cmd_wifi_scan(int argc, char **argv)
{
    wifi_scan_config_t scan_cfg = {
        .ssid        = NULL,
        .bssid       = NULL,
        .channel     = 0,
        .show_hidden = true,
    };
    printf("Scanning...\n");
    esp_err_t err = esp_wifi_scan_start(&scan_cfg, true); // blocking
    if (err != ESP_OK) {
        printf("scan failed: %s\n", esp_err_to_name(err));
        return 1;
    }
    uint16_t count = 0;
    esp_wifi_scan_get_ap_num(&count);
    if (count == 0) { printf("No APs found.\n"); return 0; }

    wifi_ap_record_t *aps = (wifi_ap_record_t *)malloc(count * sizeof(wifi_ap_record_t));
    if (!aps) { printf("OOM\n"); return 1; }
    esp_wifi_scan_get_ap_records(&count, aps);

    printf("%-32s  Ch  RSSI  Auth\n", "SSID");
    printf("%-32s  --  ----  ----\n", "--------------------------------");
    for (int i = 0; i < count; i++) {
        printf("%-32s  %2d  %4d  %s\n",
               (char *)aps[i].ssid, aps[i].primary,
               aps[i].rssi, auth_name(aps[i].authmode));
    }
    free(aps);
    return 0;
}

// ── wifi_test <ssid> <password> ──────────────────────────────────────────────
// Reconfigures WiFi with new credentials and triggers a connection attempt.
// Useful for testing without reflashing NVS.
static struct {
    struct arg_str *ssid;
    struct arg_str *password;
    struct arg_end *end;
} wifi_test_args;

static int cmd_wifi_test(int argc, char **argv)
{
    if (arg_parse(argc, argv, (void **)&wifi_test_args)) {
        arg_print_errors(stdout, wifi_test_args.end, argv[0]); return 1;
    }
    const char *ssid = wifi_test_args.ssid->sval[0];
    const char *pass = wifi_test_args.password->sval[0];

    wifi_config_t cfg = {};
    strncpy((char *)cfg.sta.ssid,     ssid, sizeof(cfg.sta.ssid) - 1);
    strncpy((char *)cfg.sta.password, pass, sizeof(cfg.sta.password) - 1);
    cfg.sta.threshold.authmode = WIFI_AUTH_WPA2_PSK;
    cfg.sta.pmf_cfg.capable    = true;
    cfg.sta.pmf_cfg.required   = false;

    esp_err_t err = esp_wifi_disconnect();
    if (err != ESP_OK && err != ESP_ERR_WIFI_NOT_CONNECT) {
        printf("disconnect err: %s\n", esp_err_to_name(err));
    }
    err = esp_wifi_set_config(WIFI_IF_STA, &cfg);
    if (err != ESP_OK) { printf("set_config err: %s\n", esp_err_to_name(err)); return 1; }
    err = esp_wifi_connect();
    if (err != ESP_OK) { printf("connect err: %s\n", esp_err_to_name(err)); return 1; }

    printf("Connecting to '%s' (pass_len=%d) — watch logs for result\n",
           ssid, (int)strlen(pass));
    return 0;
}

// ── gpio_read <pin> ───────────────────────────────────────────────────────────
static struct { struct arg_int *pin; struct arg_end *end; } gpio_read_args;

static int cmd_gpio_read(int argc, char **argv)
{
    if (arg_parse(argc, argv, (void **)&gpio_read_args)) {
        arg_print_errors(stdout, gpio_read_args.end, argv[0]); return 1;
    }
    int pin = gpio_read_args.pin->ival[0];
    if (pin < 0 || pin > 21) { printf("pin 0-21 on ESP32-C3\n"); return 1; }
    gpio_config_t io = {
        .pin_bit_mask = (1ULL << pin),
        .mode         = GPIO_MODE_INPUT,
        .pull_up_en   = GPIO_PULLUP_ENABLE,
        .pull_down_en = GPIO_PULLDOWN_DISABLE,
        .intr_type    = GPIO_INTR_DISABLE,
    };
    gpio_config(&io);
    int level = gpio_get_level((gpio_num_t)pin);
    printf("GPIO%d = %d\n", pin, level);
    return 0;
}

// ── gpio_set <pin> <0|1> ──────────────────────────────────────────────────────
static struct {
    struct arg_int *pin;
    struct arg_int *level;
    struct arg_end *end;
} gpio_set_args;

static int cmd_gpio_set(int argc, char **argv)
{
    if (arg_parse(argc, argv, (void **)&gpio_set_args)) {
        arg_print_errors(stdout, gpio_set_args.end, argv[0]); return 1;
    }
    int pin = gpio_set_args.pin->ival[0];
    int lvl = gpio_set_args.level->ival[0];
    if (pin < 0 || pin > 21)       { printf("pin 0-21 on ESP32-C3\n"); return 1; }
    if (lvl < 0 || lvl > 1)        { printf("level 0 or 1\n"); return 1; }
    gpio_config_t io = {
        .pin_bit_mask = (1ULL << pin),
        .mode         = GPIO_MODE_OUTPUT,
        .pull_up_en   = GPIO_PULLUP_DISABLE,
        .pull_down_en = GPIO_PULLDOWN_DISABLE,
        .intr_type    = GPIO_INTR_DISABLE,
    };
    gpio_config(&io);
    gpio_set_level((gpio_num_t)pin, lvl);
    printf("GPIO%d → %d\n", pin, lvl);
    return 0;
}

// ── reboot ────────────────────────────────────────────────────────────────────
static int cmd_reboot(int argc, char **argv)
{
    printf("Rebooting…\n");
    vTaskDelay(pdMS_TO_TICKS(100));
    esp_restart();
    return 0;
}

// ── factory_reset ─────────────────────────────────────────────────────────────
// Erases all NVS data (WiFi creds, Matter fabric, device config) and reboots.
// The device will need to be re-provisioned from the hub.
static int cmd_factory_reset(int argc, char **argv)
{
    printf("Erasing NVS (all config + Matter fabric) and rebooting…\n");
    vTaskDelay(pdMS_TO_TICKS(200));
    nvs_flash_erase();
    esp_restart();
    return 0;
}

// ═════════════════════════════════════════════════════════════════════════════
//  REGISTRATION
// ═════════════════════════════════════════════════════════════════════════════

static void register_all_commands(void)
{
    // -- argtable allocations --
    level_args.v   = arg_int1(NULL, NULL, "<level>",   "brightness 0-254");
    level_args.end = arg_end(2);

    ct_args.v   = arg_int1(NULL, NULL, "<mireds>", "colour temperature 153-500");
    ct_args.end = arg_end(2);

    fade_args.level    = arg_int1(NULL, NULL, "<level>",    "target brightness 0-254");
    fade_args.duration = arg_int0(NULL, NULL, "[duration]", "ramp time ms (default 1000)");
    fade_args.end      = arg_end(3);

    blink_args.count  = arg_int1(NULL, NULL, "<count>",  "blink count 1-100");
    blink_args.on_ms  = arg_int0(NULL, NULL, "[on_ms]",  "on duration ms (default 500)");
    blink_args.off_ms = arg_int0(NULL, NULL, "[off_ms]", "off duration ms (default 500)");
    blink_args.end    = arg_end(4);

    pulse_args.cycles = arg_int1(NULL, NULL, "<cycles>", "repeat count 1-20");
    pulse_args.period = arg_int0(NULL, NULL, "[period]", "period ms (default 2000)");
    pulse_args.end    = arg_end(3);

    sweep_args.cycles = arg_int1(NULL, NULL, "<cycles>", "repeat count 1-20");
    sweep_args.period = arg_int0(NULL, NULL, "[period]", "full-sweep period ms (default 4000)");
    sweep_args.end    = arg_end(3);

    raw_args.ain1 = arg_int1(NULL, NULL, "<ain1>", "AIN1 LEDC duty 0-1023");
    raw_args.ain2 = arg_int1(NULL, NULL, "<ain2>", "AIN2 LEDC duty 0-1023");
    raw_args.end  = arg_end(3);

    wifi_test_args.ssid     = arg_str1(NULL, NULL, "<ssid>",     "WiFi SSID");
    wifi_test_args.password = arg_str1(NULL, NULL, "<password>", "WiFi password");
    wifi_test_args.end      = arg_end(3);

    gpio_read_args.pin = arg_int1(NULL, NULL, "<pin>", "GPIO number 0-21");
    gpio_read_args.end = arg_end(2);

    gpio_set_args.pin   = arg_int1(NULL, NULL, "<pin>",   "GPIO number 0-21");
    gpio_set_args.level = arg_int1(NULL, NULL, "<0|1>",   "output level");
    gpio_set_args.end   = arg_end(3);

    // -- command table --
    const esp_console_cmd_t cmds[] = {
        // LED
        { "led_on",     "Turn LED strip on",                                    NULL,      cmd_led_on,     NULL          },
        { "led_off",    "Turn LED strip off",                                   NULL,      cmd_led_off,    NULL          },
        { "led_level",  "Set brightness: led_level <0-254>",                    "<level>", cmd_led_level,  &level_args   },
        { "led_ct",     "Set colour temp: led_ct <153-500 mireds>",             "<mireds>",cmd_led_ct,     &ct_args      },
        { "led_status", "Print current LED state",                              NULL,      cmd_led_status, NULL          },
        { "led_fade",   "Ramp brightness 0→level: led_fade <level> [ms]",       NULL,      cmd_led_fade,   &fade_args    },
        { "led_blink",  "Blink N times: led_blink <count> [on_ms] [off_ms]",   NULL,      cmd_led_blink,  &blink_args   },
        { "led_pulse",  "Breathe effect: led_pulse <cycles> [period_ms]",       NULL,      cmd_led_pulse,  &pulse_args   },
        { "led_sweep",  "CT warm↔cool sweep: led_sweep <cycles> [period_ms]",  NULL,      cmd_led_sweep,  &sweep_args   },
        { "led_raw",    "Raw LEDC duty: led_raw <ain1> <ain2>  (0-1023)",       NULL,      cmd_led_raw,    &raw_args     },
        // System
        { "sys_info",       "Chip model, flash, heap, uptime",                  NULL,      cmd_sys_info,   NULL          },
        { "wifi_status",    "SSID, RSSI, IP, MAC",                              NULL,      cmd_wifi_status,NULL          },
        { "wifi_scan",      "Scan for nearby APs",                              NULL,      cmd_wifi_scan,  NULL           },
        { "wifi_test",      "Test WiFi: wifi_test <ssid> <password>",           NULL,      cmd_wifi_test,  &wifi_test_args},
        { "nvs_dump",       "Dump all provisioned device config from NVS",      NULL,      cmd_nvs_dump,   NULL          },
        { "gpio_read",      "Read GPIO level (configures as input+pullup)",     "<pin>",   cmd_gpio_read,  &gpio_read_args},
        { "gpio_set",       "Set GPIO output level",                            NULL,      cmd_gpio_set,   &gpio_set_args },
        { "reboot",         "Soft-reset the chip",                              NULL,      cmd_reboot,     NULL          },
        { "factory_reset",  "Erase NVS + Matter fabric and reboot",             NULL,      cmd_factory_reset, NULL       },
    };

    for (size_t i = 0; i < sizeof(cmds) / sizeof(cmds[0]); i++) {
        ESP_ERROR_CHECK(esp_console_cmd_register(&cmds[i]));
    }
}

// ── REPL task — ROM UART poll, no ISR driver needed ──────────────────────────
// Uses esp_rom_uart_rx_one_char (non-blocking ROM read) so we never install a
// UART driver that could conflict with the Matter / WiFi interrupt stack.
static void repl_task(void *arg)
{
    char    line[256];
    int     pos = 0;
    uint8_t c;
    bool    last_cr = false;

    vTaskDelay(pdMS_TO_TICKS(200));
    printf("esp32>\r\n");
    fflush(stdout);

    while (true) {
        // Try USB Serial/JTAG first (built-in USB), fall back to UART0 (bridge)
        if (usb_serial_jtag_read_bytes(&c, 1, pdMS_TO_TICKS(5)) <= 0 &&
            uart_read_bytes(CONFIG_ESP_CONSOLE_UART_NUM, &c, 1, 0) <= 0)
            continue;

        if (c == '\r') {
            last_cr = true;
            line[pos] = '\0';
            if (pos > 0) {
                int ret = 0;
                esp_err_t err = esp_console_run(line, &ret);
                if (err == ESP_ERR_NOT_FOUND)
                    printf("Unknown: '%s'. Try 'help'.\r\n", line);
                else if (err != ESP_OK)
                    printf("err: %s\r\n", esp_err_to_name(err));
            }
            pos = 0;
            printf("esp32>\r\n");
            fflush(stdout);
        } else if (c == '\n') {
            if (!last_cr) {  // lone \n — treat same as \r
                line[pos] = '\0';
                if (pos > 0) {
                    int ret = 0;
                    esp_err_t err = esp_console_run(line, &ret);
                    if (err == ESP_ERR_NOT_FOUND)
                        printf("Unknown: '%s'. Try 'help'.\r\n", line);
                    else if (err != ESP_OK)
                        printf("err: %s\r\n", esp_err_to_name(err));
                }
                pos = 0;
                printf("esp32>\r\n");
                fflush(stdout);
            }
            last_cr = false;
        } else {
            last_cr = false;
            if ((c == 127 || c == 8) && pos > 0) {
                pos--;
            } else if (c >= 0x20 && c < 0x7f && pos < (int)sizeof(line) - 1) {
                line[pos++] = (char)c;
            }
        }
    }
}

// ── public entry point ────────────────────────────────────────────────────────
void console_start(void)
{
    usb_serial_jtag_driver_config_t usb_cfg = USB_SERIAL_JTAG_DRIVER_CONFIG_DEFAULT();
    usb_serial_jtag_driver_install(&usb_cfg);
    uart_driver_install(CONFIG_ESP_CONSOLE_UART_NUM, 1024, 0, 0, NULL, 0);

    esp_console_config_t cfg = ESP_CONSOLE_CONFIG_DEFAULT();
    cfg.max_cmdline_length = 256;
    ESP_ERROR_CHECK(esp_console_init(&cfg));
    ESP_ERROR_CHECK(esp_console_register_help_command());
    register_all_commands();

    xTaskCreate(repl_task, "repl", 8192, NULL, 5, NULL);
    ESP_LOGI(TAG, "console ready — type 'help'");
}
