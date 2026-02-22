package buffer

import (
	"io"
)

func CopyT(dst io.Writer, src io.Reader) error {
	buf := make([]byte, TPool)
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if ew != nil {
				return ew
			}
			if nr != nw {
				return io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				return nil
			}
			return er
		}
		if nr == 0 {
			return nil
		}
	}
}
