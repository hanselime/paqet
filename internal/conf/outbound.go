package conf

import (
	"fmt"
	"net"
	"strings"
)

type Outbound struct {
	Type_    string `yaml:"type"`
	Addr_    string `yaml:"addr"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`

	Type string `yaml:"-"`
	Addr string `yaml:"-"`
}

func (o *Outbound) setDefaults() {
	if o.Type_ == "" {
		o.Type = "direct"
	} else {
		o.Type = strings.ToLower(strings.TrimSpace(o.Type_))
	}
}

func (o *Outbound) validate() []error {
	var errors []error

	if o.Type == "" {
		o.Type = "direct"
	}
	if o.Type != "direct" && o.Type != "socks5" {
		errors = append(errors, fmt.Errorf("outbound type must be 'direct' or 'socks5', got %q", o.Type))
	}
	if o.Type == "socks5" {
		addr := strings.TrimSpace(o.Addr_)
		if addr == "" {
			errors = append(errors, fmt.Errorf("outbound addr is required when type is socks5"))
		} else {
			addr = strings.TrimPrefix(addr, "socks5://")
			_, err := net.ResolveTCPAddr("tcp", addr)
			if err != nil {
				errors = append(errors, fmt.Errorf("outbound addr invalid: %w", err))
			} else {
				o.Addr = addr
			}
		}
	}
	return errors
}
