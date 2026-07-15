package socks

import (
	"context"
	"io"
	"net"
	"time"

	"paqet/internal/flog"
	"paqet/internal/pkg/buffer"
	"paqet/internal/tnet"
)

type associate struct {
	conn  *net.UDPConn
	cAddr *net.UDPAddr
}

func (a *associate) accept(cAddr *net.UDPAddr) bool {
	if !cAddr.IP.Equal(a.cAddr.IP) {
		return false
	}
	if a.cAddr.Port == 0 {
		a.cAddr.Port = cAddr.Port
	}
	return cAddr.Port == a.cAddr.Port
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

	a := &associate{conn: conn, cAddr: &net.UDPAddr{}}
	if req.atyp == atypDomain || net.IP(req.addr).IsUnspecified() {
		a.cAddr.IP = tConn.RemoteAddr().(*net.TCPAddr).IP
	} else {
		a.cAddr.IP = net.IP(req.addr)
		a.cAddr.Port = int(req.port[0])<<8 | int(req.port[1])
	}
	flog.Debugf("SOCKS5 UDP_ASSOCIATE from %s relay=%s expect=%s", tConn.RemoteAddr(), bAddr, a.cAddr.IP)

	go func() {
		io.Copy(io.Discard, tConn)
		conn.Close()
	}()
	go func() {
		<-ctx.Done()
		conn.Close()
		tConn.Close()
	}()

	s.serveUDP(ctx, a)
	flog.Debugf("SOCKS5 UDP_ASSOCIATE control connection %s closed", tConn.RemoteAddr())
}

func (s *Server) serveUDP(ctx context.Context, a *associate) {
	buf := make([]byte, buffer.UPool)
	for {
		n, cAddr, err := a.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if !a.accept(cAddr) {
			flog.Debugf("SOCKS5 dropping UDP from unexpected source %s (want %s)", cAddr, a.cAddr.IP)
			continue
		}
		d, err := decodeDatagram(buf[:n])
		if err != nil {
			flog.Debugf("SOCKS5 dropping malformed UDP datagram from %s: %v", cAddr, err)
			continue
		}
		s.handleUDPConn(ctx, a, d)
	}
}

func (s *Server) handleUDPConn(ctx context.Context, a *associate, d *datagram) {
	strm, new, k, err := s.client.UDP(ctx, a.cAddr.String(), d.address())
	if err != nil {
		flog.Errorf("SOCKS5 failed to establish UDP stream for %s -> %s: %v", a.cAddr, d.address(), err)
		return
	}

	strm.SetWriteDeadline(time.Now().Add(8 * time.Second))
	_, err = strm.Write(d.data)
	strm.SetWriteDeadline(time.Time{})
	if err != nil {
		s.client.CloseUDP(k, strm)
		return
	}

	if new {
		flog.Infof("SOCKS5 accepted UDP connection %s -> %s", a.cAddr, d.address())
		hdr := (&datagram{atyp: d.atyp, addr: d.addr, port: d.port}).bytes()
		go func() {
			defer s.client.CloseUDP(k, strm)
			s.handleUDPStrm(ctx, strm, a, hdr)
		}()
	}
}

func (s *Server) handleUDPStrm(ctx context.Context, strm tnet.Strm, a *associate, hdr []byte) {
	stop := context.AfterFunc(ctx, func() { strm.Close() })
	defer stop()

	buf := make([]byte, buffer.UPool)
	hlen := copy(buf, hdr)
	for {
		strm.SetReadDeadline(time.Now().Add(8 * time.Second))
		n, err := strm.Read(buf[hlen:])
		strm.SetReadDeadline(time.Time{})
		if err != nil {
			return
		}
		if _, err := a.conn.WriteToUDP(buf[:hlen+n], a.cAddr); err != nil {
			return
		}
	}
}
