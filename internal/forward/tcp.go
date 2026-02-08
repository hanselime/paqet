package forward

import (
	"context"
	"net"
	"paqet/internal/flog"
	"paqet/internal/pkg/buffer"
)

func (f *Forward) listenTCP(ctx context.Context) error {
	listener, err := net.Listen("tcp", f.listenAddr)
	if err != nil {
		flog.Errorf("failed to bind TCP socket on %s: %v", f.listenAddr, err)
		return err
	}
	defer listener.Close()
	go func() {
		<-ctx.Done()
		listener.Close()
	}()
	flog.Infof("TCP forwarder listening on %s -> %s", f.listenAddr, f.targetAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				flog.Errorf("failed to accept TCP connection on %s: %v", f.listenAddr, err)
				continue
			}
		}

		// Acquire semaphore if configured (limits concurrent connections)
		if f.streamSemaphore != nil {
			select {
			case f.streamSemaphore <- struct{}{}:
				// Acquired, proceed
			case <-ctx.Done():
				conn.Close()
				return nil
			}
		}

		f.wg.Go(func() {
			defer func() {
				conn.Close()
				// Release semaphore
				if f.streamSemaphore != nil {
					<-f.streamSemaphore
				}
			}()
			if err := f.handleTCPConn(ctx, conn); err != nil {
				flog.Errorf("TCP connection %s -> %s closed with error: %v", conn.RemoteAddr(), f.targetAddr, err)
			} else {
				flog.Debugf("TCP connection %s -> %s closed", conn.RemoteAddr(), f.targetAddr)
			}
		})
	}
}

func (f *Forward) handleTCPConn(ctx context.Context, conn net.Conn) error {
	strm, err := f.client.TCP(f.targetAddr)
	if err != nil {
		flog.Errorf("failed to establish stream for %s -> %s: %v", conn.RemoteAddr(), f.targetAddr, err)
		return err
	}
	defer func() {
		flog.Debugf("TCP stream closed for %s -> %s", conn.RemoteAddr(), f.targetAddr)
		defer strm.Close()
	}()
	flog.Infof("accepted TCP connection %s -> %s", conn.RemoteAddr(), f.targetAddr)

	errCh := make(chan error, 2)
	go func() {
		err := buffer.CopyT(conn, strm)
		errCh <- err
	}()
	go func() {
		err := buffer.CopyT(strm, conn)
		errCh <- err
	}()

	select {
	case err := <-errCh:
		if err != nil {
			flog.Errorf("TCP stream %d failed for %s -> %s: %v", strm.SID(), conn.RemoteAddr(), f.targetAddr, err)
			return err
		}
	case <-ctx.Done():
	}

	return nil
}
