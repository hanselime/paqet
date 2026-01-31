package protocol

import (
	"encoding/gob"
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

type Proto struct {
	Type PType
	Addr *tnet.Addr
	TCPF []conf.TCPF
}

func (p *Proto) Read(r io.Reader) error {
	dec := gob.NewDecoder(r)

	err := dec.Decode(p)
	if err != nil {
		return err
	}
	return nil
}

func (p *Proto) Write(w io.Writer) error {
	enc := gob.NewEncoder(w)

	err := enc.Encode(p)
	if err != nil {
		return err
	}

	return nil
}
