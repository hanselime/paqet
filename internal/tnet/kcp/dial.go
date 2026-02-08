package kcp

import (
	"fmt"
	"net"
	"paqet/internal/conf"
	"paqet/internal/flog"
	"paqet/internal/socket"
	"paqet/internal/tnet"

	"github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/smux"
)

func Dial(addr *net.UDPAddr, cfg *conf.KCP, pConn *socket.PacketConn) (tnet.Conn, error) {
	conn, err := kcp.NewConn(addr.String(), cfg.Block, cfg.Dshard, cfg.Pshard, pConn)
	if err != nil {
		return nil, fmt.Errorf("connection attempt failed: %v", err)
	}
	aplConf(conn, cfg)
	flog.Debugf("KCP connection created, creating smux session")

	sess, err := smux.Client(conn, smuxConf(cfg))
	if err != nil {
		return nil, fmt.Errorf("failed to create smux session: %w", err)
	}

	flog.Debugf("smux session created successfully")
	return &Conn{pConn, conn, sess}, nil
}
