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

	filter := fmt.Sprintf("tcp and dst port %d", cfg.Port)
	if err := handle.SetBPFFilter(filter); err != nil {
		return nil, fmt.Errorf("failed to set BPF filter: %w", err)
	}

	return &RecvHandle{handle: handle}, nil
}

func (h *RecvHandle) Read() ([]byte, net.Addr, error) {
	data, _, err := h.handle.ReadPacketData()
	if err != nil {
		return nil, nil, err
	}
	p := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.NoCopy)

	addr := &net.UDPAddr{}

	netLayer := p.NetworkLayer()
	if netLayer == nil {
		return nil, nil, nil
	}
	switch netLayer.LayerType() {
	case layers.LayerTypeIPv4:
		addr.IP = netLayer.(*layers.IPv4).SrcIP
	case layers.LayerTypeIPv6:
		addr.IP = netLayer.(*layers.IPv6).SrcIP
	}

	trLayer := p.TransportLayer()
	if trLayer == nil {
		return nil, nil, nil
	}
	switch trLayer.LayerType() {
	case layers.LayerTypeTCP:
		addr.Port = int(trLayer.(*layers.TCP).SrcPort)
	case layers.LayerTypeUDP:
		addr.Port = int(trLayer.(*layers.UDP).SrcPort)
	}

	appLayer := p.ApplicationLayer()
	if appLayer == nil {
		return nil, nil, nil
	}

	return appLayer.Payload(), addr, nil
}

func (h *RecvHandle) Close() {
	if h.handle != nil {
		h.handle.Close()
	}
}
