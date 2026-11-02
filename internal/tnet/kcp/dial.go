package kcp

import (
	"fmt"
	"net"
	"time"

	"paqet/internal/conf"
	"paqet/internal/flog"
	"paqet/internal/metrics"
	"paqet/internal/socket"
	"paqet/internal/tnet"

	"github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/smux"
)

func Dial(addr *net.UDPAddr, cfg *conf.KCP, netCfg conf.Network) (tnet.Conn, error) {
	nCfg := netCfg
	packetConn, err := socket.New(&nCfg)
	if err != nil {
		return nil, fmt.Errorf("kcp: failed to create packetconn: %w", err)
	}

	// SYN-prime the path before KCP data, unless disabled in config.
	// Stateful middleboxes drop non-SYN TCP, so the first packet must
	// be SYN. Replies complete lazily inside ReadFrom. Default is
	// enabled; pre-existing configs without the key are unaffected.
	if netCfg.TCP.HandshakeEnabled() {
		if err := packetConn.TriggerHandshake(addr); err != nil {
			packetConn.Close()
			return nil, fmt.Errorf("kcp: failed to trigger handshake: %w", err)
		}
		// Give the SYN a fraction of a second to complete round-trips
		// before KCP starts blasting data.
		time.Sleep(50 * time.Millisecond)
	}

	conn, err := kcp.NewConn(addr.String(), cfg.Block, cfg.Dshard, cfg.Pshard, packetConn)
	if err != nil {
		packetConn.Close()
		return nil, fmt.Errorf("kcp: failed to dial connection: %w", err)
	}
	if conn == nil {
		packetConn.Close()
		return nil, fmt.Errorf("connection attempt failed: nil KCP session")
	}
	aplConf(conn, cfg)

	flog.Debugf("KCP connection established, creating smux session")

	sess, err := smux.Client(conn, smuxConf(cfg))
	if err != nil {
		conn.Close()
		packetConn.Close()
		return nil, fmt.Errorf("kcp: failed to create smux session: %w", err)
	}

	flog.Debugf("smux session established successfully")
	metrics.IncConnections()
	metricsID := metrics.RegisterConnection(
		addrString(conn.RemoteAddr()),
		addrString(conn.LocalAddr()),
	)
	return &Conn{
		PacketConn: packetConn,
		UDPSession: conn,
		Session:    sess,
		counted:    true,
		metricsID:  metricsID,
	}, nil
}
