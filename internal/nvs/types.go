// Package nvs compiles hardware templates and device config into
// ESP-IDF NVS partition CSV format for esptool flashing.
package nvs

// DeviceConfig holds per-device values injected at flash time.
type DeviceConfig struct {
	Name           string // e.g. "1/Bedroom"
	WiFiSSID       string
	WiFiPassword   string
	PSK            []byte // 32 bytes, pre-generated
	BoardID        string // e.g. "esp32-c3"
	MatterDiscrim  uint16
	MatterPasscode uint32
}
