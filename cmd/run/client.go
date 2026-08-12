package run

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"paqet/internal/client"
	"paqet/internal/conf"
	"paqet/internal/forward"
	"paqet/internal/socks"
)

func startClient(cfg *conf.Conf) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client, err := client.New(cfg)
	if err != nil {
		log.Fatalf("failed to initialize client: %v", err)
	}
	if err := client.Start(ctx); err != nil {
		log.Fatalf("client encountered an error: %v", err)
	}

	for _, ss := range cfg.SOCKS5 {
		s, err := socks.New(client)
		if err != nil {
			log.Fatalf("failed to initialize SOCKS5: %v", err)
		}
		if err := s.Start(ctx, ss); err != nil {
			log.Fatalf("SOCKS5 encountered an error: %v", err)
		}
	}

	for _, ff := range cfg.Forward {
		f, err := forward.New(client, ff.Listen.String(), ff.Target)
		if err != nil {
			log.Fatalf("failed to initialize Forward: %v", err)
		}
		if err := f.Start(ctx, ff.Protocol); err != nil {
			log.Fatalf("forward encountered an error: %v", err)
		}
	}

	<-ctx.Done()
	log.Printf("shutdown signal received, shutting down...")
}
