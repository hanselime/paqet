package conf

import "net"

type Metrics struct {
	Enabled bool         `yaml:"enabled"`
	Listen_ string       `yaml:"listen"`
	Listen  *net.TCPAddr `yaml:"-"`
}

func (m *Metrics) setDefaults() {
	if m.Listen_ == "" {
		m.Listen_ = "127.0.0.1:9090"
	}
}

func (m *Metrics) validate() []error {
	if !m.Enabled {
		return nil
	}
	var errors []error
	addr, err := validateTCPAddr(m.Listen_, true)
	if err != nil {
		errors = append(errors, err)
	}
	m.Listen = addr
	return errors
}
