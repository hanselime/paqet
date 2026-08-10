package frame

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	hdrLen          = 2
	MaxDatagramSize = 4 * 1024
	_               = uint(0xFFFF - MaxDatagramSize)
)

func Frame(payload []byte) ([]byte, error) {
	if len(payload) > MaxDatagramSize {
		return nil, ErrOversize
	}
	b := make([]byte, hdrLen+len(payload))
	binary.BigEndian.PutUint16(b, uint16(len(payload)))
	copy(b[hdrLen:], payload)
	return b, nil
}

func ReadFrame(r io.Reader, buf []byte) (int, error) {
	if len(buf) < MaxDatagramSize {
		return 0, io.ErrShortBuffer
	}

	if _, err := io.ReadFull(r, buf[:hdrLen]); err != nil {
		return 0, err
	}

	n := int(binary.BigEndian.Uint16(buf[:hdrLen]))
	if n > MaxDatagramSize {
		return 0, ErrDesync
	}
	if n == 0 {
		return 0, nil
	}

	if _, err := io.ReadFull(r, buf[:n]); err != nil {
		if errors.Is(err, io.EOF) {
			return 0, io.ErrUnexpectedEOF
		}
		return 0, err
	}
	return n, nil
}

func Enframe(dst io.Writer, src io.Reader) error {
	buf := make([]byte, hdrLen+MaxDatagramSize+1)
	for {
		n, err := src.Read(buf[hdrLen:])
		switch {
		case n > MaxDatagramSize:

		case n > 0 || err == nil:
			binary.BigEndian.PutUint16(buf, uint16(n))
			w, werr := dst.Write(buf[:hdrLen+n])
			if werr != nil {
				return werr
			}
			if w < hdrLen+n {
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

func Deframe(dst io.Writer, src io.Reader) error {
	buf := make([]byte, MaxDatagramSize)
	for {
		n, err := ReadFrame(src, buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		w, err := dst.Write(buf[:n])
		if err != nil {
			return err
		}
		if w < n {
			return io.ErrShortWrite
		}
	}
}
