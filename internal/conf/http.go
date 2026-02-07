package conf

import (
	"net"
)

type HTTP struct {
	Listen_  string       `yaml:"listen"`
	Username string       `yaml:"username"`
	Password string       `yaml:"password"`
	Listen   *net.UDPAddr `yaml:"-"`
}

func (c *HTTP) setDefaults() {}
func (c *HTTP) validate() []error {
	var errors []error

	addr, err := validateAddr(c.Listen_, true)
	if err != nil {
		errors = append(errors, err)
	}
	c.Listen = addr
	return errors
}
