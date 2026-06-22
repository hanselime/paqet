package run

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"paqet/internal/conf"
	"paqet/internal/flog"
	"paqet/internal/server"
)

func startServer(cfg *conf.Conf) {
	flog.Infof("Starting server...")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server, err := server.New(cfg)
	if err != nil {
		flog.Fatalf("Failed to initialize server: %v", err)
	}
	if err := server.Start(ctx); err != nil {
		flog.Fatalf("Server encountered an error: %v", err)
	}

	<-ctx.Done()
	fmt.Println("Shutdown signal received, shutting down...")
}
