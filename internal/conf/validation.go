package conf

import (
	"fmt"
	"net"
	"strings"
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

func parseMAC(mac string) (net.HardwareAddr, error) {
	mac = strings.TrimSpace(mac)
	if mac == "" {
		return nil, fmt.Errorf("MAC address is required")
	}

	hwAddr, err := net.ParseMAC(mac)
	if err == nil {
		return hwAddr, nil
	}

	normalized, ok := normalizeMAC48(mac)
	if !ok {
		return nil, fmt.Errorf("invalid MAC address '%s': %v", mac, err)
	}

	hwAddr, normalizeErr := net.ParseMAC(normalized)
	if normalizeErr != nil {
		return nil, fmt.Errorf("invalid MAC address '%s': %v", mac, normalizeErr)
	}

	return hwAddr, nil
}

// normalizeMAC48 accepts MAC-48 values split by ":" or "-" and left-pads
// single-character octets (macOS ARP output can use this form).
func normalizeMAC48(mac string) (string, bool) {
	separator := ""
	switch {
	case strings.Contains(mac, ":"):
		separator = ":"
	case strings.Contains(mac, "-"):
		separator = "-"
	default:
		return "", false
	}

	parts := strings.Split(mac, separator)
	if len(parts) != 6 {
		return "", false
	}

	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || len(part) > 2 || !isHex(part) {
			return "", false
		}
		if len(part) == 1 {
			part = "0" + part
		}
		normalized = append(normalized, strings.ToLower(part))
	}

	return strings.Join(normalized, ":"), true
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
