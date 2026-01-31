package tnet

import (
	"fmt"
	"net"
	"strconv"
)

type Addr struct {
	Host string
	Port int
}

func NewAddr(s string) (*Addr, error) {
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		return nil, err
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q: %w", portStr, err)
	}

	return &Addr{Host: host, Port: port}, nil
}

func (e *Addr) String() string {
	return net.JoinHostPort(e.Host, strconv.Itoa(e.Port))
}
