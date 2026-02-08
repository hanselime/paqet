package run

import (
	"context"
	"os"
	"os/signal"
	"paqet/internal/client"
	"paqet/internal/conf"
	"paqet/internal/flog"
	"paqet/internal/forward"
	"paqet/internal/socks"
	"syscall"
)

func startClient(cfg *conf.Conf) {
	flog.Infof("Starting client...")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		flog.Infof("Shutdown signal received, initiating graceful shutdown...")
		cancel()
	}()

	client, err := client.New(cfg)
	if err != nil {
		flog.Fatalf("Failed to initialize client: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		flog.Infof("Client encountered an error: %v", err)
	}

	for _, ss := range cfg.SOCKS5 {
		s, err := socks.New(client)
		if err != nil {
			flog.Fatalf("Failed to initialize SOCKS5: %v", err)
		}
		if err := s.Start(ctx, ss); err != nil {
			flog.Fatalf("SOCKS5 encountered an error: %v", err)
		}
	}
	for _, ff := range cfg.Forward {
		f, err := forward.New(client, ff.Listen.String(), ff.Target.String(), cfg)
		if err != nil {
			flog.Fatalf("Failed to initialize Forward: %v", err)
		}
		if err := f.Start(ctx, ff.Protocol); err != nil {
			flog.Infof("Forward encountered an error: %v", err)
		}
	}

	<-ctx.Done()
}
