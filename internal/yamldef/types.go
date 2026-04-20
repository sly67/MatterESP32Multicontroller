// Package yamldef provides Go types for hardware module, effect,
// template, and board profile YAML definitions.
package yamldef

// IOType values for IOPin.Type.
const (
	IOTypePWMOut        = "pwm_out"
	IOTypeDigitalPWMOut = "digital_pwm_out"
	IOTypeDigitalOut    = "digital_out"
	IOTypeDigitalIn     = "digital_in"
	IOTypeADCIn         = "adc_in"
	IOTypeI2CData       = "i2c_data"
	IOTypeI2CClock      = "i2c_clock"
	IOTypeConfig        = "config" // non-GPIO configuration value (e.g. LEDC channel, timer index)
)

// ParamType values for EffectParam.Type.
const (
	ParamTypeFloat     = "float"
	ParamTypeInt       = "int"
	ParamTypeBool      = "bool"
	ParamTypePercent   = "percent"
	ParamTypeDuration  = "duration"
	ParamTypeSpeed     = "speed"
	ParamTypeColorRGB  = "color_rgb"
	ParamTypeColorWRGB = "color_wrgb"
	ParamTypeEasing    = "easing"
	ParamTypeSelect    = "select"
)

// Module is a hardware module definition (e.g. DRV8833, BH1750).
type Module struct {
	ID           string                            `yaml:"id"`
	Name         string                            `yaml:"name"`
	Version      string                            `yaml:"version"`
	Category     string                            `yaml:"category"` // driver | sensor | io
	IO           []IOPin                           `yaml:"io"`
	Channels     []Channel                         `yaml:"channels,omitempty"`
	PinGroups    []PinGroup                        `yaml:"pin_groups,omitempty"`
	TruthTable   map[string]map[string]interface{} `yaml:"truth_table,omitempty"`
	PWMModes     map[string]map[string]interface{} `yaml:"pwm_modes,omitempty"`
	Capabilities []Capability                      `yaml:"capabilities,omitempty"`
	Matter       MatterDef                         `yaml:"matter"`
	ESPHome      *ESPHomeDef                       `yaml:"esphome,omitempty"`
	Measurement  *Measurement                      `yaml:"measurement,omitempty"`
}

// IOPin is a single input/output pin declaration within a module.
type IOPin struct {
	ID          string      `yaml:"id"          json:"id"`
	Type        string      `yaml:"type"         json:"type"`
	Label       string      `yaml:"label"        json:"label"`
	Default     string      `yaml:"default,omitempty" json:"default,omitempty"`
	Constraints Constraints `yaml:"constraints,omitempty" json:"constraints,omitempty"`
}

// Constraints holds type-specific pin constraints.
type Constraints struct {
	PWM     *PWMConstraints     `yaml:"pwm,omitempty"     json:"pwm,omitempty"`
	ADC     *ADCConstraints     `yaml:"adc,omitempty"     json:"adc,omitempty"`
	I2C     *I2CConstraints     `yaml:"i2c,omitempty"     json:"i2c,omitempty"`
	Digital *DigitalConstraints `yaml:"digital,omitempty" json:"digital,omitempty"`
}

// PWMConstraints describes PWM signal parameters.
type PWMConstraints struct {
	FrequencyHz    int     `yaml:"frequency_hz,omitempty"`
	FrequencyRange []int   `yaml:"frequency_range,omitempty"`
	DutyMin        float64 `yaml:"duty_min"`
	DutyMax        float64 `yaml:"duty_max"`
	ResolutionBits int     `yaml:"resolution_bits"`
	Invert         bool    `yaml:"invert,omitempty"`
	DeadTimeUs     int     `yaml:"dead_time_us,omitempty"`
}

// ADCConstraints describes ADC sampling parameters.
type ADCConstraints struct {
	Attenuation    string `yaml:"attenuation"` // 0db | 2.5db | 6db | 11db
	ResolutionBits int    `yaml:"resolution_bits"`
	SampleRateHz   int    `yaml:"sample_rate_hz"`
	Filter         string `yaml:"filter"` // none | moving_average | median
	FilterSamples  int    `yaml:"filter_samples"`
}

// I2CConstraints describes I2C bus parameters.
type I2CConstraints struct {
	Speed  int    `yaml:"speed"`
	Pullup string `yaml:"pullup"` // internal | external | none
}

// DigitalConstraints describes digital GPIO parameters.
type DigitalConstraints struct {
	Active       string `yaml:"active"`        // high | low
	InitialState string `yaml:"initial_state"` // high | low
}

// Channel is a logical grouping of IO pins (e.g. H-bridge A).
type Channel struct {
	ID    string   `yaml:"id"`
	In    []string `yaml:"in"`
	Label string   `yaml:"label"`
}

// PinGroup declares a relationship between pins (e.g. complementary).
type PinGroup struct {
	ID         string   `yaml:"id"`
	Pins       []string `yaml:"pins"`
	Mode       string   `yaml:"mode"` // complementary
	DeadTimeUs int      `yaml:"dead_time_us,omitempty"`
}

