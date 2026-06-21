package kcp

import (
	"context"
	"net"

	"github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/smux"

	"paqet/internal/conf"
	"paqet/internal/socket"
	"paqet/internal/tnet"
)

type Listener struct {
	PacketConn *socket.PacketConn
	cfg        *conf.KCP
	listener   *kcp.Listener
}

func Listen(ctx context.Context, cfg *conf.KCP, netCfg conf.Network) (tnet.Listener, error) {
	nCfg := netCfg
	packetConn, err := socket.New(ctx, &nCfg)
	if err != nil {
		return nil, err
	}

	l, err := kcp.ServeConn(cfg.Block, cfg.Dshard, cfg.Pshard, packetConn)
	if err != nil {
		packetConn.Close()
		return nil, err
	}

	return &Listener{PacketConn: packetConn, cfg: cfg, listener: l}, nil
}

func (l *Listener) Accept() (tnet.Conn, error) {
	conn, err := l.listener.AcceptKCP()
	if err != nil {
		return nil, err
	}
	aplConf(conn, l.cfg)
	sess, err := smux.Server(conn, smuxConf(l.cfg))
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &Conn{nil, conn, sess}, nil
}

func (l *Listener) Close() error {
	var err error
	if l.listener != nil {
		if e := l.listener.Close(); e != nil {
			err = e
		}
	}
	if l.PacketConn != nil {
		if e := l.PacketConn.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

func (l *Listener) Addr() net.Addr {
	return l.listener.Addr()
}

func (l *Listener) SetClientTCPF(addr net.Addr, f []conf.TCPF) {
	l.PacketConn.SetClientTCPF(addr, f)
}

func (l *Listener) DeleteClientTCPF(addr net.Addr) {
	l.PacketConn.DeleteClientTCPF(addr)
}
