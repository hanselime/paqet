package socket

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"paqet/internal/pkg/hash"
	"sync"
	"sync/atomic"
)


type PortPool struct {
	ports     []uint16
	current   uint32
	allocated map[uint64]uint16
	mu        sync.RWMutex
	basePort  uint16
	poolSize  int
}


type PortPoolConfig struct {
	Enabled   bool
	StartPort uint16
	EndPort   uint16
}


func DefaultPortPoolConfig() PortPoolConfig {
	return PortPoolConfig{
		Enabled:   true,
		StartPort: 50000,
		EndPort:   51000,
	}
}


func NewPortPool(config PortPoolConfig) (*PortPool, error) {
	if !config.Enabled {
		return nil, nil
	}
	
	if config.EndPort <= config.StartPort {
		return nil, fmt.Errorf("end port (%d) must be greater than start port (%d)", 
			config.EndPort, config.StartPort)
	}
	
	poolSize := int(config.EndPort - config.StartPort)
	if poolSize < 1 {
		return nil, fmt.Errorf("port pool size must be at least 1")
	}
	
	ports := make([]uint16, poolSize)
	for i := 0; i < poolSize; i++ {
		ports[i] = config.StartPort + uint16(i)
	}
	
	shufflePorts(ports)
	
	return &PortPool{
		ports:     ports,
		current:   0,
		allocated: make(map[uint64]uint16),
		basePort:  config.StartPort,
		poolSize:  poolSize,
	}, nil
}


func (pp *PortPool) AllocatePort(dstIP net.IP, dstPort uint16) uint16 {
	if pp == nil {
		return 0
	}
	
	connKey := hash.IPAddr(dstIP, dstPort)
	
	pp.mu.RLock()
	if port, exists := pp.allocated[connKey]; exists {
		pp.mu.RUnlock()
		return port
	}
	pp.mu.RUnlock()
	
	pp.mu.Lock()
	defer pp.mu.Unlock()
	
	if port, exists := pp.allocated[connKey]; exists {
		return port
	}
	
	idx := atomic.AddUint32(&pp.current, 1) % uint32(pp.poolSize)
	port := pp.ports[idx]
	
	pp.allocated[connKey] = port
	
	return port
}


func (pp *PortPool) AllocatePortForAddr(addr *net.UDPAddr) uint16 {
	if pp == nil {
		return 0
	}
	return pp.AllocatePort(addr.IP, uint16(addr.Port))
}


func (pp *PortPool) ReleasePort(dstIP net.IP, dstPort uint16) {
	if pp == nil {
		return
	}
	
	connKey := hash.IPAddr(dstIP, dstPort)
	pp.mu.Lock()
	delete(pp.allocated, connKey)
	pp.mu.Unlock()
}


func (pp *PortPool) ReleasePortForAddr(addr *net.UDPAddr) {
	if pp == nil {
		return
	}
	pp.ReleasePort(addr.IP, uint16(addr.Port))
}


func (pp *PortPool) GetStats() (totalPorts, allocatedPorts int) {
	if pp == nil {
		return 0, 0
	}
	
	pp.mu.RLock()
	defer pp.mu.RUnlock()
	
	return pp.poolSize, len(pp.allocated)
}


func (pp *PortPool) Reset() {
	if pp == nil {
		return
	}
	
	pp.mu.Lock()
	pp.allocated = make(map[uint64]uint16)
	pp.mu.Unlock()
}


func shufflePorts(ports []uint16) {
	n := len(ports)
	
	randomBytes := make([]byte, n*4)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return
	}
	
	for i := n - 1; i > 0; i-- {
		offset := i * 4
		j := int(binary.BigEndian.Uint32(randomBytes[offset:offset+4])) % (i + 1)
		
		ports[i], ports[j] = ports[j], ports[i]
	}
}


func (pp *PortPool) PortRange() (start, end uint16) {
	if pp == nil {
		return 0, 0
	}
	return pp.basePort, pp.basePort + uint16(pp.poolSize)
}
