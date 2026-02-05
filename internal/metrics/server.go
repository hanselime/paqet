package metrics

import (
	"context"
	"net"
	"net/http"
	"paqet/internal/conf"
	"paqet/internal/flog"
)

func Start(ctx context.Context, cfg conf.Metrics) {
	if !cfg.Enabled {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", Handler)

	listener, err := net.Listen("tcp", cfg.Listen_)
	if err != nil {
		flog.Errorf("metrics listener failed on %s: %v", cfg.Listen_, err)
		return
	}

	server := &http.Server{Handler: mux}

	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			flog.Errorf("metrics server error: %v", err)
		}
	}()

	flog.Infof("metrics enabled on %s", cfg.Listen_)
}
