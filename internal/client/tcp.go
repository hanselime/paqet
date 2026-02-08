package client

import (
	"paqet/internal/flog"
	"paqet/internal/protocol"
	"paqet/internal/tnet"
)

func (c *Client) TCP(addr string) (tnet.Strm, error) {
	strm, err := c.newStrm()
	if err != nil {
		flog.Debugf("failed to create stream for TCP %s: %v", addr, err)
		return nil, err
	}

	tAddr, err := tnet.NewAddr(addr)
	if err != nil {
		flog.Debugf("invalid TCP address %s: %v", addr, err)
		strm.Close()
		return nil, err
	}

	p := protocol.Proto{Type: protocol.PTCP, Addr: tAddr}
	err = p.Write(strm)
	if err != nil {
		flog.Debugf("failed to write TCP protocol header for %s on stream %d: %v", addr, strm.SID(), err)
		strm.Close()
		return nil, err
	}

	flog.Debugf("TCP stream %d created for %s", strm.SID(), addr)
	return strm, nil
}
