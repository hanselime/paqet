package quic

import (
	"context"
	"crypto/tls"
	"net"
	"paqet/internal/conf"
	"paqet/internal/socket"
	"paqet/internal/tnet"

	"github.com/quic-go/quic-go"
)

type Listener struct {
	packetConn *socket.PacketConn
	cfg        *conf.QUIC
	listener   *quic.Listener
	tlsConfig  *tls.Config
}

func Listen(cfg *conf.QUIC, pConn *socket.PacketConn) (tnet.Listener, error) {
	// Generate TLS config for server
	tlsConfig, err := cfg.GenerateTLSConfig("server")
	if err != nil {
		return nil, err
	}
	
	// Create QUIC config
	quicConfig := getQUICConfig(cfg)
	
	// Create QUIC listener using the packet connection
	listener, err := quic.Listen(pConn, tlsConfig, quicConfig)
	if err != nil {
		return nil, err
	}
	
	return &Listener{
		packetConn: pConn,
		cfg:        cfg,
		listener:   listener,
		tlsConfig:  tlsConfig,
	}, nil
}

func (l *Listener) Accept() (tnet.Conn, error) {
	qconn, err := l.listener.Accept(context.Background())
	if err != nil {
		return nil, err
	}
	
	return newConn(qconn), nil
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
