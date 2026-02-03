package buffer

import (
	"paqet/internal/flog"
	"sync"
)

var (
	TPool sync.Pool
	UPool sync.Pool
)

func Initialize(tPool, uPool int) {
	flog.Warnf("tcpbuf: %d, udpbuf: %d", tPool, uPool)
	TPool = sync.Pool{
		New: func() any {
			b := make([]byte, tPool)
			return &b
		},
	}
	UPool = sync.Pool{
		New: func() any {
			b := make([]byte, uPool)
			return &b
		},
	}
}
