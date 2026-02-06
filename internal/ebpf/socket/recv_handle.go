package socket

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"paqet/internal/conf"
)

type RecvHandle struct {
	conn *net.UDPConn
	port uint32
}

func NewRecvHandle(cfg *conf.Network) (*RecvHandle, error) {
	if err := InitBPFHandle(cfg); err != nil {
		return nil, fmt.Errorf("failed to init bpf handle: %w", err)
	}

	if cfg.Port == 0 {
		cfg.Port = 32768 + rand.Intn(32768)
	}

	port := uint32(cfg.Port)
	if err := updateTargetPorts(port, 1); err != nil {
		return nil, fmt.Errorf("failed to update port in map: %w", err)
	}

	// TODO
	laddr := &net.UDPAddr{
		IP:   net.ParseIP("0.0.0.0"),
		Port: cfg.Port,
		Zone: "",
	}

	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s:%d: %v", laddr.IP, laddr.Port, err)
	}

	return &RecvHandle{conn: conn, port: port}, nil
}

func (h *RecvHandle) Read() ([]byte, net.Addr, error) {
	data := make([]byte, 65535)
	n, addr, err := h.conn.ReadFromUDP(data)
	if err != nil {
		return nil, nil, err
	}

	offset := int(binary.BigEndian.Uint16(data[:2]))

	return data[offset:n], addr, nil
}

func (h *RecvHandle) Close() {
	if h.conn != nil {
		h.conn.Close()
	}
	// reset tcp2udp for this port
	updateTargetPorts(h.port, 0)
}
