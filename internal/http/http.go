package http

import (
	"context"
	"net"
	"net/http"
	"paqet/internal/client"
	"paqet/internal/conf"
	"paqet/internal/flog"
	"time"
)

type HTTP struct {
	client   *client.Client
	username string
	password string
}

func New(client *client.Client) (*HTTP, error) {
	return &HTTP{
		client: client,
	}, nil
}

func (h *HTTP) Start(ctx context.Context, cfg conf.HTTP) error {
	h.username = cfg.Username
	h.password = cfg.Password
	go h.listen(ctx, cfg)
	return nil
}

func (h *HTTP) listen(ctx context.Context, cfg conf.HTTP) {
	// cfg.Listen is already validated, so this should not fail
	listenAddr, err := net.ResolveTCPAddr("tcp", cfg.Listen.String())
	if err != nil {
		flog.Fatalf("HTTP proxy failed to resolve address %s: %v", cfg.Listen.String(), err)
		return
	}
	listener, err := net.ListenTCP("tcp", listenAddr)
	if err != nil {
		flog.Fatalf("HTTP proxy failed to listen on %s: %v", listenAddr.String(), err)
		return
	}

	flog.Infof("HTTP proxy listening on %s", listenAddr.String())

	server := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			flog.Debugf("HTTP proxy server error: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		flog.Debugf("HTTP proxy shutdown with: %v", err)
	}
}
