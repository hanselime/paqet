package socket

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"net"
	"paqet/internal/conf"
	"paqet/internal/pkg/hash"
	"paqet/internal/pkg/iterator"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
)

type TCPF struct {
	tcpF       iterator.Iterator[conf.TCPF]
	clientTCPF map[uint64]*iterator.Iterator[conf.TCPF]
	mu         sync.RWMutex
}

type sendRequest struct {
	payload []byte
	addr    *net.UDPAddr
	errChan chan error
	retries int
}

type SendHandle struct {
	handle         *pcap.Handle
	srcIPv4        net.IP
	srcIPv4RHWA    net.HardwareAddr
	srcIPv6        net.IP
	srcIPv6RHWA    net.HardwareAddr
	srcPort        uint16
	synOptions     []layers.TCPOption
	ackOptions     []layers.TCPOption
	time           uint32
	tsCounter      uint32
	tcpF           TCPF
	ethPool        sync.Pool
	ipv4Pool       sync.Pool
	ipv6Pool       sync.Pool
	tcpPool        sync.Pool
	bufPool        sync.Pool
	sendQueue      chan *sendRequest
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	cfg            *conf.Network
	droppedPackets atomic.Uint64
}

func NewSendHandle(cfg *conf.Network) (*SendHandle, error) {
	handle, err := newHandle(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to open pcap handle: %w", err)
	}

	// SetDirection is not fully supported on Windows Npcap, so skip it
	if runtime.GOOS != "windows" {
		if err := handle.SetDirection(pcap.DirectionOut); err != nil {
			return nil, fmt.Errorf("failed to set pcap direction out: %v", err)
		}
	}

	synOptions := []layers.TCPOption{
		{OptionType: layers.TCPOptionKindMSS, OptionLength: 4, OptionData: []byte{0x05, 0xb4}},
		{OptionType: layers.TCPOptionKindSACKPermitted, OptionLength: 2},
		{OptionType: layers.TCPOptionKindTimestamps, OptionLength: 10, OptionData: make([]byte, 8)},
		{OptionType: layers.TCPOptionKindNop},
		{OptionType: layers.TCPOptionKindWindowScale, OptionLength: 3, OptionData: []byte{8}},
	}

	ackOptions := []layers.TCPOption{
		{OptionType: layers.TCPOptionKindNop},
		{OptionType: layers.TCPOptionKindNop},
		{OptionType: layers.TCPOptionKindTimestamps, OptionLength: 10, OptionData: make([]byte, 8)},
	}

	ctx, cancel := context.WithCancel(context.Background())
	sh := &SendHandle{
		handle:     handle,
		srcPort:    uint16(cfg.Port),
		synOptions: synOptions,
		ackOptions: ackOptions,
		tcpF:       TCPF{tcpF: iterator.Iterator[conf.TCPF]{Items: cfg.TCP.LF}, clientTCPF: make(map[uint64]*iterator.Iterator[conf.TCPF])},
		time:       uint32(time.Now().UnixNano() / int64(time.Millisecond)),
		cfg:        cfg,
		sendQueue:  make(chan *sendRequest, cfg.PCAP.SendQueueSize),
		ctx:        ctx,
		cancel:     cancel,
		ethPool: sync.Pool{
			New: func() any {
				return &layers.Ethernet{SrcMAC: cfg.Interface.HardwareAddr}
			},
		},
		ipv4Pool: sync.Pool{
			New: func() any {
				return &layers.IPv4{}
			},
		},
		ipv6Pool: sync.Pool{
			New: func() any {
				return &layers.IPv6{}
			},
		},
		tcpPool: sync.Pool{
			New: func() any {
				return &layers.TCP{}
			},
		},
		bufPool: sync.Pool{
			New: func() any {
				return gopacket.NewSerializeBuffer()
			},
		},
	}
	if cfg.IPv4.Addr != nil {
		sh.srcIPv4 = cfg.IPv4.Addr.IP
		sh.srcIPv4RHWA = cfg.IPv4.Router
	}
	if cfg.IPv6.Addr != nil {
		sh.srcIPv6 = cfg.IPv6.Addr.IP
		sh.srcIPv6RHWA = cfg.IPv6.Router
	}

	// Start multiple background workers to process send queue for parallelism
	numWorkers := 1
	if cfg.Performance != nil && cfg.Performance.PacketWorkers > 0 {
		numWorkers = cfg.Performance.PacketWorkers
	}
	
	for i := 0; i < numWorkers; i++ {
		sh.wg.Add(1)
		go sh.processQueue()
	}

	return sh, nil
}

