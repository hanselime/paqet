package socket

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"paqet/internal/conf"
	"sync/atomic"
	"time"
)

type PacketConn struct {
	cfg           *conf.Network
	sendHandle    *SendHandle
	recvHandle    *RecvHandle
	readDeadline  atomic.Value
	writeDeadline atomic.Value

	ctx    context.Context
	cancel context.CancelFunc
}

// &OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: err}
func New(ctx context.Context, cfg *conf.Network) (*PacketConn, error) {
	if cfg.Port == 0 {
		cfg.Port = 32768 + rand.Intn(32768)
	}

	sendHandle, err := NewSendHandle(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create send handle on %s: %v", cfg.Interface.Name, err)
	}

	recvHandle, err := NewRecvHandle(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create receive handle on %s: %v", cfg.Interface.Name, err)
	}

	ctx, cancel := context.WithCancel(ctx)
	conn := &PacketConn{
		cfg:        cfg,
		sendHandle: sendHandle,
		recvHandle: recvHandle,
		ctx:        ctx,
		cancel:     cancel,
	}

	return conn, nil
}

func (c *PacketConn) ReadFrom(data []byte) (n int, addr net.Addr, err error) {
	var timer *time.Timer
	var deadline <-chan time.Time
	if d, ok := c.readDeadline.Load().(time.Time); ok && !d.IsZero() {
		timer = time.NewTimer(time.Until(d))
		defer timer.Stop()
		deadline = timer.C
	}

	select {
	case <-c.ctx.Done():
		return 0, nil, c.ctx.Err()
	case <-deadline:
		return 0, nil, os.ErrDeadlineExceeded
	default:
	}

	payload, addr, err := c.recvHandle.Read()
	if err != nil {
		return 0, nil, err
	}
	n = copy(data, payload)

	return n, addr, nil
}

func (c *PacketConn) WriteTo(data []byte, addr net.Addr) (n int, err error) {
	var timer *time.Timer
	var deadline <-chan time.Time
	if d, ok := c.writeDeadline.Load().(time.Time); ok && !d.IsZero() {
		timer = time.NewTimer(time.Until(d))
		defer timer.Stop()
		deadline = timer.C
	}

	select {
	case <-c.ctx.Done():
		return 0, c.ctx.Err()
	case <-deadline:
		return 0, os.ErrDeadlineExceeded
	default:
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
	c.cancel()

	if c.sendHandle != nil {
		go c.sendHandle.Close()
	}
	if c.recvHandle != nil {
		go c.recvHandle.Close()
	}

	return nil
}

func (c *PacketConn) LocalAddr() net.Addr {
	// Return IPv4 address if configured, otherwise IPv6
	if c.cfg != nil {
		if c.cfg.IPv4.Addr != nil {
			return &net.UDPAddr{
				IP:   append([]byte(nil), c.cfg.IPv4.Addr.IP...),
				Port: c.cfg.IPv4.Addr.Port,
				Zone: c.cfg.IPv4.Addr.Zone,
			}
		}
		if c.cfg.IPv6.Addr != nil {
			return &net.UDPAddr{
				IP:   append([]byte(nil), c.cfg.IPv6.Addr.IP...),
				Port: c.cfg.IPv6.Addr.Port,
				Zone: c.cfg.IPv6.Addr.Zone,
			}
		}
		// Fallback: return address with port from config
		return &net.UDPAddr{
			IP:   net.IPv4zero,
			Port: c.cfg.Port,
		}
	}
	// If cfg is nil, return a default address
	return &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: 0,
	}
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

// SetReadBuffer implements the buffer size setter for compatibility with quic-go.
// Since PacketConn uses pcap instead of UDP sockets, this is a no-op.
func (c *PacketConn) SetReadBuffer(size int) error {
	// PacketConn uses pcap handles which have their own buffer management
	// via PCAP.Sockbuf configuration. We return nil to indicate success
	// to quic-go without actually modifying any buffer.
	return nil
}

// SetWriteBuffer implements the buffer size setter for compatibility with quic-go.
// Since PacketConn uses pcap instead of UDP sockets, this is a no-op.
func (c *PacketConn) SetWriteBuffer(size int) error {
	// PacketConn uses pcap handles which have their own buffer management
	// via PCAP.Sockbuf configuration. We return nil to indicate success
	// to quic-go without actually modifying any buffer.
	return nil
}
