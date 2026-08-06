package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"paqet/internal/conf"
	"paqet/internal/tnet"
)

type PType = byte

const (
	MAGIC   byte = 0x50
	VERSION byte = 0x01

	PPING PType = 0x01
	PPONG PType = 0x02
	PTCPF PType = 0x03
	PTCP  PType = 0x04
	PUDP  PType = 0x05
)

const (
	headerLen    = 5 // MAGIC, VERSION, TYPE, LENGTH(2)
	maxHostLen   = 253
	maxTCPFCount = 64
	maxBodyLen   = 4096
	maxPort      = 0xFFFF
)

const (
	bFIN = 1 << 0
	bSYN = 1 << 1
	bRST = 1 << 2
	bPSH = 1 << 3
	bACK = 1 << 4
	bURG = 1 << 5
	bECE = 1 << 6
	bCWR = 1 << 7
	bNS  = 1 << 8
)

type Proto struct {
	Type PType
	Addr *tnet.Addr
	TCPF []conf.TCPF
}

func encodeTCPF(f conf.TCPF) uint16 {
	var v uint16
	if f.FIN {
		v |= bFIN
	}
	if f.SYN {
		v |= bSYN
	}
	if f.RST {
		v |= bRST
	}
	if f.PSH {
		v |= bPSH
	}
	if f.ACK {
		v |= bACK
	}
	if f.URG {
		v |= bURG
	}
	if f.ECE {
		v |= bECE
	}
	if f.CWR {
		v |= bCWR
	}
	if f.NS {
		v |= bNS
	}
	return v
}

func decodeTCPF(v uint16) conf.TCPF {
	return conf.TCPF{
		FIN: v&bFIN != 0, SYN: v&bSYN != 0, RST: v&bRST != 0,
		PSH: v&bPSH != 0, ACK: v&bACK != 0, URG: v&bURG != 0,
		ECE: v&bECE != 0, CWR: v&bCWR != 0, NS: v&bNS != 0,
	}
}

func readFull(r io.Reader, n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (p *Proto) Write(w io.Writer) error {
	body := make([]byte, 0, 64)

	switch p.Type {
	case PPING, PPONG:
		// no body

	case PTCP, PUDP:
		if p.Addr == nil {
			return errors.New("protocol: address required")
		}
		host := []byte(p.Addr.Host)
		if len(host) > maxHostLen {
			return fmt.Errorf("protocol: host length %d exceeds max %d", len(host), maxHostLen)
		}
		if p.Addr.Port < 0 || p.Addr.Port > maxPort {
			return fmt.Errorf("protocol: port %d out of range", p.Addr.Port)
		}
		body = append(body, byte(len(host)))
		body = append(body, host...)
		body = binary.BigEndian.AppendUint16(body, uint16(p.Addr.Port))

	case PTCPF:
		if len(p.TCPF) > maxTCPFCount {
			return fmt.Errorf("protocol: tcpf count %d exceeds max %d", len(p.TCPF), maxTCPFCount)
		}
		body = append(body, byte(len(p.TCPF)))
		for _, f := range p.TCPF {
			body = binary.BigEndian.AppendUint16(body, encodeTCPF(f))
		}

	default:
		return errors.New("protocol: unknown message type")
	}

	if len(body) > maxBodyLen {
		return fmt.Errorf("protocol: body length %d exceeds max %d", len(body), maxBodyLen)
	}

	buf := make([]byte, 0, headerLen+len(body))
	buf = append(buf, MAGIC, VERSION, p.Type)
	buf = binary.BigEndian.AppendUint16(buf, uint16(len(body)))
	buf = append(buf, body...)

	_, err := w.Write(buf)
	return err
}

func (p *Proto) Read(r io.Reader) error {
	hdr, err := readFull(r, headerLen)
	if err != nil {
		return err
	}
	if hdr[0] != MAGIC {
		return fmt.Errorf("protocol: bad magic byte 0x%02x (want 0x%02x)", hdr[0], MAGIC)
	}
	if hdr[1] != VERSION {
		return fmt.Errorf("protocol: unsupported version 0x%02x (want 0x%02x)", hdr[1], VERSION)
	}
	p.Type = hdr[2]
	p.Addr, p.TCPF = nil, nil

	n := int(binary.BigEndian.Uint16(hdr[3:]))
	if n > maxBodyLen {
		return fmt.Errorf("protocol: body length %d exceeds max %d", n, maxBodyLen)
	}
	body, err := readFull(r, n)
	if err != nil {
		return err
	}

	switch p.Type {
	case PPING, PPONG:
		return nil

	case PTCP, PUDP:
		if len(body) < 3 {
			return errors.New("protocol: truncated address body")
		}
		hl := int(body[0])
		if hl > maxHostLen || 1+hl+2 != len(body) {
			return fmt.Errorf("protocol: bad host length %d", hl)
		}
		host := string(body[1 : 1+hl])
		port := int(binary.BigEndian.Uint16(body[1+hl:]))
		p.Addr = &tnet.Addr{Host: host, Port: port}
		return nil

	case PTCPF:
		if len(body) < 1 {
			return errors.New("protocol: truncated tcpf body")
		}
		c := int(body[0])
		if c > maxTCPFCount || 1+c*2 != len(body) {
			return fmt.Errorf("protocol: bad tcpf count %d", c)
		}
		p.TCPF = make([]conf.TCPF, c)
		for i := range p.TCPF {
			p.TCPF[i] = decodeTCPF(binary.BigEndian.Uint16(body[1+i*2:]))
		}
		return nil

	default:
		return errors.New("protocol: unknown message type")
	}
}
