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

	for _, srvCfg := range cfg.Servers {
		// Create a sub-configuration for this specific server connection
		subCfg := *cfg
		subCfg.Server = srvCfg.Server
		subCfg.SOCKS5 = srvCfg.SOCKS5
		subCfg.Forward = srvCfg.Forward
		subCfg.Transport = srvCfg.Transport

		client, err := client.New(&subCfg)
		if err != nil {
			flog.Fatalf("Failed to initialize client for %s: %v", srvCfg.Server.Addr_, err)
		}
		if err := client.Start(ctx); err != nil {
			flog.Infof("Client for %s encountered an error: %v", srvCfg.Server.Addr_, err)
		}

		for _, ss := range subCfg.SOCKS5 {
			s, err := socks.New(client)
			if err != nil {
				flog.Fatalf("Failed to initialize SOCKS5: %v", err)
			}
			if err := s.Start(ctx, ss); err != nil {
				flog.Fatalf("SOCKS5 encountered an error: %v", err)
			}
		}
		for _, ff := range subCfg.Forward {
			f, err := forward.New(client, ff.Listen.String(), ff.Target.String())
			if err != nil {
				flog.Fatalf("Failed to initialize Forward: %v", err)
			}
			if err := f.Start(ctx, ff.Protocol); err != nil {
				flog.Infof("Forward encountered an error: %v", err)
			}
		}
	}

	<-ctx.Done()
}
