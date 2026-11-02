package conf

import (
	"fmt"
)

type TCP struct {
	LF_ []string `yaml:"local_flag"`
	RF_ []string `yaml:"remote_flag"`
	// Handshake enables the fake 3-way handshake (SYN-prime) that opens
	// stateful NAT/firewalls before KCP data flows. Nil (key absent)
	// means enabled, so pre-existing configs keep working unchanged.
	// Set explicitly to false only for peers whose middlebox or old
	// counterpart mishandles the magic packets.
	Handshake *bool  `yaml:"handshake"`
	LF        []TCPF `yaml:"-"`
	RF        []TCPF `yaml:"-"`
}

// HandshakeEnabled reports whether the fake 3-way handshake should run.
// Default is true; old configs without the key are unaffected.
func (t *TCP) HandshakeEnabled() bool {
	return t.Handshake == nil || *t.Handshake
}

type TCPF struct {
	FIN, SYN, RST, PSH, ACK, URG, ECE, CWR, NS bool
}

func (t *TCP) setDefaults() {
	if len(t.LF_) == 0 {
		t.LF_ = []string{"PA"}
	}
	if len(t.RF_) == 0 {
		t.RF_ = []string{"PA"}
	}
}

func (t *TCP) validate() []error {
	var errors []error

	if len(t.LF_) != 0 {
		t.LF = make([]TCPF, len(t.LF_))
		for i, fStr := range t.LF_ {
			f, err := strTCPF(fStr)
			if err != nil {
				errors = append(errors, err)
			}
			t.LF[i] = f
		}
	}
	if len(t.RF_) != 0 {
		t.RF = make([]TCPF, len(t.RF_))
		for i, fStr := range t.RF_ {
			f, err := strTCPF(fStr)
			if err != nil {
				errors = append(errors, err)
			}
			t.RF[i] = f
		}
	}

	if len(t.LF) == 0 || len(t.RF) == 0 {
		errors = append(errors, fmt.Errorf("at least one TCP flag combination required"))
	}

	maxTCPFLen := 64
	if len(t.LF_) > maxTCPFLen {
		errors = append(errors, fmt.Errorf("local_flag exceeds max %d", maxTCPFLen))
	}
	if len(t.RF_) > maxTCPFLen {
		errors = append(errors, fmt.Errorf("remote_flag exceeds max %d", maxTCPFLen))
	}

	return errors
}

func strTCPF(fStr string) (TCPF, error) {
	var f TCPF
	for _, ch := range fStr {
		switch ch {
		case 'F':
			f.FIN = true
		case 'S':
			f.SYN = true
		case 'R':
			f.RST = true
		case 'P':
			f.PSH = true
		case 'A':
			f.ACK = true
		case 'U':
			f.URG = true
		case 'E':
			f.ECE = true
		case 'C':
			f.CWR = true
		case 'N':
			f.NS = true
		default:
			return f, fmt.Errorf("invalid TCP flag '%c' in combination", ch)
		}
	}
	return f, nil
}
