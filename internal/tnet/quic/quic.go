package quic

import (
	"context"
	"paqet/internal/conf"
	"time"

	"github.com/quic-go/quic-go"
)

// getQUICConfig creates a QUIC configuration from our config
func getQUICConfig(cfg *conf.QUIC) *quic.Config {
	config := &quic.Config{
		MaxIdleTimeout:                 time.Duration(cfg.MaxIdleTimeout) * time.Second,
		MaxIncomingStreams:             int64(cfg.MaxIncomingStreams),
		MaxIncomingUniStreams:          int64(cfg.MaxIncomingUniStreams),
		InitialStreamReceiveWindow:     uint64(cfg.InitialStreamReceiveWindow),
		MaxStreamReceiveWindow:         uint64(cfg.MaxStreamReceiveWindow),
		InitialConnectionReceiveWindow: uint64(cfg.InitialConnectionReceiveWindow),
		MaxConnectionReceiveWindow:     uint64(cfg.MaxConnectionReceiveWindow),
		KeepAlivePeriod:                time.Duration(cfg.KeepAlivePeriod) * time.Second,
		EnableDatagrams:                cfg.EnableDatagrams,
		Allow0RTT:                      cfg.Enable0RTT,
	}
	
	return config
}
