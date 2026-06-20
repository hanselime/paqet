package socks

import (
	"context"
	"net"

	"paqet/internal/flog"
	"paqet/internal/pkg/buffer"
)

func (s *Server) handleConnect(ctx context.Context, conn net.Conn, req *request) {
	strm, err := s.client.TCP(req.address())
	if err != nil {
		flog.Errorf("SOCKS5 failed to establish stream for %s -> %s: %v", conn.RemoteAddr(), req.address(), err)
		s.write(conn, repFailure)
		return
	}
	defer strm.Close()
	flog.Infof("SOCKS5 accepted TCP connection %s -> %s", conn.RemoteAddr(), req.address())

	lAddr := conn.LocalAddr().(*net.TCPAddr)
	if _, err := conn.Write(append([]byte{ver, repSuccess, 0x00}, putAddr(nil, lAddr.IP, lAddr.Port)...)); err != nil {
		return
	}
	flog.Debugf("SOCKS5 stream %d created for %s -> %s", strm.SID(), conn.RemoteAddr(), req.address())

	errCh := make(chan error, 2)
	go func() { errCh <- buffer.CopyT(conn, strm) }()
	go func() { errCh <- buffer.CopyT(strm, conn) }()

	select {
	case err := <-errCh:
		if err != nil {
			flog.Errorf("SOCKS5 stream %d failed for %s -> %s: %v", strm.SID(), conn.RemoteAddr(), req.address(), err)
		}
	case <-ctx.Done():
		flog.Debugf("SOCKS5 connection %s -> %s closed due to shutdown", conn.RemoteAddr(), req.address())
	}

	flog.Debugf("SOCKS5 connection %s -> %s closed", conn.RemoteAddr(), req.address())
}
