package queue

import (
	"context"
	"encoding/binary"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"paqet/internal/pkg/frame"
)

const MaxDatagramSize = 4 * 1024

type deadliner interface {
	SetWriteDeadline(time.Time) error
}

type Queue struct {
	mu      sync.Mutex
	pending []byte
	wake    chan struct{}

	limit   int
	timeout time.Duration
	dropped atomic.Uint64
}

func New(limit int, timeout time.Duration) *Queue {
	if limit < MaxDatagramSize {
		limit = MaxDatagramSize
	}
	return &Queue{
		wake:    make(chan struct{}, 1),
		limit:   limit,
		timeout: timeout,
	}
}

func (q *Queue) Push(payload []byte) bool {
	q.mu.Lock()

	if len(q.pending) >= q.limit {
		q.mu.Unlock()
		q.dropped.Add(1)
		return false
	}

	next, err := AppendFrame(q.pending, payload)
	if err != nil {
		q.mu.Unlock()
		q.dropped.Add(1)
		return false
	}
	q.pending = next
	q.mu.Unlock()

	select {
	case q.wake <- struct{}{}:
	default:
	}
	return true
}

func (q *Queue) Run(ctx context.Context, w io.Writer, onWrite func()) error {
	dl, _ := w.(deadliner)

	var spare []byte

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-q.wake:
			q.mu.Lock()
			batch := q.pending
			q.pending = spare[:0]
			q.mu.Unlock()

			if len(batch) == 0 {
				continue
			}

			if dl != nil && q.timeout > 0 {
				if err := dl.SetWriteDeadline(time.Now().Add(q.timeout)); err != nil {
					return err
				}
			}
			n, err := w.Write(batch)

			spare = batch

			if err != nil {
				return err
			}
			if n < len(batch) {
				return io.ErrShortWrite
			}
			if onWrite != nil {
				onWrite()
			}
		}
	}
}

func (q *Queue) Fill(src io.Reader, max int) error {
	buf := make([]byte, max+1)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			q.Push(buf[:n])
		}
		if err != nil {
			return err
		}
	}
}

func (q *Queue) Dropped() uint64 { return q.dropped.Load() }

func AppendFrame(dst, payload []byte) ([]byte, error) {
	if len(payload) > frame.MaxDatagramSize {
		return dst, frame.ErrOversize
	}
	dst = binary.BigEndian.AppendUint16(dst, uint16(len(payload)))
	return append(dst, payload...), nil
}
