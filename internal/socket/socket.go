package socket

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket/pcap"

	"paqet/internal/conf"
)

type PacketConn struct {
	cfg           *conf.Network
	sendHandle    *SendHandle
	recvHandle    *RecvHandle
	readDeadline  atomic.Value
	writeDeadline atomic.Value
}

func New(cfg *conf.Network) (*PacketConn, error) {
	if cfg.Port == 0 {
		cfg.Port = 32768 + rand.Intn(32768)
	}

	sendHandle, err := NewSendHandle(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create send handle on %s: %v", cfg.Interface.Name, err)
	}

	recvHandle, err := NewRecvHandle(cfg)
	if err != nil {
		sendHandle.Close()
		return nil, fmt.Errorf("failed to create receive handle on %s: %v", cfg.Interface.Name, err)
	}

	conn := &PacketConn{
		cfg:        cfg,
		sendHandle: sendHandle,
		recvHandle: recvHandle,
	}

	return conn, nil
}

func (c *PacketConn) ReadFrom(data []byte) (n int, addr net.Addr, err error) {
	for {
		if d, ok := c.readDeadline.Load().(time.Time); ok && !d.IsZero() && !time.Now().Before(d) {
			return 0, nil, os.ErrDeadlineExceeded
		}

		p, addr, err := c.recvHandle.Read()
		if err != nil {
			if errors.Is(err, pcap.NextErrorTimeoutExpired) {
				continue
			}
			return 0, nil, err
		}

		return copy(data, p), addr, nil
	}
}

func (c *PacketConn) WriteTo(data []byte, addr net.Addr) (n int, err error) {
	if d, ok := c.writeDeadline.Load().(time.Time); ok && !d.IsZero() && !time.Now().Before(d) {
		return 0, os.ErrDeadlineExceeded
	}

	daddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return 0, net.InvalidAddrError("invalid address")
	}

	err = c.sendHandle.Write(data, daddr)
	if err != nil {
		return 0, err
	}

	return len(data), nil
}

func (c *PacketConn) Close() error {
	if c.sendHandle != nil {
		c.sendHandle.Close()
	}
	if c.recvHandle != nil {
		c.recvHandle.Close()
	}
	return nil
}

func (c *PacketConn) LocalAddr() net.Addr {
	return nil
	// return &net.UDPAddr{
	// 	IP:   append([]byte(nil), c.cfg.PrimaryAddr().IP...),
	// 	Port: c.cfg.PrimaryAddr().Port,
	// 	Zone: c.cfg.PrimaryAddr().Zone,
	// }
}

func (c *PacketConn) SetDeadline(t time.Time) error {
	c.readDeadline.Store(t)
	c.writeDeadline.Store(t)
	return nil
}

func (c *PacketConn) SetReadDeadline(t time.Time) error {
	c.readDeadline.Store(t)
	return nil
}

func (c *PacketConn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline.Store(t)
	return nil
}

func (c *PacketConn) SetDSCP(dscp int) error {
	return nil
}

func (c *PacketConn) SetClientTCPF(addr net.Addr, f []conf.TCPF) {
	c.sendHandle.setClientTCPF(addr, f)
}

func (c *PacketConn) DeleteClientTCPF(addr net.Addr) {
	c.sendHandle.deleteClientTCPF(addr)
}
