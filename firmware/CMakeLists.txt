cmake_minimum_required(VERSION 3.16)

# Required by esp_matter component — must be set before IDF project.cmake include
set(MATTER_SDK_PATH $ENV{ESP_MATTER_PATH}/connectedhomeip/connectedhomeip)

# esp-matter components + connectedhomeip chip component
set(EXTRA_COMPONENT_DIRS
    $ENV{ESP_MATTER_PATH}/components
    $ENV{ESP_MATTER_PATH}/connectedhomeip/connectedhomeip/config/esp32/components
)

set(PROJECT_VER "1.1.6")
include($ENV{IDF_PATH}/tools/cmake/project.cmake)
project(matter_hub_firmware)
