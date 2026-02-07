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
	"sync"
	"syscall"
)

func startClient(cfg *conf.Conf) {
	flog.Infof("Starting client...")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup

	for _, srvCfg := range cfg.Servers {
		wg.Add(1)
		go func(srvCfg conf.ServerConfig) {
			defer wg.Done()
			runInstance(ctx, cfg, srvCfg)
		}(srvCfg)
	}

	<-sig
	flog.Infof("Shutdown signal received, initiating graceful shutdown...")
	cancel()
	wg.Wait()
	flog.Infof("Shutdown complete.")
}

func runInstance(ctx context.Context, base *conf.Conf, srv conf.ServerConfig) {
	sub := *base
	sub.Server = srv.Server
	sub.SOCKS5 = srv.SOCKS5
	sub.Forward = srv.Forward
	sub.Transport = srv.Transport

	c, err := client.New(&sub)
	if err != nil {
		flog.Errorf("Client init failed for %s: %v", srv.Server.Addr, err)
		return
	}
	if err := c.Start(ctx); err != nil {
		flog.Infof("Client start error %s: %v", srv.Server.Addr, err)
	}

	for _, ss := range sub.SOCKS5 {
		go func(ss conf.SOCKS5) {
			s, _ := socks.New(c)
			if err := s.Start(ctx, ss); err != nil {
				flog.Errorf("SOCKS5 error %v: %v", ss.Listen, err)
			}
		}(ss)
	}
	for _, ff := range sub.Forward {
		go func(ff conf.Forward) {
			f, _ := forward.New(c, ff.Listen.String(), ff.Target.String())
			if err := f.Start(ctx, ff.Protocol); err != nil {
				flog.Errorf("Forward error %v: %v", ff.Listen, err)
			}
		}(ff)
	}
	<-ctx.Done()
}
