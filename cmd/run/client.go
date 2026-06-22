package run

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"paqet/internal/client"
	"paqet/internal/conf"
	"paqet/internal/flog"
	"paqet/internal/forward"
	"paqet/internal/socks"
)

func startClient(cfg *conf.Conf) {
	flog.Infof("Starting client...")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client, err := client.New(cfg)
	if err != nil {
		flog.Fatalf("Failed to initialize client: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		flog.Fatalf("Client encountered an error: %v", err)
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
		f, err := forward.New(client, ff.Listen.String(), ff.Target)
		if err != nil {
			flog.Fatalf("Failed to initialize Forward: %v", err)
		}
		if err := f.Start(ctx, ff.Protocol); err != nil {
			flog.Fatalf("Forward encountered an error: %v", err)
		}
	}

	<-ctx.Done()
	fmt.Println("Shutdown signal received, shutting down...")
}
