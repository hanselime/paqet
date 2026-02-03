//go:build windows

package conf

import (
	"fmt"
	"net"

	"github.com/gopacket/gopacket/pcap"
)

func getWindowsGUID(iface *net.Interface) (string, error) {
	// Get all addresses associated with the target interface
	ifaceAddrs, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("failed to get interface addresses: %v", err)
	}

	if len(ifaceAddrs) == 0 {
		return "", fmt.Errorf("interface %s has no IP addresses to match against", iface.Name)
	}

	// Get all pcap devices
	pcapDevs, err := pcap.FindAllDevs()
	if err != nil {
		return "", fmt.Errorf("failed to list pcap devices: %v", err)
	}

	// Try to match by IP address
	for _, dev := range pcapDevs {
		for _, devAddr := range dev.Addresses {
			for _, netAddr := range ifaceAddrs {
				var targetIP net.IP
				switch v := netAddr.(type) {
				case *net.IPNet:
					targetIP = v.IP
				case *net.IPAddr:
					targetIP = v.IP
				}

				if targetIP != nil && devAddr.IP.Equal(targetIP) {
					return dev.Name, nil
				}
			}
		}
	}

	return "", fmt.Errorf("could not find pcap device matching interface %s (checked %d addresses)", iface.Name, len(ifaceAddrs))
}
