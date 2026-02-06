package socket

import (
	"encoding/binary"
	"fmt"
	"net"
	"paqet/internal/conf"
	"runtime"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
)

type TCPMeta struct {
	SrcIP      net.IP
	DstIP      net.IP
	SrcPort    uint16
	DstPort    uint16
	Seq        uint32
	Ack        uint32
	SYN        bool
	FIN        bool
	RST        bool
	PSH        bool
	ACK        bool
	PayloadLen int
	TSVal      uint32
	HasTS      bool
}

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

func (h *RecvHandle) Read() ([]byte, net.Addr, *TCPMeta, error) {
	data, _, err := h.handle.ZeroCopyReadPacketData()
	if err != nil {
		return nil, nil, nil, err
	}

	addr := &net.UDPAddr{}
	meta := &TCPMeta{}
	p := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.NoCopy)

	netLayer := p.NetworkLayer()
	if netLayer == nil {
		return nil, addr, nil, nil
	}
	switch netLayer.LayerType() {
	case layers.LayerTypeIPv4:
		ipv4 := netLayer.(*layers.IPv4)
		addr.IP = ipv4.SrcIP
		meta.SrcIP = ipv4.SrcIP
		meta.DstIP = ipv4.DstIP
	case layers.LayerTypeIPv6:
		ipv6 := netLayer.(*layers.IPv6)
		addr.IP = ipv6.SrcIP
		meta.SrcIP = ipv6.SrcIP
		meta.DstIP = ipv6.DstIP
	}

	trLayer := p.TransportLayer()
	if trLayer == nil {
		return nil, addr, nil, nil
	}
	switch trLayer.LayerType() {
	case layers.LayerTypeTCP:
		tcp := trLayer.(*layers.TCP)
		addr.Port = int(tcp.SrcPort)
		meta.SrcPort = uint16(tcp.SrcPort)
		meta.DstPort = uint16(tcp.DstPort)
		meta.Seq = tcp.Seq
		meta.Ack = tcp.Ack
		meta.SYN = tcp.SYN
		meta.FIN = tcp.FIN
		meta.RST = tcp.RST
		meta.PSH = tcp.PSH
		meta.ACK = tcp.ACK
		for _, opt := range tcp.Options {
			if opt.OptionType == layers.TCPOptionKindTimestamps && len(opt.OptionData) >= 8 {
				meta.TSVal = binary.BigEndian.Uint32(opt.OptionData[0:4])
				meta.HasTS = true
				break
			}
		}
	case layers.LayerTypeUDP:
		addr.Port = int(trLayer.(*layers.UDP).SrcPort)
	}

	appLayer := p.ApplicationLayer()
	if appLayer == nil {
		return nil, addr, meta, nil
	}
	payload := appLayer.Payload()
	meta.PayloadLen = len(payload)
	return payload, addr, meta, nil
}

func (h *RecvHandle) Close() {
	if h.handle != nil {
		h.handle.Close()
	}
}
