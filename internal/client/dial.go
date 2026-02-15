package client

import (
	"fmt"
	"paqet/internal/flog"
	"paqet/internal/tnet"
	"time"
)

func (c *Client) newConn() (tnet.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	autoExpire := 300
	tc := c.iter.Next()
	go tc.sendTCPF(tc.conn)
	err := tc.conn.Ping(false)
	if err != nil {
		flog.Infof("connection lost, retrying....")
		if tc.conn != nil {
			tc.conn.Close()
		}
		newConn, err := tc.createConn()
		if err != nil {
			return nil, fmt.Errorf("failed to recreate connection: %w", err)
		}
		tc.conn = newConn
		tc.expire = time.Now().Add(time.Duration(autoExpire) * time.Second)
	}
	return tc.conn, nil
}

func (c *Client) newStrm() (tnet.Strm, error) {
	conn, err := c.newConn()
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}
	strm, err := conn.OpenStrm()
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}
	return strm, nil
}
