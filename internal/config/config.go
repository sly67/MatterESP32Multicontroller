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
	// Try to create the file exclusively — atomic first-boot write
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err == nil {
		// File did not exist — write defaults
		defer f.Close()
		data, merr := yaml.Marshal(def)
		if merr != nil {
			return def, fmt.Errorf("marshal default %s: %w", path, merr)
		}
		if _, werr := f.Write(data); werr != nil {
			return def, fmt.Errorf("write default %s: %w", path, werr)
		}
		return def, nil
	}
	if !os.IsExist(err) {
		return def, fmt.Errorf("open %s: %w", path, err)
	}
	// File already exists — read it
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
