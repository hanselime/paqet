package socks

import (
	"context"
	"io"
	"net"
	"time"

	"paqet/internal/flog"
	"paqet/internal/pkg/buffer"
)

type associate struct {
	conn *net.UDPConn
	peer *net.UDPAddr
}

func (a *associate) accept(src *net.UDPAddr) bool {
	if !src.IP.Equal(a.peer.IP) {
		return false
	}
	if a.peer.Port == 0 {
		a.peer.Port = src.Port
	}
	return src.Port == a.peer.Port
}

func (s *Server) handleAssociate(ctx context.Context, tConn net.Conn, req *request) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	lAddr := tConn.LocalAddr().(*net.TCPAddr)
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: lAddr.IP, Port: 0})
	if err != nil {
		flog.Errorf("SOCKS5 failed to open UDP relay socket: %v", err)
		s.write(tConn, repFailure)
		return
	}
	defer conn.Close()

	bAddr := conn.LocalAddr().(*net.UDPAddr)
	if _, err := tConn.Write(append([]byte{ver, repSuccess, 0x00}, putAddr(nil, bAddr.IP, bAddr.Port)...)); err != nil {
		return
	}

	a := &associate{conn: conn, peer: &net.UDPAddr{}}
	if req.atyp == atypDomain || net.IP(req.addr).IsUnspecified() {
		a.peer.IP = tConn.RemoteAddr().(*net.TCPAddr).IP
	} else {
		a.peer.IP = net.IP(req.addr)
		a.peer.Port = int(req.port[0])<<8 | int(req.port[1])
	}
	flog.Debugf("SOCKS5 UDP_ASSOCIATE from %s relay=%s expect=%s", tConn.RemoteAddr(), bAddr, a.peer.IP)

	go func() {
		io.Copy(io.Discard, tConn)
		conn.Close()
	}()
	go func() {
		<-ctx.Done()
		conn.Close()
		tConn.Close()
	}()

	s.handleUDPPacket(ctx, a)
	flog.Debugf("SOCKS5 UDP_ASSOCIATE control connection %s closed", tConn.RemoteAddr())
}

func (s *Server) handleUDPPacket(ctx context.Context, a *associate) {
	buf := make([]byte, 64*1024)
	for {
		n, src, err := a.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if !a.accept(src) {
			flog.Debugf("SOCKS5 dropping UDP from unexpected source %s (want %s)", src, a.peer.IP)
			continue
		}
		d, err := decodeDatagram(buf[:n])
		if err != nil {
			flog.Debugf("SOCKS5 dropping malformed UDP datagram from %s: %v", src, err)
			continue
		}
		s.handleUDPStrm(ctx, a, d)
	}
}

func (s *Server) handleUDPStrm(ctx context.Context, a *associate, d *datagram) {
	strm, isNew, key, err := s.client.UDP(a.peer.String(), d.address())
	if err != nil {
		flog.Errorf("SOCKS5 failed to establish UDP stream for %s -> %s: %v", a.peer, d.address(), err)
		return
	}

	strm.SetWriteDeadline(time.Now().Add(8 * time.Second))
	_, err = strm.Write(d.data)
	strm.SetWriteDeadline(time.Time{})
	if err != nil {
		flog.Errorf("SOCKS5 failed to forward %d bytes from %s -> %s: %v", len(d.data), a.peer, d.address(), err)
		s.client.CloseUDP(key)
		return
	}
	if !isNew {
		return
	}

	flog.Infof("SOCKS5 accepted UDP connection %s -> %s", a.peer, d.address())

	respAtyp := d.atyp
	respAddr := append([]byte(nil), d.addr...)
	respPort := append([]byte(nil), d.port...)
	go func() {
		defer s.client.CloseUDP(key)
		go func() {
			<-ctx.Done()
			strm.Close()
		}()
		buf := make([]byte, buffer.UPool)
		for {
			strm.SetReadDeadline(time.Now().Add(8 * time.Second))
			n, err := strm.Read(buf)
			strm.SetReadDeadline(time.Time{})
			if err != nil {
				flog.Debugf("SOCKS5 UDP stream %d read error for %s: %v", strm.SID(), a.peer, err)
				return
			}
			resp := (&datagram{atyp: respAtyp, addr: respAddr, port: respPort, data: buf[:n]}).bytes()
			if _, err := a.conn.WriteToUDP(resp, a.peer); err != nil {
				return
			}
		}
	}()
}
