package kcp

import (
	"net"
	"paqet/internal/conf"
	"paqet/internal/socket"
	"paqet/internal/tnet"

	"github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/smux"
)

type Listener struct {
	packetConn *socket.PacketConn
	cfg        *conf.KCP
	listener   *kcp.Listener
}

func Listen(cfg *conf.KCP, pConn *socket.PacketConn) (tnet.Listener, error) {
	l, err := kcp.ServeConn(cfg.Block, cfg.Dshard, cfg.Pshard, pConn)
	if err != nil {
		return nil, err
	}

	return &Listener{packetConn: pConn, cfg: cfg, listener: l}, nil
}

func (l *Listener) Accept() (tnet.Conn, error) {
	conn, err := l.listener.AcceptKCP()
	if err != nil {
		return nil, err
	}
	aplConf(conn, l.cfg)
	sess, err := smux.Server(conn, smuxConf(l.cfg))
	if err != nil {
		return nil, err
	}
	return &Conn{nil, conn, sess}, nil
}

func (l *Listener) Close() error {
	if l.listener != nil {
		l.listener.Close()
	}
	if l.packetConn != nil {
		l.packetConn.Close()
	}
	return nil
}

func (l *Listener) Addr() net.Addr {
	return l.listener.Addr()
}
