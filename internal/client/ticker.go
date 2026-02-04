package client

import (
	"context"
	"time"
)

func (c *Client) ticker(ctx context.Context) {
	interval := time.Duration(c.cfg.Transport.KCP.PingSec) * time.Second
	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			for _, tc := range c.iter.Items {
				if tc.conn == nil {
					continue
				}
				_ = tc.conn.Ping(true)
			}
			timer.Reset(interval)
		case <-ctx.Done():
			return
		}
	}
}
