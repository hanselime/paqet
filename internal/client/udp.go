package client

import (
	"context"

	"paqet/internal/flog"
	"paqet/internal/pkg/hash"
	"paqet/internal/protocol"
	"paqet/internal/tnet"
)

func (c *Client) UDP(ctx context.Context, lAddr, tAddr string) (tnet.Strm, bool, uint64, error) {
	key := hash.AddrPair(lAddr, tAddr)
	c.udpPool.mu.RLock()
	if u, ok := c.udpPool.strms[key]; ok {
		c.udpPool.mark(u)
		c.udpPool.mu.RUnlock()
		flog.Debugf("reusing UDP stream %d for %s -> %s", u.strm.SID(), lAddr, tAddr)
		return u.strm, false, key, nil
	}
	c.udpPool.mu.RUnlock()

	strm, err := c.newStrm(ctx)
	if err != nil {
		flog.Debugf("failed to create stream for UDP %s -> %s: %v", lAddr, tAddr, err)
		return nil, false, 0, err
	}

	taddr, err := tnet.NewAddr(tAddr)
	if err != nil {
		flog.Debugf("invalid UDP address %s: %v", tAddr, err)
		strm.Close()
		return nil, false, 0, err
	}

	stop := context.AfterFunc(ctx, func() { strm.Close() })
	p := protocol.Proto{Type: protocol.PUDP, Addr: taddr}
	err = p.Write(strm)
	if err != nil {
		stop()
		flog.Debugf("failed to write UDP protocol for %s -> %s on stream %d: %v", lAddr, tAddr, strm.SID(), err)
		strm.Close()
		return nil, false, 0, err
	}
	stop()

	c.udpPool.mu.Lock()
	if u, ok := c.udpPool.strms[key]; ok {
		c.udpPool.mark(u)
		c.udpPool.mu.Unlock()
		strm.Close()
		flog.Debugf("discarding duplicate UDP stream %d, reusing %d", strm.SID(), u.strm.SID())
		return u.strm, false, key, nil
	}
	c.udpPool.add(key, strm)
	c.udpPool.mu.Unlock()

	flog.Debugf("UDP stream %d created for %s -> %s", strm.SID(), lAddr, tAddr)
	return strm, true, key, nil
}

func (c *Client) CloseUDP(key uint64, strm tnet.Strm) error {
	return c.udpPool.delete(key, strm)
}

func (c *Client) Touch(key uint64) {
	c.udpPool.touch(key)
}
