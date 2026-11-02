package socket

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket/pcap"

	"paqet/internal/conf"
	"paqet/internal/metrics"
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

		n, addr, err := c.recvHandle.Read(data)
		if err != nil {
			if errors.Is(err, pcap.NextErrorTimeoutExpired) || errors.Is(err, errNoPayload) {
				continue
			}
			return 0, nil, err
		}

		// Intercept the fake 3-way handshake (see TriggerHandshake),
		// unless disabled in config. These magic payloads prime stateful
		// NAT/firewalls that drop non-SYN TCP, and must never reach KCP
		// (it would fail to decrypt) nor count as traffic. Peers without
		// handshake support never emit them, so old counterparts are
		// unaffected.
		if c.cfg.TCP.HandshakeEnabled() {
			if udpAddr, ok := addr.(*net.UDPAddr); ok {
				switch {
				case n == 9 && bytes.Equal(data[:n], []byte("PAQET_SYN")):
					// Server side: SYN received, reply SYN-ACK.
					_ = c.sendHandle.WriteWithFlags([]byte("PAQET_SYNACK"), udpAddr, conf.TCPF{SYN: true, ACK: true})
					continue
				case n == 12 && bytes.Equal(data[:n], []byte("PAQET_SYNACK")):
					// Client side: SYN-ACK received, reply ACK.
					_ = c.sendHandle.WriteWithFlags([]byte("PAQET_ACK"), udpAddr, conf.TCPF{ACK: true})
					continue
				case n == 9 && bytes.Equal(data[:n], []byte("PAQET_ACK")):
					// Server side: handshake complete.
					continue
				}
			}
		}

		if n > 0 {
			metrics.AddIngress(n, 1)
		}
		return n, addr, nil
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

	if len(data) > 0 {
		metrics.AddEgress(len(data), 1)
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

// TriggerHandshake sends a bare SYN to prime stateful NAT/firewalls
// (phone hotspot, CGNAT) that drop non-SYN TCP. The SYN creates the
// mapping; later PA packets for the same tuple pass. Replies are handled
// passively inside ReadFrom on both ends. Best-effort: caller proceeds
// with KCP even if the middlebox or server never answers.
func (c *PacketConn) TriggerHandshake(addr *net.UDPAddr) error {
	return c.sendHandle.WriteWithFlags([]byte("PAQET_SYN"), addr, conf.TCPF{SYN: true})
}

func (c *PacketConn) DeleteClientTCPF(addr net.Addr) {
	c.sendHandle.deleteClientTCPF(addr)
}
