package socks

import (
	"context"
	"net"

	"paqet/internal/flog"
	"paqet/internal/pkg/buffer"
)

func (s *Server) handleConnect(ctx context.Context, conn *net.TCPConn, req *request) {
	flog.Infof("SOCKS5 accepted TCP connection %s -> %s", conn.RemoteAddr(), req.address())

	local := conn.LocalAddr().(*net.TCPAddr)
	reply := append([]byte{ver, repSuccess, 0x00}, appendAddr(nil, local.IP, local.Port)...)
	if _, err := conn.Write(reply); err != nil {
		return
	}

	strm, err := s.client.TCP(req.address())
	if err != nil {
		flog.Errorf("SOCKS5 failed to establish stream for %s -> %s: %v", conn.RemoteAddr(), req.address(), err)
		return
	}
	defer strm.Close()
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
