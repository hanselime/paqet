package buffer

import (
	"io"
)

// copies data from src to dst using a pooled buffer
func Copy(dst io.Writer, src io.Reader) error {
	bufp := tPool.Get().(*[]byte)
	defer tPool.Put(bufp)
	buf := *bufp

	_, err := io.CopyBuffer(dst, src, buf)
	return err
}
