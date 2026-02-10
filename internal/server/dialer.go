package server

import (
	"context"
	"net"
	"time"

	"github.com/txthinking/socks5"
)

type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

type directDialer struct {
	d *net.Dialer
}

func (d *directDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return d.d.DialContext(ctx, network, address)
}

func newDirectDialer() Dialer {
	return &directDialer{
		d: &net.Dialer{Timeout: 10 * time.Second},
	}
}

type socks5Dialer struct {
	client *socks5.Client
}

func (d *socks5Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	done := make(chan result, 1)
	go func() {
		conn, err := d.client.Dial(network, address)
		done <- result{conn, err}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			return nil, res.err
		}
		select {
		case <-ctx.Done():
			res.conn.Close()
			return nil, ctx.Err()
		default:
			return res.conn, nil
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func newSOCKS5Dialer(addr, username, password string) (Dialer, error) {
	client, err := socks5.NewClient(addr, username, password, 10, 10)
	if err != nil {
		return nil, err
	}
	return &socks5Dialer{client: client}, nil
}
