package kcp

import (
	"fmt"
	"net"
	"time"

	"github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/smux"

	"paqet/internal/protocol"
	"paqet/internal/socket"
	"paqet/internal/tnet"
)

type Conn struct {
	PacketConn *socket.PacketConn
	UDPSession *kcp.UDPSession
	Session    *smux.Session
}

func (c *Conn) OpenStrm() (tnet.Strm, error) {
	strm, err := c.Session.OpenStream()
	if err != nil {
		return nil, err
	}
	return &Strm{strm}, nil
}

func (c *Conn) AcceptStrm() (tnet.Strm, error) {
	strm, err := c.Session.AcceptStream()
	if err != nil {
		return nil, err
	}
	return &Strm{strm}, nil
}

func (c *Conn) Ping(wait bool) error {
	strm, err := c.Session.OpenStream()
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	defer strm.Close()
	if wait {
		p := protocol.Proto{Type: protocol.PPING}
		err = p.Write(strm)
		if err != nil {
			return fmt.Errorf("strm ping write failed: %w", err)
		}
		err = p.Read(strm)
		if err != nil {
			return fmt.Errorf("strm ping read failed: %w", err)
		}
		if p.Type != protocol.PPONG {
			return fmt.Errorf("strm pong failed: unexpected type %d", p.Type)
		}
	}
	return nil
}

func (c *Conn) Close() error {
	var err error
	if c.Session != nil {
		if e := c.Session.Close(); e != nil {
			err = e
		}
	}
	if c.UDPSession != nil {
		if e := c.UDPSession.Close(); e != nil && err == nil {
			err = e
		}
	}
	if c.PacketConn != nil {
		if e := c.PacketConn.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

func (c *Conn) LocalAddr() net.Addr                { return c.Session.LocalAddr() }
func (c *Conn) RemoteAddr() net.Addr               { return c.Session.RemoteAddr() }
func (c *Conn) SetDeadline(t time.Time) error      { return c.UDPSession.SetDeadline(t) }
func (c *Conn) SetReadDeadline(t time.Time) error  { return c.UDPSession.SetReadDeadline(t) }
func (c *Conn) SetWriteDeadline(t time.Time) error { return c.UDPSession.SetWriteDeadline(t) }
