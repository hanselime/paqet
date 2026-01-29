package buffer

import (
	"io"
	"net"
)

// reads from dst and writes to src UDP address using provided buffer
func CopyU(dst io.ReadWriter, src *net.UDPConn, addr *net.UDPAddr, buf []byte) error {
	n, err := dst.Read(buf)
	if err != nil {
		return err
	}

	_, err = src.WriteToUDP(buf[:n], addr)
	return err
}