// Capability is a driver-specific option set exposed to effects.
type Capability struct {
	ID      string             `yaml:"id"`
	Type    string             `yaml:"type"`
	Label   string             `yaml:"label"`
	Options []CapabilityOption `yaml:"options"`
}

// CapabilityOption is one choice within a Capability.
type CapabilityOption struct {
	Value string `yaml:"value"`
	Label string `yaml:"label"`
}

// MatterDef maps a module to a Matter endpoint type and behaviors.
type MatterDef struct {
	EndpointType string   `yaml:"endpoint_type"`
	Behaviors    []string `yaml:"behaviors"`
}

// ESPHomeComponent is one ESPHome component block inside a module's esphome: section.
// Domain maps to the top-level ESPHome YAML key (e.g. "sensor", "switch", "light").
// Template is raw YAML with {PIN_ROLE} and {NAME} placeholders substituted at assembly time.
type ESPHomeComponent struct {
	Domain   string `yaml:"domain"`
	Template string `yaml:"template"`
}

// ESPHomeDef is the esphome: block in a module YAML.
type ESPHomeDef struct {
	Includes   []string           `yaml:"includes,omitempty"`
	Components []ESPHomeComponent `yaml:"components"`
}

// Measurement defines a custom sensor measurement routine.
type Measurement struct {
	TriggerIntervalMs int         `yaml:"trigger_interval_ms"`
	MaxDurationMs     int         `yaml:"max_duration_ms"`
	OnTimeout         string      `yaml:"on_timeout"` // last_value | zero | error
	Routine           []RoutineOp `yaml:"routine"`
}

// RoutineOp is a single step in a measurement routine.
type RoutineOp struct {
	Op         string      `yaml:"op"`
	Pin        string      `yaml:"pin,omitempty"`
	Value      interface{} `yaml:"value,omitempty"`
	DurationUs int         `yaml:"duration_us,omitempty"`
	Edge       string      `yaml:"edge,omitempty"`
	TimeoutUs  int         `yaml:"timeout_us,omitempty"`
	Store      string      `yaml:"store,omitempty"`
	Expr       string      `yaml:"expr,omitempty"`
	Address    int         `yaml:"address,omitempty"`
	Bytes      int         `yaml:"bytes,omitempty"`
	Count      int         `yaml:"count,omitempty"`
	Unit       string      `yaml:"unit,omitempty"`
	Min        float64     `yaml:"min,omitempty"`
	Max        float64     `yaml:"max,omitempty"`
}

// Effect is a reusable light/motor behavior definition.
type Effect struct {
	ID             string        `yaml:"id"`
	Name           string        `yaml:"name"`
	Version        string        `yaml:"version"`
	CompatibleWith []string      `yaml:"compatible_with"`
	Params         []EffectParam `yaml:"params"`
}

// EffectParam is a single configurable parameter within an effect.
type EffectParam struct {
	ID          string      `yaml:"id"`
	Type        string      `yaml:"type"`
	Label       string      `yaml:"label"`
	Default     interface{} `yaml:"default,omitempty"`
	Unit        string      `yaml:"unit,omitempty"`
	Min         interface{} `yaml:"min,omitempty"`
	Max         interface{} `yaml:"max,omitempty"`
	OptionsFrom string      `yaml:"options_from,omitempty"` // e.g. "capability.channel_mode"
}

// Template is a device hardware configuration template.
type Template struct {
	ID      string           `yaml:"id"`
	Board   string           `yaml:"board"`
	Modules []TemplateModule `yaml:"modules"`
}

// TemplateModule is one module instance within a template.
type TemplateModule struct {
	Module       string                 `yaml:"module"`
	Pins         map[string]string      `yaml:"pins"`
	EndpointName string                 `yaml:"endpoint_name"`
	Effect       string                 `yaml:"effect,omitempty"`
	EffectParams map[string]interface{} `yaml:"effect_params,omitempty"`
	Params       map[string]interface{} `yaml:"params,omitempty"`
}

// BoardProfile describes the GPIO/peripheral capabilities of a board.
type BoardProfile struct {
	ID          string    `yaml:"id"`
	Name        string    `yaml:"name"`
	Chip        string    `yaml:"chip"`
	GPIOPins    []GPIOPin `yaml:"gpio_pins"`
	I2CBuses    []I2CBus  `yaml:"i2c_buses,omitempty"`
	ADCChannels int       `yaml:"adc_channels"`
	PWMChannels int       `yaml:"pwm_channels"`
}

// GPIOPin describes the capabilities of a single GPIO pin on a board.
type GPIOPin struct {
	ID   string `yaml:"id"`
	ADC  bool   `yaml:"adc,omitempty"`
	PWM  bool   `yaml:"pwm,omitempty"`
	I2C  bool   `yaml:"i2c,omitempty"`
	Note string `yaml:"note,omitempty"`
}

// I2CBus describes an I2C hardware bus on a board.
type I2CBus struct {
	ID         string `yaml:"id"`
	SDADefault string `yaml:"sda_default"`
	SCLDefault string `yaml:"scl_default"`
}
