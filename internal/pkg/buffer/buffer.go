package buffer

import (
	"sync"
)

var TPool = sync.Pool{
	New: func() any {
		b := make([]byte, 64*1024)
		return &b
	},
}

var UPool = sync.Pool{
	New: func() any {
		b := make([]byte, 32*1024)
		return &b
	},
}
