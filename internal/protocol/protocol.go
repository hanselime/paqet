package protocol

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"paqet/internal/conf"
	"paqet/internal/tnet"
)

type PType = byte

const (
	PPING PType = 0x01
	PPONG PType = 0x02
	PTCPF PType = 0x03
	PTCP  PType = 0x04
	PUDP  PType = 0x05
)

const (
	wireMagic   uint16 = 0x5051 // "PQ"
	wireVersion byte   = 0x01
)

type Proto struct {
	Type PType
	Addr *tnet.Addr
	TCPF []conf.TCPF
}

func (p *Proto) Read(r io.Reader) error {
	br := bufio.NewReader(r)

	var head [7]byte
	if _, err := io.ReadFull(br, head[:]); err != nil {
		return err
	}
	if binary.BigEndian.Uint16(head[0:2]) != wireMagic {
		return fmt.Errorf("invalid protocol magic")
	}
	if head[2] != wireVersion {
		return fmt.Errorf("unsupported protocol version: %d", head[2])
	}

	p.Type = head[3]
	addrLen := binary.BigEndian.Uint16(head[4:6])
	tcpfCount := int(head[6])

	if addrLen > 0 {
		addrBytes := make([]byte, int(addrLen))
		if _, err := io.ReadFull(br, addrBytes); err != nil {
			return err
		}
		addr, err := tnet.NewAddr(string(addrBytes))
		if err != nil {
			return fmt.Errorf("invalid protocol address: %w", err)
		}
		p.Addr = addr
	} else {
		p.Addr = nil
	}

	if tcpfCount > 0 {
		flags := make([]byte, tcpfCount*2)
		if _, err := io.ReadFull(br, flags); err != nil {
			return err
		}
		p.TCPF = make([]conf.TCPF, tcpfCount)
		for i := 0; i < tcpfCount; i++ {
			raw := binary.BigEndian.Uint16(flags[i*2 : i*2+2])
			p.TCPF[i] = decodeTCPF(raw)
		}
	} else {
		p.TCPF = nil
	}

	return nil
}

func (p *Proto) Write(w io.Writer) error {
	bw := bufio.NewWriter(w)

	var addrBytes []byte
	if p.Addr != nil {
		addrBytes = []byte(p.Addr.String())
	}
	if len(addrBytes) > int(^uint16(0)) {
		return fmt.Errorf("protocol address too long")
	}
	if len(p.TCPF) > int(^byte(0)) {
		return fmt.Errorf("too many tcp flag entries")
	}

	var head [7]byte
	binary.BigEndian.PutUint16(head[0:2], wireMagic)
	head[2] = wireVersion
	head[3] = p.Type
	binary.BigEndian.PutUint16(head[4:6], uint16(len(addrBytes)))
	head[6] = byte(len(p.TCPF))

	if _, err := bw.Write(head[:]); err != nil {
		return err
	}
	if len(addrBytes) > 0 {
		if _, err := bw.Write(addrBytes); err != nil {
			return err
		}
	}
	if len(p.TCPF) > 0 {
		buf := make([]byte, len(p.TCPF)*2)
		for i, f := range p.TCPF {
			binary.BigEndian.PutUint16(buf[i*2:i*2+2], encodeTCPF(f))
		}
		if _, err := bw.Write(buf); err != nil {
			return err
		}
	}

	return bw.Flush()
}

func encodeTCPF(f conf.TCPF) uint16 {
	var b uint16
	if f.FIN {
		b |= 1 << 0
	}
	if f.SYN {
		b |= 1 << 1
	}
	if f.RST {
		b |= 1 << 2
	}
	if f.PSH {
		b |= 1 << 3
	}
	if f.ACK {
		b |= 1 << 4
	}
	if f.URG {
		b |= 1 << 5
	}
	if f.ECE {
		b |= 1 << 6
	}
	if f.CWR {
		b |= 1 << 7
	}
	if f.NS {
		b |= 1 << 8
	}
	return b
}

func decodeTCPF(b uint16) conf.TCPF {
	return conf.TCPF{
		FIN: b&(1<<0) != 0,
		SYN: b&(1<<1) != 0,
		RST: b&(1<<2) != 0,
		PSH: b&(1<<3) != 0,
		ACK: b&(1<<4) != 0,
		URG: b&(1<<5) != 0,
		ECE: b&(1<<6) != 0,
		CWR: b&(1<<7) != 0,
		NS:  b&(1<<8) != 0,
	}
}
