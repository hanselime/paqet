package quic

import (
	"io"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

// Strm wraps a QUIC stream to implement the tnet.Strm interface
type Strm struct {
	stream quic.Stream
}

func (s *Strm) Read(p []byte) (n int, err error) {
	return s.stream.Read(p)
}

func (s *Strm) Write(p []byte) (n int, err error) {
	return s.stream.Write(p)
}

func (s *Strm) Close() error {
	return s.stream.Close()
}

func (s *Strm) LocalAddr() net.Addr {
	// QUIC streams don't have their own addresses, return nil
	return nil
}

func (s *Strm) RemoteAddr() net.Addr {
	// QUIC streams don't have their own addresses, return nil
	return nil
}

func (s *Strm) SetDeadline(t time.Time) error {
	return s.stream.SetDeadline(t)
}

func (s *Strm) SetReadDeadline(t time.Time) error {
	return s.stream.SetReadDeadline(t)
}

func (s *Strm) SetWriteDeadline(t time.Time) error {
	return s.stream.SetWriteDeadline(t)
}

func (s *Strm) CloseWrite() error {
	s.stream.CancelWrite(0)
	return nil
}

func (s *Strm) CloseRead() error {
	s.stream.CancelRead(0)
	return nil
}

// WriteTo implements io.WriterTo for efficient copying
func (s *Strm) WriteTo(w io.Writer) (n int64, err error) {
	return io.Copy(w, s.stream)
}

// ReadFrom implements io.ReaderFrom for efficient copying
func (s *Strm) ReadFrom(r io.Reader) (n int64, err error) {
	return io.Copy(s.stream, r)
}
