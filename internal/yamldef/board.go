package yamldef

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ParseBoard parses and validates a BoardProfile from YAML bytes.
func ParseBoard(data []byte) (*BoardProfile, error) {
	var b BoardProfile
	if err := yaml.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse board YAML: %w", err)
	}
	if err := validateBoard(&b); err != nil {
		return nil, err
	}
	return &b, nil
}

func validateBoard(b *BoardProfile) error {
	if b.ID == "" {
		return fmt.Errorf("board id is required")
	}
	if b.Name == "" {
		return fmt.Errorf("board %q: name is required", b.ID)
	}
	if b.Chip == "" {
		return fmt.Errorf("board %q: chip is required", b.ID)
	}
	if len(b.GPIOPins) == 0 {
		return fmt.Errorf("board %q: must define at least one gpio_pin", b.ID)
	}
	pinIDs := map[string]bool{}
	for _, p := range b.GPIOPins {
		if p.ID == "" {
			return fmt.Errorf("board %q: gpio pin missing id", b.ID)
		}
		if pinIDs[p.ID] {
			return fmt.Errorf("board %q: duplicate gpio pin id %q", b.ID, p.ID)
		}
		pinIDs[p.ID] = true
	}
	return nil
}
