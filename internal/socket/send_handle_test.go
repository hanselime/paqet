package socket

import (
	"context"
	"net"
	"paqet/internal/conf"
	"paqet/internal/pkg/iterator"
	"testing"
	"time"
)

// TestSendQueueBackpressure tests that the send queue properly applies backpressure
func TestSendQueueBackpressure(t *testing.T) {
	// Create a minimal configuration
	cfg := &conf.Network{
		PCAP: conf.PCAP{
			SendQueueSize:  2, // Small queue size for testing
			MaxRetries:     0, // No retries for this test
			InitialBackoff: 10,
			MaxBackoff:     100,
		},
		TCP: conf.TCP{
			LF: []conf.TCPF{{PSH: true, ACK: true}},
		},
	}

	// Create a mock send handle without actual pcap handle
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sh := &SendHandle{
		cfg:       cfg,
		sendQueue: make(chan *sendRequest, cfg.PCAP.SendQueueSize),
		ctx:       ctx,
		cancel:    cancel,
		tcpF:      TCPF{tcpF: iterator.Iterator[conf.TCPF]{Items: cfg.TCP.LF}, clientTCPF: make(map[uint64]*iterator.Iterator[conf.TCPF])},
	}

	// Don't start processQueue goroutine so queue fills up

	// Test that we can send up to queue size
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}
	
	// Fill the queue with goroutines that will block waiting for results
	errChan := make(chan error, cfg.PCAP.SendQueueSize+1)
	for i := 0; i < cfg.PCAP.SendQueueSize; i++ {
		go func() {
			err := sh.Write([]byte("test"), addr)
			errChan <- err
		}()
	}

	// Wait a bit for goroutines to queue requests
	time.Sleep(10 * time.Millisecond)

	// Next write should fail with backpressure
	err := sh.Write([]byte("test"), addr)
	if err == nil {
		t.Error("Expected error due to full queue, got nil")
	}
	if err != nil && err.Error() != "send queue full, packet dropped" {
		t.Errorf("Expected 'send queue full' error, got: %v", err)
	}

	// Check dropped packets counter
	if sh.DroppedPackets() == 0 {
		t.Error("Expected dropped packets counter to be incremented")
	}
}

// TestCalculateBackoff tests the exponential backoff calculation
func TestCalculateBackoff(t *testing.T) {
	cfg := &conf.Network{
		PCAP: conf.PCAP{
			InitialBackoff: 10,
			MaxBackoff:     1000,
		},
	}

	sh := &SendHandle{
		cfg: cfg,
	}

	// Test first retry
	backoff1 := sh.calculateBackoff(1)
	if backoff1 < 10*time.Millisecond || backoff1 > 12*time.Millisecond {
		t.Errorf("First retry backoff out of expected range: %v", backoff1)
	}

	// Test second retry (should be roughly 2x)
	backoff2 := sh.calculateBackoff(2)
	if backoff2 < 20*time.Millisecond || backoff2 > 24*time.Millisecond {
		t.Errorf("Second retry backoff out of expected range: %v", backoff2)
	}

	// Test that it caps at max backoff
	backoff10 := sh.calculateBackoff(10)
	if backoff10 < 1000*time.Millisecond || backoff10 > 1200*time.Millisecond {
		t.Errorf("High retry backoff should be capped at max: %v", backoff10)
	}
}

// TestQueueDepth tests the queue depth reporting
func TestQueueDepth(t *testing.T) {
	cfg := &conf.Network{
		PCAP: conf.PCAP{
			SendQueueSize:  10,
			MaxRetries:     0,
			InitialBackoff: 10,
			MaxBackoff:     100,
		},
		TCP: conf.TCP{
			LF: []conf.TCPF{{PSH: true, ACK: true}},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sh := &SendHandle{
		cfg:       cfg,
		sendQueue: make(chan *sendRequest, cfg.PCAP.SendQueueSize),
		ctx:       ctx,
		cancel:    cancel,
		tcpF:      TCPF{tcpF: iterator.Iterator[conf.TCPF]{Items: cfg.TCP.LF}, clientTCPF: make(map[uint64]*iterator.Iterator[conf.TCPF])},
	}

	// Initially queue should be empty
	if depth := sh.QueueDepth(); depth != 0 {
		t.Errorf("Expected queue depth 0, got %d", depth)
	}

	// Add some items
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}
	for i := 0; i < 3; i++ {
		go func() {
			_ = sh.Write([]byte("test"), addr)
		}()
	}

	// Wait for items to be queued
	time.Sleep(10 * time.Millisecond)

	// Check depth
	if depth := sh.QueueDepth(); depth != 3 {
		t.Errorf("Expected queue depth 3, got %d", depth)
	}
}

// TestSetClientTCPFWithNilAddr tests that setClientTCPF handles nil address gracefully
func TestSetClientTCPFWithNilAddr(t *testing.T) {
	cfg := &conf.Network{
		TCP: conf.TCP{
			LF: []conf.TCPF{{PSH: true, ACK: true}},
		},
	}

	sh := &SendHandle{
		cfg:  cfg,
		tcpF: TCPF{tcpF: iterator.Iterator[conf.TCPF]{Items: cfg.TCP.LF}, clientTCPF: make(map[uint64]*iterator.Iterator[conf.TCPF])},
	}

	// Test with nil address (should not panic)
	tcpFlags := []conf.TCPF{{PSH: true, ACK: true}}
	sh.setClientTCPF(nil, tcpFlags)

	// Verify that no entry was added to the map
	if len(sh.tcpF.clientTCPF) != 0 {
		t.Errorf("Expected clientTCPF map to be empty, got %d entries", len(sh.tcpF.clientTCPF))
	}

	// Test with valid address
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080}
	sh.setClientTCPF(addr, tcpFlags)

	// Verify that the entry was added
	if len(sh.tcpF.clientTCPF) != 1 {
		t.Errorf("Expected clientTCPF map to have 1 entry, got %d", len(sh.tcpF.clientTCPF))
	}
}
