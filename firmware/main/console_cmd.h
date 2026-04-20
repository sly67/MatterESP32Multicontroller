#pragma once

#ifdef __cplusplus
extern "C" {
#endif

// Start the esp_console REPL on UART0.
// Registers "led" commands for direct hardware testing.
// Must be called after drv8833_init().
void console_start(void);

#ifdef __cplusplus
}
#endif
