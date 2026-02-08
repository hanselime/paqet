package connpool

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

var (
	ErrPoolClosed = errors.New("connection pool is closed")
	ErrPoolFull   = errors.New("connection pool is full")
)

type poolConn struct {
	net.Conn
	pool       *ConnPool
	unusable   bool
	lastUsed   time.Time
	returnedAt time.Time
}

func (pc *poolConn) Close() error {
	if pc.unusable {
		// Connection is unusable, close it immediately
		if pc.Conn != nil {
			return pc.Conn.Close()
		}
		return nil
	}
	// Return connection to pool
	return pc.pool.put(pc)
}

// MarkUnusable marks the connection as unusable so it won't be returned to pool
func (pc *poolConn) MarkUnusable() {
	pc.unusable = true
}

type ConnPool struct {
	factory     func(context.Context) (net.Conn, error)
	conns       chan *poolConn
	mu          sync.RWMutex
	closed      bool
	idleTimeout time.Duration
	maxPoolSize int
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// New creates a new connection pool
func New(maxPoolSize int, idleTimeout time.Duration, factory func(context.Context) (net.Conn, error)) (*ConnPool, error) {
	if maxPoolSize <= 0 {
		maxPoolSize = 10
	}
	if idleTimeout <= 0 {
		idleTimeout = 90 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())
	pool := &ConnPool{
		factory:     factory,
		conns:       make(chan *poolConn, maxPoolSize),
		idleTimeout: idleTimeout,
		maxPoolSize: maxPoolSize,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start idle connection cleanup goroutine
	pool.wg.Add(1)
	go pool.cleanupIdleConns()

	return pool, nil
}

// Get retrieves a connection from the pool or creates a new one
func (p *ConnPool) Get(ctx context.Context) (net.Conn, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, ErrPoolClosed
	}
	p.mu.RUnlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case pc := <-p.conns:
		// Got a connection from pool
		// Check if it's still valid
		if pc.Conn == nil {
			// Try to get another one
			return p.Get(ctx)
		}
		// Test connection with a quick operation (set deadline)
		if err := pc.SetDeadline(time.Now().Add(1 * time.Second)); err != nil {
			// Connection is dead, close it and get a new one
			pc.Conn.Close()
			return p.Get(ctx)
		}
		// Reset deadline
		pc.SetDeadline(time.Time{})
		pc.lastUsed = time.Now()
		return pc, nil
	default:
		// No connection available, create new one
		conn, err := p.factory(ctx)
		if err != nil {
			return nil, err
		}
		pc := &poolConn{
			Conn:     conn,
			pool:     p,
			lastUsed: time.Now(),
		}
		return pc, nil
	}
}

// put returns a connection to the pool
func (p *ConnPool) put(pc *poolConn) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		// Pool is closed, close the connection
		if pc.Conn != nil {
			return pc.Conn.Close()
		}
		return nil
	}

	if pc.unusable {
		// Connection is unusable, close it
		if pc.Conn != nil {
			return pc.Conn.Close()
		}
		return nil
	}

	pc.returnedAt = time.Now()

	select {
	case p.conns <- pc:
		// Successfully returned to pool
		return nil
	default:
		// Pool is full, close the connection
		if pc.Conn != nil {
			return pc.Conn.Close()
		}
		return nil
	}
}

// Close closes all connections in the pool
func (p *ConnPool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	// Signal cleanup goroutine to stop
	p.cancel()

	// Close all connections in the pool
	close(p.conns)
	for pc := range p.conns {
		if pc.Conn != nil {
			pc.Conn.Close()
		}
	}

	// Wait for cleanup goroutine to finish
	p.wg.Wait()

	return nil
}

// cleanupIdleConns periodically removes idle connections from the pool
func (p *ConnPool) cleanupIdleConns() {
	defer p.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.mu.RLock()
			if p.closed {
				p.mu.RUnlock()
				return
			}
			p.mu.RUnlock()

			// Try to check and remove idle connections
			toCheck := len(p.conns)
			for i := 0; i < toCheck; i++ {
				select {
				case pc := <-p.conns:
					if pc == nil || pc.Conn == nil {
						continue
					}
					// Check if connection has been idle too long
					idleTime := time.Since(pc.returnedAt)
					if idleTime > p.idleTimeout {
						// Connection has been idle too long, close it
						pc.Conn.Close()
					} else {
						// Return to pool
						select {
						case p.conns <- pc:
						default:
							// Pool full, close connection
							pc.Conn.Close()
						}
					}
				default:
					// No more connections to check
					break
				}
			}
		}
	}
}

// Len returns the number of connections currently in the pool
func (p *ConnPool) Len() int {
	return len(p.conns)
}
