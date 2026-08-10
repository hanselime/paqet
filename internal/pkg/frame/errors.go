package frame

import "errors"

var (
	ErrOversize = errors.New("buffer: payload exceeds max frame size")
	ErrDesync   = errors.New("buffer: invalid frame length, stream desynchronised")
)
