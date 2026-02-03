//go:build !windows

package conf

import (
	"fmt"
	"net"
)

func getWindowsGUID(iface *net.Interface) (string, error) {
	return "", fmt.Errorf("windows GUID discovery is not supported on this OS")
}
