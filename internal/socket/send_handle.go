package socket

import (
	"encoding/binary"
	"fmt"
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

type SendHandle struct {
	handle      *pcap.Handle
	srcIPv4     net.IP
	srcIPv4RHWA net.HardwareAddr
	srcIPv6     net.IP
	srcIPv6RHWA net.HardwareAddr
	srcPort     uint16
	ipv4TOS     uint8
	ipv4DF      bool
	ipv4TTL     uint8
	ipv6TC      uint8
	ipv6Hop     uint8
	synOptions  []layers.TCPOption
	ackOptions  []layers.TCPOption
	time        uint32
	tsCounter   uint32
	tcpF        TCPF
	flowMu      sync.RWMutex
	flows       map[uint64]*flowState
	ethPool     sync.Pool
	ipv4Pool    sync.Pool
	ipv6Pool    sync.Pool
	tcpPool     sync.Pool
	bufPool     sync.Pool
}

type flowState struct {
	mu              sync.Mutex
	nextSeq         uint32
	lastRemoteSeq   uint32
	lastRemoteInc   uint32
	lastRemoteSeen  bool
	lastRemoteTSVal uint32
}

func cloneTCPOptions(opts []layers.TCPOption) []layers.TCPOption {
	c := make([]layers.TCPOption, len(opts))
	for i, opt := range opts {
		c[i] = opt
		if opt.OptionData != nil {
			b := make([]byte, len(opt.OptionData))
			copy(b, opt.OptionData)
			c[i].OptionData = b
		}
	}
	return c
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

	rand.Seed(time.Now().UnixNano())

	sh := &SendHandle{
		handle:     handle,
		srcPort:    uint16(cfg.Port),
		ipv4TOS:    uint8(cfg.IPv4TOS),
		ipv4DF:     cfg.IPv4DF,
		ipv4TTL:    uint8(cfg.IPv4TTL),
		ipv6TC:     uint8(cfg.IPv6TC),
		ipv6Hop:    uint8(cfg.IPv6Hop),
		synOptions: synOptions,
		ackOptions: ackOptions,
		tcpF:       TCPF{tcpF: iterator.Iterator[conf.TCPF]{Items: cfg.TCP.LF}, clientTCPF: make(map[uint64]*iterator.Iterator[conf.TCPF])},
		time:       uint32(time.Now().UnixNano() / int64(time.Millisecond)),
		flows:      make(map[uint64]*flowState),
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
	return sh, nil
}

func (h *SendHandle) buildIPv4Header(dstIP net.IP) *layers.IPv4 {
	ip := h.ipv4Pool.Get().(*layers.IPv4)
	flags := layers.IPv4DontFragment
	if !h.ipv4DF {
		flags = 0
	}
	*ip = layers.IPv4{
		Version:  4,
		IHL:      5,
		TOS:      h.ipv4TOS,
		TTL:      h.ipv4TTL,
		Flags:    flags,
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
		TrafficClass: h.ipv6TC,
		HopLimit:     h.ipv6Hop,
		NextHeader:   layers.IPProtocolTCP,
		SrcIP:        h.srcIPv6,
		DstIP:        dstIP,
	}
	return ip
}

func (h *SendHandle) buildTCPHeader(dstIP net.IP, dstPort uint16, f conf.TCPF, payloadLen int) *layers.TCP {
	tcp := h.tcpPool.Get().(*layers.TCP)
	*tcp = layers.TCP{
		SrcPort: layers.TCPPort(h.srcPort),
		DstPort: layers.TCPPort(dstPort),
		FIN:     f.FIN, SYN: f.SYN, RST: f.RST, PSH: f.PSH, ACK: f.ACK, URG: f.URG, ECE: f.ECE, CWR: f.CWR, NS: f.NS,
		Window: 65535,
	}

	state := h.getFlowState(dstIP, dstPort)
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.nextSeq == 0 {
		state.nextSeq = rand.Uint32()
	}

	seq := state.nextSeq
	ack := uint32(0)
	if state.lastRemoteSeen {
		ack = state.lastRemoteSeq + state.lastRemoteInc
	}

	counter := atomic.AddUint32(&h.tsCounter, 1)
	tsVal := h.time + (counter >> 3)
	tsEcr := state.lastRemoteTSVal

	if f.SYN {
		opts := cloneTCPOptions(h.synOptions)
		binary.BigEndian.PutUint32(opts[2].OptionData[0:4], tsVal)
		binary.BigEndian.PutUint32(opts[2].OptionData[4:8], 0)
		tcp.Options = opts
		tcp.Seq = seq
		if f.ACK {
			tcp.Ack = ack
		}
	} else {
		opts := cloneTCPOptions(h.ackOptions)
		binary.BigEndian.PutUint32(opts[2].OptionData[0:4], tsVal)
		binary.BigEndian.PutUint32(opts[2].OptionData[4:8], tsEcr)
		tcp.Options = opts
		tcp.Seq = seq
		if f.ACK {
			tcp.Ack = ack
		}
	}

	inc := uint32(payloadLen)
	if f.SYN || f.FIN {
		inc++
	}
	state.nextSeq = seq + inc

	return tcp
}

func (h *SendHandle) Write(payload []byte, addr *net.UDPAddr) error {
	buf := h.bufPool.Get().(gopacket.SerializeBuffer)
	ethLayer := h.ethPool.Get().(*layers.Ethernet)
	defer func() {
		buf.Clear()
		h.bufPool.Put(buf)
		h.ethPool.Put(ethLayer)
	}()

	dstIP := addr.IP
	dstPort := uint16(addr.Port)

	f := h.getClientTCPF(dstIP, dstPort)
	tcpLayer := h.buildTCPHeader(dstIP, dstPort, f, len(payload))
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
	if err := gopacket.SerializeLayers(buf, opts, ethLayer, ipLayer, tcpLayer, gopacket.Payload(payload)); err != nil {
		return err
	}
	return h.handle.WritePacketData(buf.Bytes())
}

func (h *SendHandle) observeTCP(meta *TCPMeta) {
	if meta == nil {
		return
	}
	if meta.SrcIP == nil || len(meta.SrcIP) == 0 {
		return
	}
	if meta.SrcPort == 0 {
		return
	}
	state := h.getFlowState(meta.SrcIP, meta.SrcPort)
	state.mu.Lock()
	defer state.mu.Unlock()

	state.lastRemoteSeen = true
	state.lastRemoteSeq = meta.Seq
	inc := uint32(meta.PayloadLen)
	if meta.SYN || meta.FIN {
		inc++
	}
	state.lastRemoteInc = inc
	if meta.HasTS {
		state.lastRemoteTSVal = meta.TSVal
	}
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

func (h *SendHandle) getFlowState(dstIP net.IP, dstPort uint16) *flowState {
	key := hash.IPAddr(dstIP, dstPort)
	h.flowMu.RLock()
	st := h.flows[key]
	h.flowMu.RUnlock()
	if st != nil {
		return st
	}
	h.flowMu.Lock()
	defer h.flowMu.Unlock()
	if st = h.flows[key]; st != nil {
		return st
	}
	st = &flowState{}
	h.flows[key] = st
	return st
}

func (h *SendHandle) Close() {
	if h.handle != nil {
		h.handle.Close()
	}
}
