package buffer

import (
	"sync"
)

var (
	TPool sync.Pool
	UPool sync.Pool
)

func Initialize(tPool, uPool int) {
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
