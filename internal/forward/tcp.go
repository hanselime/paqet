package forward

import (
	"context"
	"net"

	"paqet/internal/flog"
	"paqet/internal/pkg/buffer"
)

func (f *Forward) serveTCP(ctx context.Context, listener net.Listener) {
	flog.Infof("TCP forwarder listening on %s -> %s", f.listenAddr, f.targetAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				flog.Errorf("failed to accept TCP connection on %s: %v", f.listenAddr, err)
				continue
			}
		}

		go func() {
			defer conn.Close()
			f.handleTCPConn(ctx, conn)
		}()
	}
}

func (f *Forward) handleTCPConn(ctx context.Context, conn net.Conn) {
	strm, err := f.client.TCP(f.targetAddr)
	if err != nil {
		flog.Errorf("failed to establish stream for %s -> %s: %v", conn.RemoteAddr(), f.targetAddr, err)
		return
	}
	defer strm.Close()
	flog.Infof("accepted TCP connection %s -> %s", conn.RemoteAddr(), f.targetAddr)

	errCh := make(chan error, 2)
	go func() { errCh <- buffer.CopyT(conn, strm) }()
	go func() { errCh <- buffer.CopyT(strm, conn) }()

	select {
	case err := <-errCh:
		if err != nil {
			flog.Errorf("TCP stream %d failed for %s -> %s: %v", strm.SID(), conn.RemoteAddr(), f.targetAddr, err)
		}
	case <-ctx.Done():
	}
}
