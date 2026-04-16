package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

var defaults = App{WebPort: 48060, OTAPort: 48061}

// Load reads config files from dir, writing defaults if missing.
func Load(dir string) (*Config, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir config dir: %w", err)
	}

	app, err := loadOrWrite[App](filepath.Join(dir, "app.yaml"), defaults)
	if err != nil {
		return nil, err
	}
	wifi, err := loadOrWrite[WiFi](filepath.Join(dir, "wifi.yaml"), WiFi{})
	if err != nil {
		return nil, err
	}
	usb, err := loadOrWrite[USB](filepath.Join(dir, "usb.yaml"), USB{Ports: []string{"/dev/ttyUSB0"}})
	if err != nil {
		return nil, err
	}
	psk, err := loadOrWrite[PSKPolicy](filepath.Join(dir, "psk-policy.yaml"), PSKPolicy{LengthBytes: 32})
	if err != nil {
		return nil, err
	}

	return &Config{
		App:       app,
		WiFi:      wifi,
		USB:       usb,
		PSKPolicy: psk,
		WebPort:   app.WebPort,
		OTAPort:   app.OTAPort,
	}, nil
}

func loadOrWrite[T any](path string, def T) (T, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		data, err := yaml.Marshal(def)
		if err != nil {
			return def, fmt.Errorf("marshal default %s: %w", path, err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return def, fmt.Errorf("write default %s: %w", path, err)
		}
		return def, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return def, fmt.Errorf("read %s: %w", path, err)
	}
	var v T
	if err := yaml.Unmarshal(data, &v); err != nil {
		return def, fmt.Errorf("parse %s: %w", path, err)
	}
	return v, nil
}
