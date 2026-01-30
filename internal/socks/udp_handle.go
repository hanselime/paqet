package socks

import (
	"io"
	"net"
	"paqet/internal/flog"
	"paqet/internal/pkg/buffer"
	"time"

	"github.com/txthinking/socks5"
)

func (h *Handler) UDPHandle(server *socks5.Server, addr *net.UDPAddr, d *socks5.Datagram) error {
	bufp := buffer.UPool.Get().(*[]byte)
	defer buffer.UPool.Put(bufp)
	buf := *bufp
	strm, new, k, err := h.client.UDP(addr.String(), d.Address())
	if err != nil {
		flog.Errorf("SOCKS5 failed to establish UDP stream for %s -> %s: %v", addr, d.Address(), err)
		return err
	}
	strm.SetWriteDeadline(time.Now().Add(8 * time.Second))
	_, err = strm.Write(d.Data)
	strm.SetWriteDeadline(time.Time{})
	if err != nil {
		flog.Errorf("SOCKS5 failed to forward %d bytes from %s -> %s: %v", len(d.Data), addr, d.Address(), err)
		h.client.CloseUDP(k)
		return err
	}

	if new {
		flog.Infof("SOCKS5 accepted UDP connection %s -> %s", addr, d.Address())
		go func() {
			defer func() {
				flog.Debugf("SOCKS5 UDP stream %d closed for %s -> %s", strm.SID(), addr, d.Address())
				h.client.CloseUDP(k)
			}()
			for {
				select {
				case <-h.ctx.Done():
					return
				default:
					strm.SetDeadline(time.Now().Add(8 * time.Second))
					n, err := strm.Read(buf)
					strm.SetDeadline(time.Time{})
					if err != nil {
						flog.Debugf("SOCKS5 UDP stream %d read error for %s -> %s: %v", strm.SID(), addr, d.Address(), err)
						return
					}
					dd := socks5.NewDatagram(d.Atyp, d.DstAddr, d.DstPort, buf[:n])
					_, err = server.UDPConn.WriteToUDP(dd.Bytes(), addr)
					if err != nil {
						flog.Errorf("SOCKS5 failed to write UDP response %d bytes to %s: %v", len(dd.Bytes()), addr, err)
						return
					}
				}
			}
		}()
	}
	return nil
}

func (h *Handler) handleUDPAssociate(conn *net.TCPConn) error {
	addr := conn.LocalAddr().(*net.TCPAddr)

	bufp := rPool.Get().(*[]byte)
	defer rPool.Put(bufp)
	buf := *bufp
	buf = append(buf, socks5.Ver)
	buf = append(buf, socks5.RepSuccess)
	buf = append(buf, 0x00) // reserved
	if ip4 := addr.IP.To4(); ip4 != nil {
		// IPv4
		buf = append(buf, socks5.ATYPIPv4)
		buf = append(buf, ip4...)
	} else if ip6 := addr.IP.To16(); ip6 != nil {
		// IPv6
		buf = append(buf, socks5.ATYPIPv6)
		buf = append(buf, ip6...)
	} else {
		// Domain name
		host := addr.IP.String()
		buf = append(buf, socks5.ATYPDomain)
		buf = append(buf, byte(len(host)))
		buf = append(buf, host...)
	}
	buf = append(buf, byte(addr.Port>>8), byte(addr.Port&0xff))

	if _, err := conn.Write(buf); err != nil {
		return err
	}
	flog.Debugf("SOCKS5 accepted UDP_ASSOCIATE from %s, waiting for TCP connection to close", conn.RemoteAddr())

	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, conn)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil && h.ctx.Err() == nil {
			flog.Errorf("SOCKS5 TCP connection for UDP associate closed with: %v", err)
		}
	case <-h.ctx.Done():
		conn.Close() // Force close the connection to unblock io.Copy
		<-done       // Wait for the goroutine to finish
		flog.Debugf("SOCKS5 UDP_ASSOCIATE connection %s closed due to shutdown", conn.RemoteAddr())
	}

	flog.Debugf("SOCKS5 UDP_ASSOCIATE TCP connection %s closed", conn.RemoteAddr())
	return nil
}