func (h *SendHandle) buildIPv4Header(dstIP net.IP) *layers.IPv4 {
	ip := h.ipv4Pool.Get().(*layers.IPv4)
	*ip = layers.IPv4{
		Version:  4,
		IHL:      5,
		TOS:      184,
		TTL:      64,
		Flags:    layers.IPv4DontFragment,
		Protocol: layers.IPProtocolTCP,
		SrcIP:    h.srcIPv4,
		DstIP:    dstIP,
	}
	return ip
}

func (h *SendHandle) buildIPv6Header(dstIP net.IP) *layers.IPv6 {
	ip := h.ipv6Pool.Get().(*layers.IPv6)
	*ip = layers.IPv6{
		Version:      6,
		TrafficClass: 184,
		HopLimit:     64,
		NextHeader:   layers.IPProtocolTCP,
		SrcIP:        h.srcIPv6,
		DstIP:        dstIP,
	}
	return ip
}

func (h *SendHandle) buildTCPHeader(dstPort uint16, f conf.TCPF) *layers.TCP {
	tcp := h.tcpPool.Get().(*layers.TCP)
	*tcp = layers.TCP{
		SrcPort: layers.TCPPort(h.srcPort),
		DstPort: layers.TCPPort(dstPort),
		FIN:     f.FIN, SYN: f.SYN, RST: f.RST, PSH: f.PSH, ACK: f.ACK, URG: f.URG, ECE: f.ECE, CWR: f.CWR, NS: f.NS,
		Window: 65535,
	}

	counter := atomic.AddUint32(&h.tsCounter, 1)
	tsVal := h.time + (counter >> 3)
	if f.SYN {
		binary.BigEndian.PutUint32(h.synOptions[2].OptionData[0:4], tsVal)
		binary.BigEndian.PutUint32(h.synOptions[2].OptionData[4:8], 0)
		tcp.Options = h.synOptions
		tcp.Seq = 1 + (counter & 0x7)
		tcp.Ack = 0
		if f.ACK {
			tcp.Ack = tcp.Seq + 1
		}
	} else {
		tsEcr := tsVal - (counter%200 + 50)
		binary.BigEndian.PutUint32(h.ackOptions[2].OptionData[0:4], tsVal)
		binary.BigEndian.PutUint32(h.ackOptions[2].OptionData[4:8], tsEcr)
		tcp.Options = h.ackOptions
		seq := h.time + (counter << 7)
		tcp.Seq = seq
		tcp.Ack = seq - (counter & 0x3FF) + 1400
	}

	return tcp
}

func (h *SendHandle) Write(payload []byte, addr *net.UDPAddr) error {
	// Make a copy of the payload since it may be reused by caller
	payloadCopy := make([]byte, len(payload))
	copy(payloadCopy, payload)

	req := &sendRequest{
		payload: payloadCopy,
		addr:    addr,
		errChan: make(chan error, 1),
		retries: 0,
	}

	// Try to enqueue the request with flow control
	select {
	case h.sendQueue <- req:
		// Successfully queued
	case <-h.ctx.Done():
		return h.ctx.Err()
	default:
		// Queue is full - apply back-pressure
		h.droppedPackets.Add(1)
		return fmt.Errorf("send queue full, packet dropped")
	}

	// Wait for the result
	select {
	case err := <-req.errChan:
		return err
	case <-h.ctx.Done():
		return h.ctx.Err()
	}
}

