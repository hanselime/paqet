package server

import (
	"context"
	"net"
	"time"

	"paqet/internal/flog"
	"paqet/internal/metrics"
	"paqet/internal/pkg/buffer"
	"paqet/internal/protocol"
	"paqet/internal/tnet"
)

func (s *Server) handleTCPProtocol(ctx context.Context, strm tnet.Strm, p *protocol.Proto) {
	flog.Infof("accepted TCP stream %d: %s -> %s", strm.SID(), strm.RemoteAddr(), p.Addr.String())
	s.handleTCP(ctx, strm, p.Addr.String())
}

func (s *Server) handleTCP(ctx context.Context, strm tnet.Strm, addr string) {
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		flog.Errorf("failed to establish TCP connection to %s for stream %d: %v", addr, strm.SID(), err)
		metrics.IncFailure()
		metrics.IncFailureTCP()
		return
	}
	defer func() {
		conn.Close()
		flog.Debugf("closed TCP connection %s for stream %d", addr, strm.SID())
	}()

	errChan := make(chan error, 2)
	go func() { errChan <- buffer.CopyT(conn, strm) }()
	go func() { errChan <- buffer.CopyT(strm, conn) }()

	select {
	case err := <-errChan:
		if err != nil {
			flog.Errorf("TCP stream %d failed for %s -> %s: %v", strm.SID(), conn.RemoteAddr(), addr, err)
			metrics.IncFailure()
			metrics.IncFailureTCP()
			return
		}
	case <-ctx.Done():
	}
	metrics.IncSuccess()
	metrics.IncSuccessTCP()
}
