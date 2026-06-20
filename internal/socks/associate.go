package socks

import (
	"context"
	"io"
	"net"
	"time"

	"paqet/internal/flog"
	"paqet/internal/pkg/buffer"
)

func (s *Server) handleAssociate(ctx context.Context, conn *net.TCPConn) {
	local := conn.LocalAddr().(*net.TCPAddr)
	reply := append([]byte{ver, repSuccess, 0x00}, appendAddr(nil, local.IP, local.Port)...)
	if _, err := conn.Write(reply); err != nil {
		return
	}
	flog.Debugf("SOCKS5 accepted UDP_ASSOCIATE from %s, holding control connection", conn.RemoteAddr())

	done := make(chan struct{})
	go func() {
		io.Copy(io.Discard, conn)
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		conn.Close()
		<-done
	}
	flog.Debugf("SOCKS5 UDP_ASSOCIATE control connection %s closed", conn.RemoteAddr())
}

func (s *Server) serveUDP(ctx context.Context) {
	buf := make([]byte, 64*1024)
	for {
		n, addr, err := s.udp.ReadFromUDP(buf)
		if err != nil {
			return
		}
		d, err := parseDatagram(buf[:n])
		if err != nil {
			flog.Debugf("SOCKS5 dropping malformed UDP datagram from %s: %v", addr, err)
			continue
		}
		s.relayDatagram(ctx, addr, d)
	}
}

func (s *Server) relayDatagram(ctx context.Context, addr *net.UDPAddr, d *datagram) {
	strm, isNew, key, err := s.client.UDP(addr.String(), d.address())
	if err != nil {
		flog.Errorf("SOCKS5 failed to establish UDP stream for %s -> %s: %v", addr, d.address(), err)
		return
	}

	strm.SetWriteDeadline(time.Now().Add(8 * time.Second))
	_, err = strm.Write(d.data)
	strm.SetWriteDeadline(time.Time{})
	if err != nil {
		flog.Errorf("SOCKS5 failed to forward %d bytes from %s -> %s: %v", len(d.data), addr, d.address(), err)
		s.client.CloseUDP(key)
		return
	}
	if !isNew {
		return
	}

	flog.Infof("SOCKS5 accepted UDP connection %s -> %s", addr, d.address())
	go func() {
		defer func() {
			flog.Debugf("SOCKS5 UDP stream %d closed for %s -> %s", strm.SID(), addr, d.address())
			s.client.CloseUDP(key)
		}()

		buf := make([]byte, buffer.UPool)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			strm.SetReadDeadline(time.Now().Add(8 * time.Second))
			n, err := strm.Read(buf)
			strm.SetReadDeadline(time.Time{})
			if err != nil {
				flog.Debugf("SOCKS5 UDP stream %d read error for %s -> %s: %v", strm.SID(), addr, d.address(), err)
				return
			}

			resp := (&datagram{atyp: d.atyp, addr: d.addr, port: d.port, data: buf[:n]}).bytes()
			if _, err := s.udp.WriteToUDP(resp, addr); err != nil {
				flog.Errorf("SOCKS5 failed to write UDP response to %s: %v", addr, err)
				return
			}
		}
	}()
}