func (h *SendHandle) processQueue() {
	defer h.wg.Done()

	for {
		select {
		case <-h.ctx.Done():
			return
		case req := <-h.sendQueue:
			err := h.executeWrite(req)
			if err != nil && req.retries < h.cfg.PCAP.MaxRetries {
				// Retry with exponential backoff
				req.retries++
				backoff := h.calculateBackoff(req.retries)
				
				select {
				case <-time.After(backoff):
					// Requeue for retry
					select {
					case h.sendQueue <- req:
						continue
					case <-h.ctx.Done():
						if req.errChan != nil {
							req.errChan <- h.ctx.Err()
						}
						return
					default:
						// Queue full on retry - drop
						h.droppedPackets.Add(1)
						if req.errChan != nil {
							req.errChan <- fmt.Errorf("send queue full on retry: %w", err)
						}
					}
				case <-h.ctx.Done():
					if req.errChan != nil {
						req.errChan <- h.ctx.Err()
					}
					return
				}
			} else {
				// Send result back to caller
				if req.errChan != nil {
					req.errChan <- err
				}
			}
		}
	}
}

func (h *SendHandle) calculateBackoff(retries int) time.Duration {
	// Exponential backoff with jitter
	backoffMs := float64(h.cfg.PCAP.InitialBackoff) * math.Pow(2, float64(retries-1))
	if backoffMs > float64(h.cfg.PCAP.MaxBackoff) {
		backoffMs = float64(h.cfg.PCAP.MaxBackoff)
	}
	// Add up to 20% jitter to avoid thundering herd
	jitter := backoffMs * 0.2 * rand.Float64()
	return time.Duration(backoffMs+jitter) * time.Millisecond
}

func (h *SendHandle) executeWrite(req *sendRequest) error {
	buf := h.bufPool.Get().(gopacket.SerializeBuffer)
	ethLayer := h.ethPool.Get().(*layers.Ethernet)
	defer func() {
		buf.Clear()
		h.bufPool.Put(buf)
		h.ethPool.Put(ethLayer)
	}()

	dstIP := req.addr.IP
	dstPort := uint16(req.addr.Port)

	f := h.getClientTCPF(dstIP, dstPort)
	tcpLayer := h.buildTCPHeader(dstPort, f)
	defer h.tcpPool.Put(tcpLayer)

	var ipLayer gopacket.SerializableLayer
	if dstIP.To4() != nil {
		ip := h.buildIPv4Header(dstIP)
		defer h.ipv4Pool.Put(ip)
		ipLayer = ip
		tcpLayer.SetNetworkLayerForChecksum(ip)
		ethLayer.DstMAC = h.srcIPv4RHWA
		ethLayer.EthernetType = layers.EthernetTypeIPv4
	} else {
		ip := h.buildIPv6Header(dstIP)
		defer h.ipv6Pool.Put(ip)
		ipLayer = ip
		tcpLayer.SetNetworkLayerForChecksum(ip)
		ethLayer.DstMAC = h.srcIPv6RHWA
		ethLayer.EthernetType = layers.EthernetTypeIPv6
	}

	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, ethLayer, ipLayer, tcpLayer, gopacket.Payload(req.payload)); err != nil {
		return err
	}
	return h.handle.WritePacketData(buf.Bytes())
}

func (h *SendHandle) getClientTCPF(dstIP net.IP, dstPort uint16) conf.TCPF {
	h.tcpF.mu.RLock()
	defer h.tcpF.mu.RUnlock()
	if ff := h.tcpF.clientTCPF[hash.IPAddr(dstIP, dstPort)]; ff != nil {
		return ff.Next()
	}
	return h.tcpF.tcpF.Next()
}

func (h *SendHandle) setClientTCPF(addr net.Addr, f []conf.TCPF) {
	a := *addr.(*net.UDPAddr)
	h.tcpF.mu.Lock()
	h.tcpF.clientTCPF[hash.IPAddr(a.IP, uint16(a.Port))] = &iterator.Iterator[conf.TCPF]{Items: f}
	h.tcpF.mu.Unlock()
}

func (h *SendHandle) Close() {
	if h.cancel != nil {
		h.cancel()
	}
	h.wg.Wait()
	if h.sendQueue != nil {
		close(h.sendQueue)
	}
	if h.handle != nil {
		h.handle.Close()
	}
}

func (h *SendHandle) DroppedPackets() uint64 {
	return h.droppedPackets.Load()
}

func (h *SendHandle) QueueDepth() int {
	return len(h.sendQueue)
}
