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

func (s *Server) handleUDPProtocol(ctx context.Context, strm tnet.Strm, p *protocol.Proto) {
	flog.Infof("accepted UDP stream %d: %s -> %s", strm.SID(), strm.RemoteAddr(), p.Addr.String())
	s.handleUDP(ctx, strm, p.Addr.String())
}

func (s *Server) handleUDP(ctx context.Context, strm tnet.Strm, addr string) {
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	conn, err := dialer.DialContext(ctx, "udp", addr)
	if err != nil {
		flog.Errorf("failed to establish UDP connection to %s for stream %d: %v", addr, strm.SID(), err)
		metrics.IncFailure()
		metrics.IncFailureUDP()
		return
	}
	defer func() {
		conn.Close()
		flog.Debugf("closed UDP connection %s for stream %d", addr, strm.SID())
	}()

	errChan := make(chan error, 2)
	go func() { errChan <- buffer.CopyU(conn, strm) }()
	go func() { errChan <- buffer.CopyU(strm, conn) }()

	select {
	case err := <-errChan:
		if err != nil {
			flog.Errorf("UDP stream %d failed for %s -> %s: %v", strm.SID(), conn.RemoteAddr(), addr, err)
			metrics.IncFailure()
			metrics.IncFailureUDP()
		}
	case <-ctx.Done():
	}

	metrics.IncSuccess()
	metrics.IncSuccessUDP()
}
