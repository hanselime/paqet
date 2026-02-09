package quic

import (
	"context"
	"crypto/tls"
	"net"
	"paqet/internal/conf"
	"paqet/internal/socket"
	"paqet/internal/tnet"
	"time"

	"github.com/quic-go/quic-go"
)

type Listener struct {
	packetConn *socket.PacketConn
	cfg        *conf.QUIC
	listener   *quic.Listener
	tlsConfig  *tls.Config
	ctx        context.Context
}

func Listen(cfg *conf.QUIC, pConn *socket.PacketConn) (tnet.Listener, error) {
	// Generate TLS config for server
	tlsConfig, err := cfg.GenerateTLSConfig("server")
	if err != nil {
		return nil, err
	}

	// Create QUIC config
	quicConfig := getQUICConfig(cfg)

	// Create QUIC listener using the packet connection
	listener, err := quic.Listen(pConn, tlsConfig, quicConfig)
	if err != nil {
		return nil, err
	}

	return &Listener{
		packetConn: pConn,
		cfg:        cfg,
		listener:   listener,
		tlsConfig:  tlsConfig,
		ctx:        context.Background(),
	}, nil
}

// SetContext allows setting a context for the listener for proper cancellation
func (l *Listener) SetContext(ctx context.Context) {
	l.ctx = ctx
}

func (l *Listener) Accept() (tnet.Conn, error) {
	// Use listener's context with timeout to prevent indefinite blocking
	ctx := l.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Accept with timeout to allow periodic context checks
	// Use a loop instead of recursion to prevent stack overflow under sustained timeouts
	for {
		// Add timeout to Accept to allow periodic context checks
		acceptCtx, cancel := context.WithTimeout(ctx, 5*time.Second)

		qconn, err := l.listener.Accept(acceptCtx)
		cancel() // Clean up context immediately

		if err != nil {
			// If timeout, check if parent context was cancelled
			if err == context.DeadlineExceeded {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
					// Parent context not cancelled, just a timeout, continue loop
					continue
				}
			}
			return nil, err
		}

		// Pass listener's context to connection for proper shutdown propagation
		return newConnWithContext(qconn, l.ctx), nil
	}
}

func (l *Listener) Close() error {
	var firstErr error

	if l.listener != nil {
		if err := l.listener.Close(); err != nil {
			firstErr = err
		}
	}
	if l.packetConn != nil {
		if err := l.packetConn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (l *Listener) Addr() net.Addr {
	return l.listener.Addr()
}
