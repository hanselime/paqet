package flog

import (
	"errors"
	"io"
	"net"
	"syscall"
)

func WErr(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, io.EOF):
		return nil
	case errors.Is(err, net.ErrClosed):
		return nil
	case errors.Is(err, io.ErrClosedPipe), errors.Is(err, syscall.EPIPE):
		return nil
	case errors.Is(err, syscall.ECONNRESET):
		return nil
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return nil
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Err != nil {
		return WErr(opErr.Err)
	}

	return err
}
