package buffer

import (
	"io"
)

func CopyT(dst io.Writer, src io.Reader) error {
	bufp := TPool.Get().(*[]byte)
	defer TPool.Put(bufp)
	buf := *bufp

	_, err := io.CopyBuffer(dst, src, buf)
	return err
}
