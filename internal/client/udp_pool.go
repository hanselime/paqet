package client

import (
	"paqet/internal/flog"
	"paqet/internal/tnet"
	"sync"
)

type udpPool struct {
	strms map[uint64]tnet.Strm
	mu    sync.RWMutex
}

func (p *udpPool) delete(key uint64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if strm, exists := p.strms[key]; exists {
		flog.Debugf("closing UDP session stream %d", strm.SID())
		strm.Close()
	} else {
		flog.Debugf("UDP session key %d not found for close", key)
	}
	delete(p.strms, key)

	return nil
}

func (p *udpPool) clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, strm := range p.strms {
		strm.Close()
		delete(p.strms, key)
	}
}
