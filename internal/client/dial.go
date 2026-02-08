package client

import (
	"fmt"
	"math"
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
		if c, err := tc.createConn(); err == nil {
			tc.conn = c
		}
		tc.expire = time.Now().Add(time.Duration(autoExpire) * time.Second)
	}
	return tc.conn, nil
}

func (c *Client) newStrm() (tnet.Strm, error) {
	return c.newStrmWithRetry(0)
}

func (c *Client) newStrmWithRetry(attempt int) (tnet.Strm, error) {
	maxAttempts := c.cfg.Performance.MaxRetryAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	
	if attempt >= maxAttempts {
		return nil, fmt.Errorf("failed to create stream after %d attempts", attempt)
	}
	
	conn, err := c.newConn()
	if err != nil {
		flog.Debugf("session creation failed (attempt %d/%d), retrying after backoff", attempt+1, maxAttempts)
		backoff := c.calculateRetryBackoff(attempt)
		time.Sleep(backoff)
		return c.newStrmWithRetry(attempt + 1)
	}
	
	strm, err := conn.OpenStrm()
	if err != nil {
		flog.Debugf("failed to open stream (attempt %d/%d), retrying: %v", attempt+1, maxAttempts, err)
		backoff := c.calculateRetryBackoff(attempt)
		time.Sleep(backoff)
		return c.newStrmWithRetry(attempt + 1)
	}
	
	return strm, nil
}

func (c *Client) calculateRetryBackoff(attempt int) time.Duration {
	initialBackoff := c.cfg.Performance.RetryInitialBackoffMs
	maxBackoff := c.cfg.Performance.RetryMaxBackoffMs
	
	if initialBackoff <= 0 {
		initialBackoff = 100
	}
	if maxBackoff <= 0 {
		maxBackoff = 10000
	}
	
	// Exponential backoff: initialBackoff * 2^attempt
	backoffMs := float64(initialBackoff) * math.Pow(2, float64(attempt))
	if backoffMs > float64(maxBackoff) {
		backoffMs = float64(maxBackoff)
	}
	
	return time.Duration(backoffMs) * time.Millisecond
}
