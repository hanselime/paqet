package server

import (
	"context"
	"net"
	"paqet/internal/flog"
	"paqet/internal/pkg/buffer"
	"paqet/internal/protocol"
	"paqet/internal/tnet"
	"time"
)

func (s *Server) handleTCPProtocol(ctx context.Context, strm tnet.Strm, p *protocol.Proto) error {
	flog.Infof("accepted TCP stream %d: %s -> %s", strm.SID(), strm.RemoteAddr(), p.Addr.String())
	return s.handleTCP(ctx, strm, p.Addr.String())
}

func (s *Server) handleTCP(ctx context.Context, strm tnet.Strm, addr string) error {
	var conn net.Conn
	var err error
	
	// Try to get connection from pool if enabled
	pool, poolErr := s.getConnPool(addr)
	if poolErr != nil {
		flog.Warnf("failed to get connection pool for %s: %v, falling back to direct dial", addr, poolErr)
	}
	
	if pool != nil {
		conn, err = pool.Get(ctx)
		if err != nil {
			flog.Errorf("failed to get connection from pool for %s: %v, falling back to direct dial", addr, err)
			pool = nil // Disable pooling for this connection
		}
	}
	
	// Fall back to direct dial if pooling is disabled or failed
	if pool == nil {
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		conn, err = dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			flog.Errorf("failed to establish TCP connection to %s for stream %d: %v", addr, strm.SID(), err)
			return err
		}
	}
	
	defer func() {
		conn.Close()
		flog.Debugf("closed TCP connection %s for stream %d", addr, strm.SID())
	}()
	flog.Debugf("TCP connection established to %s for stream %d", addr, strm.SID())

	errChan := make(chan error, 2)
	go func() {
		err := buffer.CopyT(conn, strm)
		errChan <- err
	}()
	go func() {
		err := buffer.CopyT(strm, conn)
		errChan <- err
	}()

	select {
	case err := <-errChan:
		if err != nil {
			flog.Errorf("TCP stream %d to %s failed: %v", strm.SID(), addr, err)
			// Mark connection as unusable if it's from a pool
			if pc, ok := conn.(interface{ MarkUnusable() }); ok {
				pc.MarkUnusable()
			}
			return err
		}
	case <-ctx.Done():
	}
	return nil
}
