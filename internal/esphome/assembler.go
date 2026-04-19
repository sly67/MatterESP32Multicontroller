package esphome

import (
	"fmt"
	"strings"

	"github.com/karthangar/matteresp32hub/internal/yamldef"
)

// ComponentConfig is one component in an ESPHome device.
type ComponentConfig struct {
	Type string            `json:"type"` // module ID, e.g. "dht22"
	Name string            `json:"name"` // user-facing label, e.g. "Room Temp"
	Pins map[string]string `json:"pins"` // pin role → GPIO, e.g. {"DATA": "GPIO4"}
}

// Config is the full set of parameters for assembling an ESPHome YAML.
type Config struct {
	Board         string            // e.g. "esp32-c3"
	DeviceName    string            // e.g. "Kitchen Sensor" → slug: "kitchen-sensor"
	DeviceID      string            // chip MAC, used in heartbeat URL
	WiFiSSID      string
	WiFiPassword  string
	HAIntegration bool
	APIKey        string // base64-encoded 32-byte key; required if HAIntegration == true
	OTAPassword   string // hex random bytes
	HubURL        string // e.g. "http://192.168.1.10:8080"
	Components    []ComponentConfig
}

// boardDef maps board IDs to [platform, board-name] pairs.
var boardDef = map[string][2]string{
	"esp32-c3": {"esp32", "esp32-c3-devkitm-1"},
	"esp32-h2": {"esp32", "esp32-h2-devkitm-1"},
	"esp32":    {"esp32", "esp32dev"},
	"esp32-s3": {"esp32", "esp32-s3-devkitc-1"},
}

func slug(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "-"))
}

// idSlug converts a component name into a valid ESPHome/C++ identifier.
func idSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// Assemble builds a complete ESPHome YAML string from cfg and the module library.
func Assemble(cfg Config, modules map[string]*yamldef.Module) (string, error) {
	bd, ok := boardDef[cfg.Board]
	if !ok {
		return "", fmt.Errorf("unsupported board: %q", cfg.Board)
	}
	platform, boardName := bd[0], bd[1]
	deviceSlug := slug(cfg.DeviceName)

	var sb strings.Builder

	fmt.Fprintf(&sb, "esphome:\n  name: %s\n\n", deviceSlug)
	fmt.Fprintf(&sb, "%s:\n  board: %s\n  framework:\n    type: esp-idf\n\n", platform, boardName)
	fmt.Fprintf(&sb, "wifi:\n  ssid: %q\n  password: %q\n  ap:\n    ssid: %q\n    password: \"changeme\"\n\n",
		cfg.WiFiSSID, cfg.WiFiPassword, deviceSlug+"-fallback")
	sb.WriteString("logger:\n\n")
	fmt.Fprintf(&sb, "ota:\n  - platform: esphome\n    password: %q\n\n", cfg.OTAPassword)

	if cfg.HAIntegration && cfg.APIKey != "" {
		fmt.Fprintf(&sb, "api:\n  encryption:\n    key: %q\n\n", cfg.APIKey)
	}

	fmt.Fprintf(&sb, "http_request:\n  useragent: MatterHub-ESPHome/1.0\n\n")
	fmt.Fprintf(&sb, "interval:\n  - interval: 60s\n    then:\n      - http_request.post:\n          url: %q\n\n",
		cfg.HubURL+"/api/devices/"+cfg.DeviceID+"/heartbeat")

	// Collect rendered component entries grouped by domain
	type entry struct{ domain, rendered string }
	var entries []entry
	domainSeen := map[string]bool{}
	var domainOrder []string

	for _, comp := range cfg.Components {
		mod, ok := modules[comp.Type]
		if !ok {
			return "", fmt.Errorf("module %q not found in library", comp.Type)
		}
		if mod.ESPHome == nil {
			return "", fmt.Errorf("module %q has no esphome: block", comp.Type)
		}
		for _, ec := range mod.ESPHome.Components {
			rendered := ec.Template
			rendered = strings.ReplaceAll(rendered, "{NAME}", comp.Name)
			rendered = strings.ReplaceAll(rendered, "{ID}", idSlug(comp.Name))
			for role, gpio := range comp.Pins {
				rendered = strings.ReplaceAll(rendered, "{"+role+"}", gpio)
			}
			entries = append(entries, entry{ec.Domain, rendered})
			if !domainSeen[ec.Domain] {
				domainSeen[ec.Domain] = true
				domainOrder = append(domainOrder, ec.Domain)
			}
		}
	}

	byDomain := map[string][]string{}
	for _, e := range entries {
		byDomain[e.domain] = append(byDomain[e.domain], e.rendered)
	}

	for _, domain := range domainOrder {
		sb.WriteString(domain + ":\n")
		for _, tmpl := range byDomain[domain] {
			lines := strings.Split(strings.TrimRight(tmpl, "\n"), "\n")
			fmt.Fprintf(&sb, "  - %s\n", lines[0])
			for _, line := range lines[1:] {
				if line == "" {
					sb.WriteByte('\n')
				} else {
					fmt.Fprintf(&sb, "    %s\n", line)
				}
			}
		}
		sb.WriteByte('\n')
	}

	return sb.String(), nil
}
