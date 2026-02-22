package socket

import (
	"fmt"
	"net"
	"paqet/internal/conf"
	"runtime"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
)

type RecvHandle struct {
	handle *pcap.Handle
	eth    layers.Ethernet
	ipv4   layers.IPv4
	ipv6   layers.IPv6
	tcp    layers.TCP
	udp    layers.UDP
	parser *gopacket.DecodingLayerParser
}

func NewRecvHandle(cfg *conf.Network) (*RecvHandle, error) {
	handle, err := newHandle(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to open pcap handle: %w", err)
	}

	// SetDirection is not fully supported on Windows Npcap, so skip it
	if runtime.GOOS != "windows" {
		if err := handle.SetDirection(pcap.DirectionIn); err != nil {
			return nil, fmt.Errorf("failed to set pcap direction in: %v", err)
		}
	}

	h := &RecvHandle{handle: handle}
	h.parser = gopacket.NewDecodingLayerParser(layers.LayerTypeEthernet, &h.eth, &h.ipv4, &h.ipv6, &h.tcp, &h.udp)

	filter := fmt.Sprintf("tcp and dst port %d", cfg.Port)
	if err := handle.SetBPFFilter(filter); err != nil {
		return nil, fmt.Errorf("failed to set BPF filter: %w", err)
	}

	return h, nil
}

func (h *RecvHandle) Read() ([]byte, net.Addr, error) {
	data, _, err := h.handle.ReadPacketData()
	if err != nil {
		return nil, nil, err
	}

	decodedLayers := make([]gopacket.LayerType, 0, 4)
	err = h.parser.DecodeLayers(data, &decodedLayers)
	if err != nil {
		return nil, nil, nil // Ignore malformed packets
	}

	addr := &net.UDPAddr{}
	var payload []byte

	for _, layerType := range decodedLayers {
		switch layerType {
		case layers.LayerTypeIPv4:
			addr.IP = h.ipv4.SrcIP
		case layers.LayerTypeIPv6:
			addr.IP = h.ipv6.SrcIP
		case layers.LayerTypeTCP:
			addr.Port = int(h.tcp.SrcPort)
			payload = h.tcp.Payload
		case layers.LayerTypeUDP:
			addr.Port = int(h.udp.SrcPort)
			payload = h.udp.Payload
		}
	}

	return payload, addr, nil
}

func (h *RecvHandle) ReadTo(data []byte) (int, net.Addr, error) {
	for {
		payload, addr, err := h.Read()
		if err != nil {
			return 0, nil, err
		}
		if payload != nil {
			n := copy(data, payload)
			return n, addr, nil
		}
	}
}

func (h *RecvHandle) Close() {
	if h.handle != nil {
		h.handle.Close()
	}
}
