package socket

import (
	"net"
	"testing"

	"paqet/internal/conf"
)

func TestPacketConnLocalAddr(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *conf.Network
		wantPort int
		wantIP   net.IP
	}{
		{
			name: "IPv4 configured",
			cfg: &conf.Network{
				IPv4: conf.Addr{
					Addr: &net.UDPAddr{
						IP:   net.ParseIP("10.0.0.1"),
						Port: 9999,
					},
				},
				Port: 9999,
			},
			wantPort: 9999,
			wantIP:   net.ParseIP("10.0.0.1"),
		},
		{
			name: "IPv6 configured",
			cfg: &conf.Network{
				IPv6: conf.Addr{
					Addr: &net.UDPAddr{
						IP:   net.ParseIP("::1"),
						Port: 8888,
					},
				},
				Port: 8888,
			},
			wantPort: 8888,
			wantIP:   net.ParseIP("::1"),
		},
		{
			name: "No addresses configured - fallback",
			cfg: &conf.Network{
				Port: 7777,
			},
			wantPort: 7777,
			wantIP:   net.IPv4zero,
		},
		{
			name:     "Nil config - fallback",
			cfg:      nil,
			wantPort: 0,
			wantIP:   net.IPv4zero,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := &PacketConn{
				cfg: tt.cfg,
			}

			addr := pc.LocalAddr()
			if addr == nil {
				t.Fatal("LocalAddr() returned nil")
			}

			udpAddr, ok := addr.(*net.UDPAddr)
			if !ok {
				t.Fatalf("LocalAddr() returned %T, want *net.UDPAddr", addr)
			}

			if udpAddr.Port != tt.wantPort {
				t.Errorf("LocalAddr() port = %d, want %d", udpAddr.Port, tt.wantPort)
			}

			if !udpAddr.IP.Equal(tt.wantIP) {
				t.Errorf("LocalAddr() IP = %v, want %v", udpAddr.IP, tt.wantIP)
			}
		})
	}
}

func TestPacketConnLocalAddrNotNil(t *testing.T) {
	// Regression test for nil pointer dereference bug
	pc := &PacketConn{
		cfg: &conf.Network{
			Port: 1234,
		},
	}

	addr := pc.LocalAddr()
	if addr == nil {
		t.Error("LocalAddr() must not return nil")
	}
}
