package conf

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
)

func getGatewayMAC(ifaceName string, isIPv6 bool) (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("gateway auto-discovery is only supported on Linux")
	}
	if isIPv6 {
		return "", fmt.Errorf("IPv6 gateway auto-discovery is not yet implemented")
	}

	gatewayIP, err := getIPv4DefaultGateway(ifaceName)
	if err != nil {
		return "", err
	}

	mac, err := getIPv4ARP(gatewayIP)
	if err != nil {
		return "", fmt.Errorf("gateway IP %s found, but MAC not in ARP cache: %v", gatewayIP, err)
	}

	return mac, nil
}

func getIPv4DefaultGateway(ifaceName string) (string, error) {
	file, err := os.Open("/proc/net/route")
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
	}

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 {
			iface := fields[0]
			dest := fields[1]
			gatewayHex := fields[2]

			if iface == ifaceName && dest == "00000000" {
				ipBytes, err := hex.DecodeString(gatewayHex)
				if err != nil {
					continue
				}
				if len(ipBytes) != 4 {
					continue
				}
				ip := net.IPv4(ipBytes[3], ipBytes[2], ipBytes[1], ipBytes[0])
				return ip.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no default route found for interface %s", ifaceName)
}

func getIPv4ARP(ip string) (string, error) {
	file, err := os.Open("/proc/net/arp")
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 4 {
			entryIP := fields[0]
			mac := fields[3]

			if entryIP == ip {
				if mac == "00:00:00:00:00:00" {
					return "", fmt.Errorf("incomplete ARP entry")
				}
				return mac, nil
			}
		}
	}
	return "", fmt.Errorf("IP %s not found in ARP cache", ip)
}
