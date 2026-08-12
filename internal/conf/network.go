package conf

import (
	"fmt"
	"net"
	"runtime"
)

type Addr struct {
	Addr_      string           `yaml:"addr"`
	RouterMac_ string           `yaml:"router_mac"`
	Addr       *net.UDPAddr     `yaml:"-"`
	Router     net.HardwareAddr `yaml:"-"`
}

type Network struct {
	Interface_ string         `yaml:"interface"`
	GUID       string         `yaml:"guid"`
	IPv4       Addr           `yaml:"ipv4"`
	IPv6       Addr           `yaml:"ipv6"`
	PCAP       PCAP           `yaml:"pcap"`
	TCP        TCP            `yaml:"tcp"`
	Interface  *net.Interface `yaml:"-"`
	Port       int            `yaml:"-"`
}

func (n *Network) setDefaults(role string) {
	n.PCAP.setDefaults(role)
	n.TCP.setDefaults()
}

func (n *Network) validate() []error {
	var errors []error

	if n.Interface_ == "" {
		errors = append(errors, fmt.Errorf("network interface is required"))
	}
	if len(n.Interface_) > 15 {
		errors = append(errors, fmt.Errorf("network interface name too long (max 15 characters): '%s'", n.Interface_))
	}
	lIface, err := net.InterfaceByName(n.Interface_)
	if err != nil {
		errors = append(errors, fmt.Errorf("failed to find network interface %s: %v", n.Interface_, err))
	}
	n.Interface = lIface

	if runtime.GOOS == "windows" && n.GUID == "" {
		errors = append(errors, fmt.Errorf("guid is required on windows"))
	}

	if n.IPv4.Addr_ == "" && n.IPv6.Addr_ == "" {
		errors = append(errors, fmt.Errorf("at least one address family (IPv4 or IPv6) must be configured"))
		return errors
	}
	if n.IPv4.Addr_ != "" {
		errors = append(errors, n.IPv4.validate()...)
	}
	if n.IPv6.Addr_ != "" {
		errors = append(errors, n.IPv6.validate()...)
	}

	ipv4OK := n.IPv4.Addr != nil
	ipv6OK := n.IPv6.Addr != nil

	if ipv4OK && ipv6OK && n.IPv4.Addr.Port != n.IPv6.Addr.Port {
		errors = append(errors, fmt.Errorf("IPv4 port (%d) and IPv6 port (%d) must match when both are configured", n.IPv4.Addr.Port, n.IPv6.Addr.Port))
	}
	if ipv4OK {
		n.Port = n.IPv4.Addr.Port
	}
	if ipv6OK {
		n.Port = n.IPv6.Addr.Port
	}

	errors = append(errors, n.PCAP.validate()...)
	errors = append(errors, n.TCP.validate()...)

	return errors
}

func (n *Addr) validate() []error {
	var errors []error

	l, err := validateAddr(n.Addr_, false)
	if err != nil {
		errors = append(errors, err)
	}
	n.Addr = l

	if n.RouterMac_ == "" {
		errors = append(errors, fmt.Errorf("router MAC address is required"))
	}

	hwAddr, err := net.ParseMAC(n.RouterMac_)
	if err != nil {
		errors = append(errors, fmt.Errorf("invalid Router MAC address '%s': %v", n.RouterMac_, err))
	}
	n.Router = hwAddr

	return errors
}
