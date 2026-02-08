package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"paqet/internal/conf"
	"paqet/internal/flog"
	"paqet/internal/pkg/connpool"
	"paqet/internal/socket"
	"paqet/internal/tnet"
	"paqet/internal/tnet/kcp"
)

type Server struct {
	cfg              *conf.Conf
	pConn            *socket.PacketConn
	wg               sync.WaitGroup
	streamSemaphore  chan struct{}       // Limits concurrent stream processing
	connPools        map[string]*connpool.ConnPool
	connPoolsMu      sync.RWMutex
}

func New(cfg *conf.Conf) (*Server, error) {
	s := &Server{
		cfg: cfg,
	}
	
	// Initialize semaphore for limiting concurrent streams
	maxStreams := cfg.Performance.MaxConcurrentStreams
	if maxStreams > 0 {
		s.streamSemaphore = make(chan struct{}, maxStreams)
	}
	
	// Initialize connection pools map if enabled
	if cfg.Performance.EnableConnectionPooling {
		s.connPools = make(map[string]*connpool.ConnPool)
	}

	return s, nil
}

// getConnPool gets or creates a connection pool for a specific target address
func (s *Server) getConnPool(addr string) (*connpool.ConnPool, error) {
	if !s.cfg.Performance.EnableConnectionPooling {
		return nil, nil
	}
	
	s.connPoolsMu.RLock()
	pool, exists := s.connPools[addr]
	s.connPoolsMu.RUnlock()
	
	if exists {
		return pool, nil
	}
	
	// Create new pool
	s.connPoolsMu.Lock()
	defer s.connPoolsMu.Unlock()
	
	// Double-check after acquiring write lock
	pool, exists = s.connPools[addr]
	if exists {
		return pool, nil
	}
	
	// Create connection factory
	factory := func(ctx context.Context) (net.Conn, error) {
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		return dialer.DialContext(ctx, "tcp", addr)
	}
	
	pool, err := connpool.New(
		s.cfg.Performance.TCPConnectionPoolSize,
		time.Duration(s.cfg.Performance.TCPConnectionIdleTimeout)*time.Second,
		factory,
	)
	if err != nil {
		return nil, err
	}
	
	s.connPools[addr] = pool
	return pool, nil
}

func (s *Server) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		flog.Infof("Shutdown signal received, initiating graceful shutdown...")
		cancel()
	}()

	pConn, err := socket.New(ctx, &s.cfg.Network)
	if err != nil {
		return fmt.Errorf("could not create raw packet conn: %w", err)
	}
	s.pConn = pConn

	listener, err := kcp.Listen(s.cfg.Transport.KCP, pConn)
	if err != nil {
		return fmt.Errorf("could not start KCP listener: %w", err)
	}
	defer listener.Close()
	
	poolingStatus := "disabled"
	if s.cfg.Performance.EnableConnectionPooling {
		poolingStatus = fmt.Sprintf("enabled (pool size: %d, idle timeout: %ds)", 
			s.cfg.Performance.TCPConnectionPoolSize, 
			s.cfg.Performance.TCPConnectionIdleTimeout)
	}
	flog.Infof("Server started - listening for packets on :%d (max concurrent streams: %d, connection pooling: %s)", 
		s.cfg.Listen.Addr.Port, 
		s.cfg.Performance.MaxConcurrentStreams,
		poolingStatus)

	s.wg.Go(func() {
		s.listen(ctx, listener)
	})

	s.wg.Wait()
	
	// Close all connection pools
	if s.cfg.Performance.EnableConnectionPooling {
		s.connPoolsMu.Lock()
		for addr, pool := range s.connPools {
			flog.Debugf("closing connection pool for %s", addr)
			pool.Close()
		}
		s.connPoolsMu.Unlock()
	}
	
	flog.Infof("Server shutdown completed")
	return nil
}

func (s *Server) listen(ctx context.Context, listener tnet.Listener) {
	go func() {
		<-ctx.Done()
		listener.Close()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		conn, err := listener.Accept()
		if err != nil {
			flog.Errorf("failed to accept connection: %v", err)
			continue
		}
		flog.Infof("accepted new connection from %s (local: %s)", conn.RemoteAddr(), conn.LocalAddr())

		s.wg.Go(func() {
			defer conn.Close()
			s.handleConn(ctx, conn)
		})
	}
}
