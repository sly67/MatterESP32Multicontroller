package yamldef

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ParseTemplate parses and validates a Template from YAML bytes.
func ParseTemplate(data []byte) (*Template, error) {
	var t Template
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse template YAML: %w", err)
	}
	if err := validateTemplate(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

func validateTemplate(t *Template) error {
	if t.ID == "" {
		return fmt.Errorf("template id is required")
	}
	if t.Board == "" {
		return fmt.Errorf("template %q: board is required", t.ID)
	}
	if len(t.Modules) == 0 {
		return fmt.Errorf("template %q: must have at least one module", t.ID)
	}
	for i, m := range t.Modules {
		if m.Module == "" {
			return fmt.Errorf("template %q: module[%d] missing module id", t.ID, i)
		}
		if len(m.Pins) == 0 {
			return fmt.Errorf("template %q: module %q has no pins assigned", t.ID, m.Module)
		}
		for pinID, gpio := range m.Pins {
			if gpio == "" {
				return fmt.Errorf("template %q: module %q pin %q has no GPIO assigned", t.ID, m.Module, pinID)
			}
		}
		if m.EndpointName == "" {
			return fmt.Errorf("template %q: module %q missing endpoint_name", t.ID, m.Module)
		}
	}
	return nil
}
