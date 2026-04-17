package yamldef

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

var validCategories = map[string]bool{
	"driver": true, "sensor": true, "io": true,
}

var validIOTypes = map[string]bool{
	IOTypePWMOut:        true,
	IOTypeDigitalPWMOut: true,
	IOTypeDigitalOut:    true,
	IOTypeDigitalIn:     true,
	IOTypeADCIn:         true,
	IOTypeI2CData:       true,
	IOTypeI2CClock:      true,
}

// ParseModule parses and validates a Module from YAML bytes.
func ParseModule(data []byte) (*Module, error) {
	var m Module
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse module YAML: %w", err)
	}
	if err := validateModule(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func validateModule(m *Module) error {
	if m.ID == "" {
		return fmt.Errorf("module id is required")
	}
	if m.Name == "" {
		return fmt.Errorf("module %q: name is required", m.ID)
	}
	if !validCategories[m.Category] {
		return fmt.Errorf("module %q: category %q must be one of: driver, sensor, io", m.ID, m.Category)
	}
	if m.Matter.EndpointType == "" {
		return fmt.Errorf("module %q: matter.endpoint_type is required", m.ID)
	}
	pinIDs := map[string]bool{}
	for _, pin := range m.IO {
		if pin.ID == "" {
			return fmt.Errorf("module %q: io pin missing id", m.ID)
		}
		if !validIOTypes[pin.Type] {
			return fmt.Errorf("module %q: io type %q is not valid for pin %q", m.ID, pin.Type, pin.ID)
		}
		if pinIDs[pin.ID] {
			return fmt.Errorf("module %q: duplicate io pin id %q", m.ID, pin.ID)
		}
		pinIDs[pin.ID] = true
	}
	for _, pg := range m.PinGroups {
		for _, p := range pg.Pins {
			if !pinIDs[p] {
				return fmt.Errorf("module %q: pin_group %q references unknown pin %q", m.ID, pg.ID, p)
			}
		}
	}
	for _, ch := range m.Channels {
		for _, p := range ch.In {
			if !pinIDs[p] {
				return fmt.Errorf("module %q: channel %q references unknown pin %q", m.ID, ch.ID, p)
			}
		}
	}
	if m.Measurement != nil {
		for _, op := range m.Measurement.Routine {
			if op.Op == "wait_edge" || op.Op == "measure_pulse" {
				if op.TimeoutUs == 0 {
					return fmt.Errorf("module %q: routine op %q requires timeout_us", m.ID, op.Op)
				}
			}
		}
	}
	return nil
}
