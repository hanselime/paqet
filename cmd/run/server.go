package run

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"paqet/internal/conf"
	"paqet/internal/server"
)

func startServer(cfg *conf.Conf) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server, err := server.New(cfg)
	if err != nil {
		log.Fatalf("failed to initialize server: %v", err)
	}
	if err := server.Start(ctx); err != nil {
		log.Fatalf("server encountered an error: %v", err)
	}

	<-ctx.Done()
	log.Printf("shutdown signal received, shutting down...")
}
