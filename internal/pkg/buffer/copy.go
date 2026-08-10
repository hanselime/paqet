package buffer

import (
	"errors"
	"io"
)

const (
	TCPSize = 8 * 1024
	UDPSize = 4 * 1024
	_       = uint(0xFFFF - UDPSize)
)

func CopyT(dst io.Writer, src io.Reader) error {
	buf := make([]byte, TCPSize)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			w, werr := dst.Write(buf[:n])
			if werr != nil {
				return werr
			}
			if w < n {
				return io.ErrShortWrite
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func CopyU(dst io.Writer, src io.Reader) error {
	buf := make([]byte, UDPSize+1)
	for {
		n, err := src.Read(buf)
		switch {
		case n > UDPSize:
		case n > 0:
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}
