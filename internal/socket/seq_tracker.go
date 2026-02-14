package socket

import (
	"crypto/rand"
	"encoding/binary"
	"net"
	"paqet/internal/pkg/hash"
	"sync"
	"time"
)


type DestinationSeq struct {
	localSeq   uint32
	remoteAck  uint32
	lastUpdate time.Time
	mu         sync.Mutex
}


type SeqTracker struct {
	sequences sync.Map
	
	cleanupInterval time.Duration
	maxIdleTime     time.Duration
	stopCleanup     chan struct{}
}


func NewSeqTracker(cleanupInterval, maxIdleTime time.Duration) *SeqTracker {
	st := &SeqTracker{
		cleanupInterval: cleanupInterval,
		maxIdleTime:     maxIdleTime,
		stopCleanup:     make(chan struct{}),
	}
	
	go st.cleanupLoop()
	
	return st
}


func (st *SeqTracker) GetSeqAck(dstIP net.IP, dstPort uint16, payloadLen int) (seq, ack uint32) {
	key := hash.IPAddr(dstIP, dstPort)
	
	if val, ok := st.sequences.Load(key); ok {
		ds := val.(*DestinationSeq)
		ds.mu.Lock()
		defer ds.mu.Unlock()
		
		seq = ds.localSeq
		ack = ds.remoteAck
		
		ds.localSeq += uint32(payloadLen)
		
		if payloadLen > 0 {
			ds.remoteAck += uint32(payloadLen / 2)
		} else {
			ds.remoteAck += 1
		}
		
		ds.lastUpdate = time.Now()
		
		return seq, ack
	}
	
	seq = randomISN()
	ack = randomISN()
	
	ds := &DestinationSeq{
		localSeq:   seq + uint32(payloadLen),
		remoteAck:  ack,
		lastUpdate: time.Now(),
	}
	
	st.sequences.Store(key, ds)
	
	return seq, ack
}


func (st *SeqTracker) GetSeqAckForAddr(addr *net.UDPAddr, payloadLen int) (seq, ack uint32) {
	return st.GetSeqAck(addr.IP, uint16(addr.Port), payloadLen)
}


func (st *SeqTracker) ResetDestination(dstIP net.IP, dstPort uint16) {
	key := hash.IPAddr(dstIP, dstPort)
	st.sequences.Delete(key)
}


func (st *SeqTracker) cleanupLoop() {
	ticker := time.NewTicker(st.cleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			st.cleanupStale()
		case <-st.stopCleanup:
			return
		}
	}
}


func (st *SeqTracker) cleanupStale() {
	now := time.Now()
	toDelete := make([]uint64, 0)
	
	st.sequences.Range(func(key, value interface{}) bool {
		ds := value.(*DestinationSeq)
		ds.mu.Lock()
		lastUpdate := ds.lastUpdate
		ds.mu.Unlock()
		
		if now.Sub(lastUpdate) > st.maxIdleTime {
			toDelete = append(toDelete, key.(uint64))
		}
		return true
	})
	
	for _, key := range toDelete {
		st.sequences.Delete(key)
	}
}


func (st *SeqTracker) GetStats() (activeConnections int) {
	st.sequences.Range(func(key, value interface{}) bool {
		activeConnections++
		return true
	})
	return activeConnections
}


func (st *SeqTracker) Close() {
	close(st.stopCleanup)
}


func randomISN() uint32 {
	var b [4]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return uint32(time.Now().UnixNano() & 0xFFFFFFFF)
	}
	return binary.BigEndian.Uint32(b[:])
}
