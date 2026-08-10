package socket

import (
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"

	"paqet/internal/conf"
)

type decoder struct {
	parser  *gopacket.DecodingLayerParser
	eth     layers.Ethernet
	ip4     layers.IPv4
	ip6     layers.IPv6
	tcp     layers.TCP
	decoded []gopacket.LayerType
}

type RecvHandle struct {
	handle *pcap.Handle
	dPool  sync.Pool
}

func NewRecvHandle(cfg *conf.Network) (*RecvHandle, error) {
	handle, err := newHandle(cfg, time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("failed to open pcap handle: %w", err)
	}

	// SetDirection is not fully supported on Windows Npcap, so skip it
	if runtime.GOOS != "windows" {
		if err := handle.SetDirection(pcap.DirectionIn); err != nil {
			return nil, fmt.Errorf("failed to set pcap direction in: %v", err)
		}
	}

	filter := fmt.Sprintf("tcp and dst port %d", cfg.Port)
	if err := handle.SetBPFFilter(filter); err != nil {
		return nil, fmt.Errorf("failed to set BPF filter: %w", err)
	}

	h := &RecvHandle{handle: handle}
	h.dPool.New = func() any {
		d := &decoder{decoded: make([]gopacket.LayerType, 0, 4)}
		d.parser = gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &d.eth, &d.ip4, &d.ip6, &d.tcp)
		d.parser.IgnoreUnsupported = true
		return d
	}

	return h, nil
}

func (h *RecvHandle) Read() ([]byte, net.Addr, error) {
	data, _, err := h.handle.ReadPacketData()
	if err != nil {
		return nil, nil, err
	}

	d := h.dPool.Get().(*decoder)
	defer h.dPool.Put(d)

	if err := d.parser.DecodeLayers(data, &d.decoded); err != nil {
		return nil, nil, errNoPayload
	}

	addr := &net.UDPAddr{}
	var payload []byte
	for _, t := range d.decoded {
		switch t {
		case layers.LayerTypeIPv4:
			addr.IP = d.ip4.SrcIP
		case layers.LayerTypeIPv6:
			addr.IP = d.ip6.SrcIP
		case layers.LayerTypeTCP:
			addr.Port = int(d.tcp.SrcPort)
			payload = d.tcp.Payload
		}
	}

	if addr.IP == nil || len(payload) == 0 {
		return nil, nil, errNoPayload
	}

	return payload, addr, nil
}

func (h *RecvHandle) Close() {
	if h.handle != nil {
		h.handle.Close()
	}
}
