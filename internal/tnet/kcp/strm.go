package kcp

import (
	"paqet/internal/metrics"
	"sync"

	"github.com/xtaci/smux"
)

type Strm struct {
	*smux.Stream
	once    sync.Once
	counted bool
	connID  uint64
}

func (s *Strm) SID() int {
	return int(s.ID())
}

func newStrm(stream *smux.Stream, connID uint64) *Strm {
	metrics.IncStreams()
	if connID != 0 {
		metrics.IncConnStreams(connID)
	}
	return &Strm{Stream: stream, counted: true, connID: connID}
}

func (s *Strm) Close() error {
	s.once.Do(func() {
		if s.counted {
			metrics.DecStreams()
			if s.connID != 0 {
				metrics.DecConnStreams(s.connID)
			}
		}
	})
	return s.Stream.Close()
}

func (s *Strm) Read(p []byte) (int, error) {
	n, err := s.Stream.Read(p)
	if n > 0 && s.connID != 0 {
		metrics.AddConnIngress(s.connID, n)
	}
	return n, err
}

func (s *Strm) Write(p []byte) (int, error) {
	n, err := s.Stream.Write(p)
	if n > 0 && s.connID != 0 {
		metrics.AddConnEgress(s.connID, n)
	}
	return n, err
}
