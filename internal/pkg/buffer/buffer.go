package buffer

import (
	"sync"
)

// tPool: 64KB buffers for TCP streams
var tPool = sync.Pool{
	New: func() any {
		b := make([]byte, 64*1024)
		return &b
	},
}

// uPool: 16KB buffers for UDP packets (covers jumbo frames + headers)
var uPool = sync.Pool{
	New: func() any {
		b := make([]byte, 16*1024)
		return &b
	},
}

// returns a UDP buffer pointer from the pool
func GetU() *[]byte {
	return uPool.Get().(*[]byte)
}

// returns a UDP buffer pointer to the pool
func PutU(buf *[]byte) {
	uPool.Put(buf)
}
