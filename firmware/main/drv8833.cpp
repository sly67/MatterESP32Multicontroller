#include "drv8833.h"
#include "driver/ledc.h"
#include "esp_log.h"
#include <math.h>

static const char *TAG = "drv8833";

#define LEDC_MODE       LEDC_LOW_SPEED_MODE
#define LEDC_TIMER      LEDC_TIMER_0
#define LEDC_FREQ_HZ    20000
#define LEDC_RES        LEDC_TIMER_11_BIT   // 0–2047 (11-bit max at 20 kHz on ESP32-C3)
#define LEDC_MAX_DUTY   2047

#define CH_AIN1  LEDC_CHANNEL_0
#define CH_AIN2  LEDC_CHANNEL_1

// Colour-temperature range (mireds): 153 = coolest, 500 = warmest
#define CT_MIN  153
#define CT_MAX  500

static drv8833_cfg_t s_cfg = {-1, -1};

static esp_err_t init_channel(ledc_channel_t ch, int gpio)
{
    if (gpio < 0) return ESP_OK;
    ledc_channel_config_t conf = {
        .gpio_num   = gpio,
        .speed_mode = LEDC_MODE,
        .channel    = ch,
        .intr_type  = LEDC_INTR_DISABLE,
        .timer_sel  = LEDC_TIMER,
        .duty       = 0,
        .hpoint     = 0,
        .flags      = { .output_invert = 0 },
    };
    return ledc_channel_config(&conf);
}

// Gamma 2.2 curve: Matter level 0–254 → LEDC duty 0–2047
// Equal perceived brightness steps across the 2048-point scale.
static uint32_t level_to_duty(uint8_t level)
{
    if (level == 0)   return 0;
    if (level >= 254) return LEDC_MAX_DUTY;
    float norm = (float)level / 254.0f;
    return (uint32_t)(powf(norm, 2.2f) * (float)LEDC_MAX_DUTY + 0.5f);
}

// Set duty + phase-start (hpoint) then latch atomically on next period
static void set_ch(ledc_channel_t ch, uint32_t duty, uint32_t hpoint)
{
    ledc_set_duty_with_hpoint(LEDC_MODE, ch, duty, hpoint);
    ledc_update_duty(LEDC_MODE, ch);
}

// Raw duty for both channels (manual / test use — no phase coordination)
void drv8833_set_duty(uint32_t ain1, uint32_t ain2)
{
    if (ain1 > LEDC_MAX_DUTY) ain1 = LEDC_MAX_DUTY;
    if (ain2 > LEDC_MAX_DUTY) ain2 = LEDC_MAX_DUTY;
    set_ch(CH_AIN1, ain1, 0);
    set_ch(CH_AIN2, ain2, 0);
}

esp_err_t drv8833_init(const drv8833_cfg_t *cfg)
{
    s_cfg = *cfg;

    ledc_timer_config_t timer = {
        .speed_mode      = LEDC_MODE,
        .duty_resolution = LEDC_RES,
        .timer_num       = LEDC_TIMER,
        .freq_hz         = LEDC_FREQ_HZ,
        .clk_cfg         = LEDC_AUTO_CLK,
    };
    esp_err_t err = ledc_timer_config(&timer);
    if (err != ESP_OK) return err;

    err = init_channel(CH_AIN1, cfg->gpio_ain1);
    if (err != ESP_OK) return err;
    err = init_channel(CH_AIN2, cfg->gpio_ain2);
    if (err != ESP_OK) return err;

    ESP_LOGI(TAG, "init AIN1=GPIO%d AIN2=GPIO%d", cfg->gpio_ain1, cfg->gpio_ain2);
    return ESP_OK;
}

void drv8833_set(bool on_off, uint8_t level, uint16_t color_temp_mireds)
{
    if (!on_off) {
        set_ch(CH_AIN1, 0, 0);
        set_ch(CH_AIN2, 0, 0);
        return;
    }

    uint32_t budget = level_to_duty(level);

    // Map colour temp → warm fraction (0 = pure cool, 1023 = pure warm)
    uint32_t ct = color_temp_mireds < CT_MIN ? CT_MIN
                : color_temp_mireds > CT_MAX ? CT_MAX
                : color_temp_mireds;
    uint32_t duty_warm = budget * (ct - CT_MIN) / (CT_MAX - CT_MIN);
    uint32_t duty_cool = budget - duty_warm;

    // Phase-sequential: AIN1 (warm) fires first, AIN2 (cool) starts exactly
    // where AIN1 stops — guaranteed no overlap, no brake state.
    //   Counter: 0 → duty_warm  : AIN1 HIGH, AIN2 LOW  (warm LEDs)
    //            duty_warm → budget: AIN1 LOW,  AIN2 HIGH (cool LEDs)
    //            budget → 2047   : both LOW               (off / dimming)
    set_ch(CH_AIN1, duty_warm, 0);
    set_ch(CH_AIN2, duty_cool, duty_warm);
}
