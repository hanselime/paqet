package conf

import (
	"fmt"
	"net"
	"strconv"
)

func validateAddr(addr string, vPort bool) (*net.UDPAddr, error) {
	if addr == "" {
		return nil, fmt.Errorf("address is required")
	}

	uAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address '%s': %v", addr, err)
	}

	if vPort {
		if uAddr.Port < 1 || uAddr.Port > 65535 {
			return nil, fmt.Errorf("port must be between 1-65535")
		}
	}

	return uAddr, nil
}

func validateTCPAddr(addr string, vPort bool) (*net.TCPAddr, error) {
	if addr == "" {
		return nil, fmt.Errorf("address is required")
	}

	tAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address '%s': %v", addr, err)
	}

	if vPort {
		if tAddr.Port < 1 || tAddr.Port > 65535 {
			return nil, fmt.Errorf("port must be between 1-65535")
		}
	}

	return tAddr, nil
}

func validateHostPort(addr string, vPort bool) error {
	if addr == "" {
		return fmt.Errorf("address is required")
	}

	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid address '%s': %v", addr, err)
	}

	if vPort {
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("port must be between 1-65535")
		}
	}

	return nil
}

// func validateMAC(mac string) (net.HardwareAddr, error) {
// 	if mac == "" {
// 		return nil, fmt.Errorf("MAC address is required")
// 	}

// 	hwAddr, err := net.ParseMAC(mac)
// 	if err != nil {
// 		return nil, fmt.Errorf("invalid MAC address '%s': %v", mac, err)
// 	}

// 	return hwAddr, nil
// }
