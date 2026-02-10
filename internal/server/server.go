package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"paqet/internal/conf"
	"paqet/internal/flog"
	"paqet/internal/socket"
	"paqet/internal/tnet"
	"paqet/internal/tnet/kcp"
)

type Server struct {
	cfg    *conf.Conf
	dialer Dialer
	pConn  *socket.PacketConn
	wg     sync.WaitGroup
}

func New(cfg *conf.Conf) (*Server, error) {
	var dialer Dialer
	if cfg.Outbound.Type == "socks5" {
		d, err := newSOCKS5Dialer(cfg.Outbound.Addr, cfg.Outbound.Username, cfg.Outbound.Password)
		if err != nil {
			return nil, fmt.Errorf("outbound socks5: %w", err)
		}
		dialer = d
	} else {
		dialer = newDirectDialer()
	}
	s := &Server{
		cfg:    cfg,
		dialer: dialer,
	}
	return s, nil
}

func (s *Server) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		flog.Infof("Shutdown signal received, initiating graceful shutdown...")
		cancel()
	}()

	pConn, err := socket.New(ctx, &s.cfg.Network)
	if err != nil {
		return fmt.Errorf("could not create raw packet conn: %w", err)
	}
	s.pConn = pConn

	listener, err := kcp.Listen(s.cfg.Transport.KCP, pConn)
	if err != nil {
		return fmt.Errorf("could not start KCP listener: %w", err)
	}
	defer listener.Close()
	flog.Infof("Server started - listening for packets on :%d", s.cfg.Listen.Addr.Port)

	s.wg.Go(func() {
		s.listen(ctx, listener)
	})

	s.wg.Wait()
	flog.Infof("Server shutdown completed")
	return nil
}

func (s *Server) listen(ctx context.Context, listener tnet.Listener) {
	go func() {
		<-ctx.Done()
		listener.Close()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		conn, err := listener.Accept()
		if err != nil {
			flog.Errorf("failed to accept connection: %v", err)
			continue
		}
		flog.Infof("accepted new connection from %s (local: %s)", conn.RemoteAddr(), conn.LocalAddr())

		s.wg.Go(func() {
			defer conn.Close()
			s.handleConn(ctx, conn)
		})
	}
}
