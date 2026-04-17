package yamldef

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

var validParamTypes = map[string]bool{
	ParamTypeFloat:     true,
	ParamTypeInt:       true,
	ParamTypeBool:      true,
	ParamTypePercent:   true,
	ParamTypeDuration:  true,
	ParamTypeSpeed:     true,
	ParamTypeColorRGB:  true,
	ParamTypeColorWRGB: true,
	ParamTypeEasing:    true,
	ParamTypeSelect:    true,
}

// ParseEffect parses and validates an Effect from YAML bytes.
func ParseEffect(data []byte) (*Effect, error) {
	var e Effect
	if err := yaml.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("parse effect YAML: %w", err)
	}
	if err := validateEffect(&e); err != nil {
		return nil, err
	}
	return &e, nil
}

func validateEffect(e *Effect) error {
	if e.ID == "" {
		return fmt.Errorf("effect id is required")
	}
	if e.Name == "" {
		return fmt.Errorf("effect %q: name is required", e.ID)
	}
	if len(e.CompatibleWith) == 0 {
		return fmt.Errorf("effect %q: compatible_with must list at least one module id", e.ID)
	}
	paramIDs := map[string]bool{}
	for _, p := range e.Params {
		if p.ID == "" {
			return fmt.Errorf("effect %q: param missing id", e.ID)
		}
		if !validParamTypes[p.Type] {
			return fmt.Errorf("effect %q: param type %q is not valid for param %q", e.ID, p.Type, p.ID)
		}
		if paramIDs[p.ID] {
			return fmt.Errorf("effect %q: duplicate param id %q", e.ID, p.ID)
		}
		paramIDs[p.ID] = true
	}
	return nil
}
