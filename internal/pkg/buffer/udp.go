package buffer

import (
	"io"
)

func CopyU(dst io.Writer, src io.Reader) error {
	buf := make([]byte, UPool)

	_, err := io.CopyBuffer(dst, src, buf)
	return err
}
