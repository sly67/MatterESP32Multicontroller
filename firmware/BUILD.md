# Firmware Build Guide

## Requirements

- [ESP-IDF v5.1.x](https://docs.espressif.com/projects/esp-idf/en/v5.1/esp32c3/get-started/)
- [esp-matter v1.2+](https://github.com/espressif/esp-matter)

```bash
# Install ESP-IDF
git clone --recursive https://github.com/espressif/esp-idf.git ~/esp/esp-idf
cd ~/esp/esp-idf && git checkout v5.1.4
./install.sh esp32c3
. ./export.sh

# Install esp-matter
git clone --recursive https://github.com/espressif/esp-matter.git ~/esp/esp-matter
cd ~/esp/esp-matter && ./install.sh
. ./export.sh
```

## Docker build (recommended)

Two-stage Docker build: slow base image built once, fast firmware image rebuilt per change.

```bash
cd firmware/

# Step 1 — build the toolchain base (once, ~15 min; redo only when Dockerfile.base changes)
docker build -f Dockerfile.base -t matter-fw-base:latest .

# Step 2 — build firmware (every time, ~3–5 min)
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
docker build -f Dockerfile.build -t matter-fw-builder:latest .
BIN="matter_hub_firmware_v${TIMESTAMP}.bin"
docker run --rm -v "$(pwd)":/output matter-fw-builder:latest \
    bash -c "cp build/matter_hub_firmware.bin /output/${BIN}"

# Deploy to hub incoming directory
sudo cp "${BIN}" /Portainer/MatterESP32/firmware/incoming/
sudo chown 1000:1000 /Portainer/MatterESP32/firmware/incoming/${BIN}
```

The output binary is `build/matter_hub_firmware.bin`.

## Flash manually (development)

```bash
idf.py -p /dev/ttyUSB0 flash monitor
```

## Flash via hub (production)

Upload `build/matter_hub_firmware.bin` in the web UI → **Firmware** tab,
then use the **Flash** wizard to write firmware + NVS to a device.

The hub writes:
- `0x0000` — full firmware binary (bootloader + partition table + app)
- `0x9000` — NVS partition with device-specific config (WiFi, Matter credentials, module pins)

## Matter commissioning

After flashing, the device:
1. Connects to WiFi using credentials from NVS
2. Starts Matter commissioning mode with the discriminator/passcode stored in NVS
3. Shows up as a **Color Temperature Light** in your Matter controller

Commission via Apple Home, Google Home, or:
```bash
chip-tool pairing ble-wifi <node-id> <ssid> <password> <passcode> <discriminator>
```

## DRV8833 LED mapping

| Matter control | H-bridge output | LEDs |
|----------------|----------------|------|
| On + warm CT (≥326 mireds) | AIN1 PWM, AIN2=0 | GPIO0 side |
| On + cool CT (<326 mireds) | AIN1=0, AIN2 PWM | GPIO1 side |
| Off | AIN1=0, AIN2=0 | all off |
| Level | PWM duty 0–100% | brightness |
