package config

// App is the top-level application configuration.
type App struct {
	WebPort int `yaml:"web_port"`
	OTAPort int `yaml:"ota_port"`
}

// WiFi holds default WiFi credentials used when flashing devices.
type WiFi struct {
	SSID     string `yaml:"ssid"`
	Password string `yaml:"password"`
}

// USB holds declared USB port paths.
type USB struct {
	Ports []string `yaml:"ports"`
}

// PSKPolicy controls PSK generation behaviour.
type PSKPolicy struct {
	LengthBytes int `yaml:"length_bytes"`
}

// Config is the full loaded configuration.
type Config struct {
	App       App       `yaml:"-"`
	WiFi      WiFi      `yaml:"-"`
	USB       USB       `yaml:"-"`
	PSKPolicy PSKPolicy `yaml:"-"`
	WebPort   int
	OTAPort   int
}
