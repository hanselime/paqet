package client

import (
	"sync"

	"paqet/internal/flog"
	"paqet/internal/tnet"
)

type udpPool struct {
	strms map[uint64]tnet.Strm
	mu    sync.RWMutex
}

func (p *udpPool) delete(key uint64, strm tnet.Strm) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	s, ok := p.strms[key]
	if !ok || s != strm {
		return nil
	}

	flog.Debugf("closing UDP session stream %d", strm.SID())
	s.Close()
	delete(p.strms, key)

	return nil
}
