package conf

import "fmt"

// Obfuscation configuration for traffic obfuscation and randomization
type Obfuscation struct {
	// Obfuscation mode: none, padding, tls
	Mode string `yaml:"mode"`

	// Padding mode settings
	Padding struct {
		MinPad int `yaml:"min_pad"` // Minimum padding bytes (default: 16)
		MaxPad int `yaml:"max_pad"` // Maximum padding bytes (default: 128)
	} `yaml:"padding"`

	// Header randomization settings
	Headers struct {
		RandomizeTOS    bool `yaml:"randomize_tos"`     // Enable TOS randomization
		RandomizeTTL    bool `yaml:"randomize_ttl"`     // Enable TTL randomization
		RandomizeWindow bool `yaml:"randomize_window"`  // Enable window randomization
	} `yaml:"headers"`

	// Framing settings
	Framing struct {
		Mode    string `yaml:"mode"`     // Framing mode: fixed, random
		MinSize int    `yaml:"min_size"` // Minimum frame size (default: 64)
		MaxSize int    `yaml:"max_size"` // Maximum frame size (default: 1400)
		Jitter  int    `yaml:"jitter_ms"` // Timing jitter in milliseconds
	} `yaml:"framing"`
}

func (o *Obfuscation) setDefaults() {
	if o.Mode == "" {
		o.Mode = "none" // Default: no obfuscation for backward compatibility
	}

	// Padding defaults
	if o.Padding.MinPad == 0 {
		o.Padding.MinPad = 16
	}
	if o.Padding.MaxPad == 0 {
		o.Padding.MaxPad = 128
	}

	// Headers defaults - enable randomization by default when obfuscation is enabled
	if o.Mode != "none" {
		if !o.Headers.RandomizeTOS {
			o.Headers.RandomizeTOS = true
		}
		if !o.Headers.RandomizeTTL {
			o.Headers.RandomizeTTL = true
		}
		if !o.Headers.RandomizeWindow {
			o.Headers.RandomizeWindow = true
		}
	}

	// Framing defaults
	if o.Framing.Mode == "" {
		o.Framing.Mode = "fixed"
	}
	if o.Framing.MinSize == 0 {
		o.Framing.MinSize = 64
	}
	if o.Framing.MaxSize == 0 {
		o.Framing.MaxSize = 1400
	}
	if o.Framing.Jitter == 0 {
		o.Framing.Jitter = 0 // No jitter by default
	}
}

func (o *Obfuscation) validate() []error {
	var errors []error

	// Validate obfuscation mode
	validModes := []string{"none", "padding", "tls"}
	validMode := false
	for _, m := range validModes {
		if o.Mode == m {
			validMode = true
			break
		}
	}
	if !validMode {
		errors = append(errors, fmt.Errorf("obfuscation mode must be one of: %v", validModes))
	}

	// Validate padding settings
	if o.Padding.MinPad < 0 || o.Padding.MinPad > 255 {
		errors = append(errors, fmt.Errorf("padding min_pad must be between 0-255"))
	}
	if o.Padding.MaxPad < o.Padding.MinPad || o.Padding.MaxPad > 255 {
		errors = append(errors, fmt.Errorf("padding max_pad must be between min_pad-255"))
	}

	// Validate framing settings
	validFramingModes := []string{"fixed", "random"}
	validFramingMode := false
	for _, m := range validFramingModes {
		if o.Framing.Mode == m {
			validFramingMode = true
			break
		}
	}
	if !validFramingMode {
		errors = append(errors, fmt.Errorf("framing mode must be one of: %v", validFramingModes))
	}

	if o.Framing.MinSize < 1 || o.Framing.MinSize > 65535 {
		errors = append(errors, fmt.Errorf("framing min_size must be between 1-65535"))
	}
	if o.Framing.MaxSize < o.Framing.MinSize || o.Framing.MaxSize > 65535 {
		errors = append(errors, fmt.Errorf("framing max_size must be between min_size-65535"))
	}
	if o.Framing.Jitter < 0 || o.Framing.Jitter > 1000 {
		errors = append(errors, fmt.Errorf("framing jitter_ms must be between 0-1000"))
	}

	return errors
}
