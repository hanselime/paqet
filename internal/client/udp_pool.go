package client

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"paqet/internal/flog"
	"paqet/internal/tnet"
)

const (
	udpIdle  = 60 * time.Second
	udpSweep = 1 * time.Second
)

type udpStrm struct {
	strm tnet.Strm
	last atomic.Int64
}

type udpPool struct {
	strms map[uint64]*udpStrm
	now   atomic.Int64
	mu    sync.RWMutex
}

func newUDPPool() *udpPool {
	p := &udpPool{strms: make(map[uint64]*udpStrm)}
	p.now.Store(time.Now().UnixNano())
	return p
}

func (p *udpPool) mark(u *udpStrm) {
	u.last.Store(p.now.Load())
}

func (p *udpPool) add(key uint64, strm tnet.Strm) {
	u := &udpStrm{strm: strm}
	p.mark(u)
	p.strms[key] = u
}

func (p *udpPool) touch(key uint64) {
	p.mu.RLock()
	if u, ok := p.strms[key]; ok {
		p.mark(u)
	}
	p.mu.RUnlock()
}

func (p *udpPool) ticker(ctx context.Context) {
	t := time.NewTicker(udpSweep)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			p.now.Store(time.Now().UnixNano())
			p.sweep()
		case <-ctx.Done():
			return
		}
	}
}

func (p *udpPool) sweep() {
	cutoff := p.now.Load() - int64(udpIdle)

	var dead []*udpStrm
	p.mu.Lock()
	for key, u := range p.strms {
		if u.last.Load() <= cutoff {
			delete(p.strms, key)
			dead = append(dead, u)
		}
	}
	p.mu.Unlock()

	for _, u := range dead {
		flog.Debugf("closing idle UDP stream %d", u.strm.SID())
		u.strm.Close()
	}
}

func (p *udpPool) delete(key uint64, strm tnet.Strm) error {
	p.mu.Lock()
	u, ok := p.strms[key]
	if !ok || u.strm != strm {
		p.mu.Unlock()
		return nil
	}
	delete(p.strms, key)
	p.mu.Unlock()

	flog.Debugf("closing UDP session stream %d", strm.SID())
	u.strm.Close()

	return nil
}
