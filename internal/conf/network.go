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
	IPv4TOS    int            `yaml:"ipv4_tos"`
	IPv4DF     bool           `yaml:"ipv4_df"`
	IPv4TTL    int            `yaml:"ipv4_ttl"`
	IPv6TC     int            `yaml:"ipv6_tc"`
	IPv6Hop    int            `yaml:"ipv6_hoplimit"`
	Interface  *net.Interface `yaml:"-"`
	Port       int            `yaml:"-"`
}

func (n *Network) setDefaults(role string) {
	n.PCAP.setDefaults(role)
	n.TCP.setDefaults()
	if n.IPv4TTL == 0 {
		n.IPv4TTL = 64
	}
	if n.IPv6Hop == 0 {
		n.IPv6Hop = 64
	}
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

	ipv4Configured := n.IPv4.Addr_ != ""
	ipv6Configured := n.IPv6.Addr_ != ""
	if !ipv4Configured && !ipv6Configured {
		errors = append(errors, fmt.Errorf("at least one address family (IPv4 or IPv6) must be configured"))
		return errors
	}
	if ipv4Configured {
		errors = append(errors, n.IPv4.validate()...)
	}
	if ipv6Configured {
		errors = append(errors, n.IPv6.validate()...)
	}
	if ipv4Configured && ipv6Configured {
		if n.IPv4.Addr.Port != n.IPv6.Addr.Port {
			errors = append(errors, fmt.Errorf("IPv4 port (%d) and IPv6 port (%d) must match when both are configured", n.IPv4.Addr.Port, n.IPv6.Addr.Port))
		}
	}
	if n.IPv4.Addr != nil {
		n.Port = n.IPv4.Addr.Port
	}
	if n.IPv6.Addr != nil {
		n.Port = n.IPv6.Addr.Port
	}

	errors = append(errors, n.PCAP.validate()...)
	errors = append(errors, n.TCP.validate()...)
	if n.IPv4TOS < 0 || n.IPv4TOS > 255 {
		errors = append(errors, fmt.Errorf("ipv4_tos must be between 0-255"))
	}
	if n.IPv6TC < 0 || n.IPv6TC > 255 {
		errors = append(errors, fmt.Errorf("ipv6_tc must be between 0-255"))
	}
	if n.IPv4TTL < 1 || n.IPv4TTL > 255 {
		errors = append(errors, fmt.Errorf("ipv4_ttl must be between 1-255"))
	}
	if n.IPv6Hop < 1 || n.IPv6Hop > 255 {
		errors = append(errors, fmt.Errorf("ipv6_hoplimit must be between 1-255"))
	}

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
		errors = append(errors, fmt.Errorf("Router MAC address is required"))
	}

	hwAddr, err := net.ParseMAC(n.RouterMac_)
	if err != nil {
		errors = append(errors, fmt.Errorf("invalid Router MAC address '%s': %v", n.RouterMac_, err))
	}
	n.Router = hwAddr

	return errors
}
