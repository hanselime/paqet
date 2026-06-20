package socks

import (
	"errors"
	"io"
	"net"
	"strconv"
)

const (
	ver     = 0x05
	verAuth = 0x01

	methodNone = 0x00
	methodAuth = 0x02
	methodNA   = 0xff

	cmdConnect = 0x01
	cmdUDP     = 0x03

	atypIPv4   = 0x01
	atypDomain = 0x03
	atypIPv6   = 0x04

	repSuccess   = 0x00
	repFailure   = 0x01
	repCmdUnsupp = 0x07
)

var errProtocol = errors.New("socks: protocol error")

type request struct {
	cmd  byte
	atyp byte
	addr []byte
	port []byte
}

func (r *request) address() string { return addrString(r.atyp, r.addr, r.port) }

func addrString(atyp byte, addr, port []byte) string {
	var host string
	switch atyp {
	case atypIPv4, atypIPv6:
		host = net.IP(addr).String()
	default:
		host = string(addr)
	}
	p := int(port[0])<<8 | int(port[1])
	return net.JoinHostPort(host, strconv.Itoa(p))
}

func readAddr(r io.Reader) (atyp byte, addr, port []byte, err error) {
	var b [1]byte
	if _, err = io.ReadFull(r, b[:]); err != nil {
		return
	}
	atyp = b[0]
	switch atyp {
	case atypIPv4:
		addr = make([]byte, net.IPv4len)
	case atypIPv6:
		addr = make([]byte, net.IPv6len)
	case atypDomain:
		if _, err = io.ReadFull(r, b[:]); err != nil {
			return
		}
		addr = make([]byte, b[0])
	default:
		return atyp, nil, nil, errProtocol
	}
	if _, err = io.ReadFull(r, addr); err != nil {
		return
	}
	port = make([]byte, 2)
	_, err = io.ReadFull(r, port)
	return
}

func putAddr(buf []byte, ip net.IP, port int) []byte {
	if ip4 := ip.To4(); ip4 != nil {
		buf = append(buf, atypIPv4)
		buf = append(buf, ip4...)
	} else if ip16 := ip.To16(); ip16 != nil {
		buf = append(buf, atypIPv6)
		buf = append(buf, ip16...)
	} else {
		host := ip.String()
		buf = append(buf, atypDomain, byte(len(host)))
		buf = append(buf, host...)
	}
	return append(buf, byte(port>>8), byte(port))
}

type datagram struct {
	atyp byte
	addr []byte
	port []byte
	data []byte
}

func (d *datagram) address() string {
	return addrString(d.atyp, d.addr, d.port)
}

func (d *datagram) bytes() []byte {
	buf := make([]byte, 0, 5+len(d.addr)+2+len(d.data))
	buf = append(buf, 0, 0, 0, d.atyp) // RSV(2) + FRAG(1) + ATYP
	if d.atyp == atypDomain {
		buf = append(buf, byte(len(d.addr)))
	}
	buf = append(buf, d.addr...)
	buf = append(buf, d.port...)
	return append(buf, d.data...)
}

func decodeDatagram(p []byte) (*datagram, error) {
	if len(p) < 4 || p[2] != 0 {
		return nil, errProtocol
	}
	atyp, rest := p[3], p[4:]
	var addr []byte
	switch atyp {
	case atypIPv4:
		if len(rest) < net.IPv4len+2 {
			return nil, errProtocol
		}
		addr, rest = rest[:net.IPv4len], rest[net.IPv4len:]
	case atypIPv6:
		if len(rest) < net.IPv6len+2 {
			return nil, errProtocol
		}
		addr, rest = rest[:net.IPv6len], rest[net.IPv6len:]
	case atypDomain:
		if len(rest) < 1 || len(rest) < 1+int(rest[0])+2 {
			return nil, errProtocol
		}
		n := int(rest[0])
		addr, rest = rest[1:1+n], rest[1+n:]
	default:
		return nil, errProtocol
	}
	return &datagram{atyp: atyp, addr: addr, port: rest[:2], data: rest[2:]}, nil
}
