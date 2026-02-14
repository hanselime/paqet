package conf

import (
	"fmt"
	"net"
	"runtime"
	"time"
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
	PortPool   PortPool       `yaml:"port_pool"`
	TCPState   TCPState       `yaml:"tcp_state"`
	RateLimit  RateLimit      `yaml:"rate_limit"`
	Interface  *net.Interface `yaml:"-"`
	Port       int            `yaml:"-"`
}

func (n *Network) setDefaults(role string) {
	n.PCAP.setDefaults(role)
	n.TCP.setDefaults()
	n.PortPool.setDefaults()
	n.TCPState.setDefaults()
	n.RateLimit.setDefaults()
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
	if errs := n.TCP.validate(); len(errs) != 0 {
		errors = append(errors, errs...)
	}
	if errs := n.PortPool.validate(); len(errs) != 0 {
		errors = append(errors, errs...)
	}
	if errs := n.TCPState.validate(); len(errs) != 0 {
		errors = append(errors, errs...)
	}
	if errs := n.RateLimit.validate(); len(errs) != 0 {
		errors = append(errors, errs...)
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

type PortPool struct {
	Enabled   bool   `yaml:"enabled"`
	StartPort uint16 `yaml:"start_port"`
	EndPort   uint16 `yaml:"end_port"`
}

type TCPState struct {
	Enabled           bool          `yaml:"enabled"`
	ConnectionTimeout time.Duration `yaml:"connection_timeout"`
	CleanupInterval   time.Duration `yaml:"cleanup_interval"`
}

type RateLimit struct {
	Enabled           bool `yaml:"enabled"`
	PacketsPerSecond  int  `yaml:"packets_per_second"`
	Burst             int  `yaml:"burst"`
	Adaptive          bool `yaml:"adaptive"`
}

func (pp *PortPool) setDefaults() {
	if pp.Enabled && pp.StartPort == 0 {
		pp.StartPort = 50000
	}
	if pp.Enabled && pp.EndPort == 0 {
		pp.EndPort = 51000
	}
}

func (pp *PortPool) validate() []error {
	var errors []error
	
	if pp.Enabled {
		if pp.EndPort <= pp.StartPort {
			errors = append(errors, fmt.Errorf(
				"port_pool: end_port (%d) must be greater than start_port (%d)",
				pp.EndPort, pp.StartPort))
		}
		
		if pp.StartPort < 1024 {
			errors = append(errors, fmt.Errorf(
				"port_pool: start_port (%d) should be >= 1024 (avoid privileged ports)",
				pp.StartPort))
		}
		
		poolSize := pp.EndPort - pp.StartPort
		if poolSize < 10 {
			errors = append(errors, fmt.Errorf(
				"port_pool: pool size (%d) is too small, recommended at least 100 ports",
				poolSize))
		}
	}
	
	return errors
}

func (ts *TCPState) setDefaults() {
	if ts.Enabled {
		if ts.ConnectionTimeout == 0 {
			ts.ConnectionTimeout = 5 * time.Minute
		}
		if ts.CleanupInterval == 0 {
			ts.CleanupInterval = 60 * time.Second
		}
	}
}

func (ts *TCPState) validate() []error {
	var errors []error
	
	if ts.Enabled {
		if ts.ConnectionTimeout < 10*time.Second {
			errors = append(errors, fmt.Errorf(
				"tcp_state: connection_timeout (%s) is too short, minimum 10s recommended",
				ts.ConnectionTimeout))
		}
		
		if ts.CleanupInterval < 5*time.Second {
			errors = append(errors, fmt.Errorf(
				"tcp_state: cleanup_interval (%s) is too short, minimum 5s recommended",
				ts.CleanupInterval))
		}
		
		if ts.CleanupInterval > ts.ConnectionTimeout {
			errors = append(errors, fmt.Errorf(
				"tcp_state: cleanup_interval (%s) should be less than connection_timeout (%s)",
				ts.CleanupInterval, ts.ConnectionTimeout))
		}
	}
	
	return errors
}

func (rl *RateLimit) setDefaults() {
	if rl.Enabled {
		if rl.PacketsPerSecond == 0 {
			rl.PacketsPerSecond = 2000
		}
		if rl.Burst == 0 {
			rl.Burst = rl.PacketsPerSecond / 10
		}
	}
}

func (rl *RateLimit) validate() []error {
	var errors []error
	
	if rl.Enabled {
		if rl.PacketsPerSecond < 1 {
			errors = append(errors, fmt.Errorf(
				"rate_limit: packets_per_second must be >= 1"))
		}
		
		if rl.PacketsPerSecond > 100000 {
			errors = append(errors, fmt.Errorf(
				"rate_limit: packets_per_second (%d) is extremely high, may still overwhelm networks",
				rl.PacketsPerSecond))
		}
		
		if rl.Burst < 1 {
			errors = append(errors, fmt.Errorf(
				"rate_limit: burst must be >= 1"))
		}
		
		if rl.Burst > rl.PacketsPerSecond {
			errors = append(errors, fmt.Errorf(
				"rate_limit: burst (%d) should not exceed packets_per_second (%d)",
				rl.Burst, rl.PacketsPerSecond))
		}
	}
	
	return errors
}
