package socket

import (
	"encoding/binary"
	"fmt"
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

type IP struct {
	srcIPv4     net.IP
	srcIPv4RHWA net.HardwareAddr
	srcIPv6     net.IP
	srcIPv6RHWA net.HardwareAddr
	protocol    int
}

type tcpF struct {
	tcpF       iterator.Iterator[conf.TCPF]
	clientTCPF map[uint64]*iterator.Iterator[conf.TCPF]
	mu         sync.RWMutex
}

type TCP struct {
	srcPort    uint16
	synOptions []layers.TCPOption
	ackOptions []layers.TCPOption
	time       uint32
	tsCounter  uint32
	tcpF       *tcpF
}

type Pool struct {
	ethPool  sync.Pool
	ipv4Pool sync.Pool
	ipv6Pool sync.Pool
	tcpPool  sync.Pool
	bufPool  sync.Pool
}

type SendHandle struct {
	handle *pcap.Handle
	ip     *IP
	tcp    *TCP
	pool   *Pool
	Write  func(payload []byte, addr *net.UDPAddr) error
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

	ip := &IP{protocol: cfg.IP.Protocol}
	if cfg.IPv4.Addr != nil {
		ip.srcIPv4 = cfg.IPv4.Addr.IP
		ip.srcIPv4RHWA = cfg.IPv4.Router
	}
	if cfg.IPv6.Addr != nil {
		ip.srcIPv6 = cfg.IPv6.Addr.IP
		ip.srcIPv6RHWA = cfg.IPv6.Router
	}

	tcpf := &tcpF{
		tcpF:       iterator.Iterator[conf.TCPF]{Items: cfg.TCP.LF},
		clientTCPF: make(map[uint64]*iterator.Iterator[conf.TCPF]),
	}
	tcp := &TCP{
		srcPort: uint16(cfg.LPort),
		time:    uint32(time.Now().UnixNano() / int64(time.Millisecond)),
		tcpF:    tcpf,
	}
	tcp.synOptions = []layers.TCPOption{
		{OptionType: layers.TCPOptionKindMSS, OptionLength: 4, OptionData: []byte{0x05, 0xb4}},
		{OptionType: layers.TCPOptionKindSACKPermitted, OptionLength: 2},
		{OptionType: layers.TCPOptionKindTimestamps, OptionLength: 10, OptionData: make([]byte, 8)},
		{OptionType: layers.TCPOptionKindNop},
		{OptionType: layers.TCPOptionKindWindowScale, OptionLength: 3, OptionData: []byte{8}},
	}
	tcp.ackOptions = []layers.TCPOption{
		{OptionType: layers.TCPOptionKindNop},
		{OptionType: layers.TCPOptionKindNop},
		{OptionType: layers.TCPOptionKindTimestamps, OptionLength: 10, OptionData: make([]byte, 8)},
	}

	pool := &Pool{
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

	sh := &SendHandle{
		handle: handle,
		ip:     ip,
		tcp:    tcp,
		pool:   pool,
	}

	switch cfg.IP.Protocol {
	case 6:
		sh.Write = sh.WriteTCP
	default:
		sh.Write = sh.WriteIP
	}

	return sh, nil
}

func (h *SendHandle) buildIPv4Header(dstIP net.IP, protocol int) *layers.IPv4 {
	ip := h.pool.ipv4Pool.Get().(*layers.IPv4)
	*ip = layers.IPv4{
		Version:  4,
		IHL:      5,
		TOS:      184,
		TTL:      128,
		Flags:    layers.IPv4DontFragment,
		Protocol: layers.IPProtocol(protocol),
		SrcIP:    h.ip.srcIPv4,
		DstIP:    dstIP,
	}
	return ip
}

func (h *SendHandle) buildIPv6Header(dstIP net.IP, protocol int) *layers.IPv6 {
	ip := h.pool.ipv6Pool.Get().(*layers.IPv6)
	*ip = layers.IPv6{
		Version:      6,
		TrafficClass: 184,
		HopLimit:     128,
		NextHeader:   layers.IPProtocol(protocol),
		SrcIP:        h.ip.srcIPv6,
		DstIP:        dstIP,
	}
	return ip
}

func (h *SendHandle) buildTCPHeader(dstPort uint16, f conf.TCPF) *layers.TCP {
	tcp := h.pool.tcpPool.Get().(*layers.TCP)
	*tcp = layers.TCP{
		SrcPort: layers.TCPPort(h.tcp.srcPort),
		DstPort: layers.TCPPort(dstPort),
		FIN:     f.FIN, SYN: f.SYN, RST: f.RST, PSH: f.PSH, ACK: f.ACK, URG: f.URG, ECE: f.ECE, CWR: f.CWR, NS: f.NS,
		Window: 65535,
	}

	counter := atomic.AddUint32(&h.tcp.tsCounter, 1)
	tsVal := h.tcp.time + (counter >> 3)
	if f.SYN {
		binary.BigEndian.PutUint32(h.tcp.synOptions[2].OptionData[0:4], tsVal)
		binary.BigEndian.PutUint32(h.tcp.synOptions[2].OptionData[4:8], 0)
		tcp.Options = h.tcp.synOptions
		tcp.Seq = 1 + (counter & 0x7)
		tcp.Ack = 0
		if f.ACK {
			tcp.Ack = tcp.Seq + 1
		}
	} else {
		tsEcr := tsVal - (counter%200 + 50)
		binary.BigEndian.PutUint32(h.tcp.ackOptions[2].OptionData[0:4], tsVal)
		binary.BigEndian.PutUint32(h.tcp.ackOptions[2].OptionData[4:8], tsEcr)
		tcp.Options = h.tcp.ackOptions
		seq := h.tcp.time + (counter << 7)
		tcp.Seq = seq
		tcp.Ack = seq - (counter & 0x3FF) + 1400
	}

	return tcp
}

func (h *SendHandle) WriteIP(payload []byte, addr *net.UDPAddr) error {
	buf := h.pool.bufPool.Get().(gopacket.SerializeBuffer)
	ethLayer := h.pool.ethPool.Get().(*layers.Ethernet)
	defer func() {
		buf.Clear()
		h.pool.bufPool.Put(buf)
		h.pool.ethPool.Put(ethLayer)
	}()

	dstIP := addr.IP

	var ipLayer gopacket.SerializableLayer
	if dstIP.To4() != nil {
		ip := h.buildIPv4Header(dstIP, h.ip.protocol)
		defer h.pool.ipv4Pool.Put(ip)
		ipLayer = ip
		ethLayer.DstMAC = h.ip.srcIPv4RHWA
		ethLayer.EthernetType = layers.EthernetTypeIPv4
	} else {
		ip := h.buildIPv6Header(dstIP, h.ip.protocol)
		defer h.pool.ipv6Pool.Put(ip)
		ipLayer = ip
		ethLayer.DstMAC = h.ip.srcIPv6RHWA
		ethLayer.EthernetType = layers.EthernetTypeIPv6
	}

	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, ethLayer, ipLayer, gopacket.Payload(payload)); err != nil {
		return err
	}
	return h.handle.WritePacketData(buf.Bytes())

}

func (h *SendHandle) WriteTCP(payload []byte, addr *net.UDPAddr) error {
	buf := h.pool.bufPool.Get().(gopacket.SerializeBuffer)
	ethLayer := h.pool.ethPool.Get().(*layers.Ethernet)
	defer func() {
		buf.Clear()
		h.pool.bufPool.Put(buf)
		h.pool.ethPool.Put(ethLayer)
	}()

	dstIP := addr.IP
	dstPort := uint16(addr.Port)

	f := h.getClientTCPF(dstIP, dstPort)
	tcpLayer := h.buildTCPHeader(dstPort, f)
	defer h.pool.tcpPool.Put(tcpLayer)

	var ipLayer gopacket.SerializableLayer
	if dstIP.To4() != nil {
		ip := h.buildIPv4Header(dstIP, h.ip.protocol)
		defer h.pool.ipv4Pool.Put(ip)
		ipLayer = ip
		tcpLayer.SetNetworkLayerForChecksum(ip)
		ethLayer.DstMAC = h.ip.srcIPv4RHWA
		ethLayer.EthernetType = layers.EthernetTypeIPv4
	} else {
		ip := h.buildIPv6Header(dstIP, h.ip.protocol)
		defer h.pool.ipv6Pool.Put(ip)
		ipLayer = ip
		tcpLayer.SetNetworkLayerForChecksum(ip)
		ethLayer.DstMAC = h.ip.srcIPv6RHWA
		ethLayer.EthernetType = layers.EthernetTypeIPv6
	}

	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	if err := gopacket.SerializeLayers(buf, opts, ethLayer, ipLayer, tcpLayer, gopacket.Payload(payload)); err != nil {
		return err
	}
	return h.handle.WritePacketData(buf.Bytes())
}

func (h *SendHandle) getClientTCPF(dstIP net.IP, dstPort uint16) conf.TCPF {
	h.tcp.tcpF.mu.RLock()
	defer h.tcp.tcpF.mu.RUnlock()
	if ff := h.tcp.tcpF.clientTCPF[hash.IPAddr(dstIP, dstPort)]; ff != nil {
		return ff.Next()
	}
	return h.tcp.tcpF.tcpF.Next()
}

func (h *SendHandle) setClientTCPF(addr net.Addr, f []conf.TCPF) {
	a := *addr.(*net.UDPAddr)
	h.tcp.tcpF.mu.Lock()
	h.tcp.tcpF.clientTCPF[hash.IPAddr(a.IP, uint16(a.Port))] = &iterator.Iterator[conf.TCPF]{Items: f}
	h.tcp.tcpF.mu.Unlock()
}

func (h *SendHandle) Close() {
	if h.handle != nil {
		h.handle.Close()
	}
}
