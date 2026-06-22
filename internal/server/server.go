package server

import (
	"context"
	"fmt"

	"paqet/internal/conf"
	"paqet/internal/flog"
	"paqet/internal/tnet"
	"paqet/internal/tnet/kcp"
)

type Server struct {
	cfg      *conf.Conf
	listener tnet.Listener
}

func New(cfg *conf.Conf) (*Server, error) {
	s := &Server{cfg: cfg}
	return s, nil
}

func (s *Server) Start(ctx context.Context) error {
	listener, err := kcp.Listen(ctx, s.cfg.Transport.KCP, s.cfg.Network)
	if err != nil {
		return fmt.Errorf("could not start KCP listener: %w", err)
	}
	s.listener = listener
	flog.Infof("Server started - listening for packets on :%d", s.cfg.Listen.Addr.Port)

	go func() {
		s.listen(ctx, listener)
		<-ctx.Done()
		listener.Close()
	}()

	return nil
}

func (s *Server) listen(ctx context.Context, listener tnet.Listener) {
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

		go func() {
			defer conn.Close()
			defer s.listener.DeleteClientTCPF(conn.RemoteAddr())
			s.handleConn(ctx, conn)
		}()
	}
}
