package conf

import (
	"fmt"
	"paqet/internal/flog"
)

type PCAP struct {
	Sockbuf       int `yaml:"sockbuf"`
	SendQueueSize int `yaml:"send_queue_size"`
	MaxRetries    int `yaml:"max_retries"`
	InitialBackoff int `yaml:"initial_backoff_ms"`
	MaxBackoff     int `yaml:"max_backoff_ms"`
}

func (p *PCAP) setDefaults(role string) {
	if p.Sockbuf == 0 {
		if role == "server" {
			p.Sockbuf = 8 * 1024 * 1024
		} else {
			p.Sockbuf = 4 * 1024 * 1024
		}
	}
	if p.SendQueueSize == 0 {
		p.SendQueueSize = 1000
	}
	if p.MaxRetries == 0 {
		p.MaxRetries = 3
	}
	if p.InitialBackoff == 0 {
		p.InitialBackoff = 10 // 10ms
	}
	if p.MaxBackoff == 0 {
		p.MaxBackoff = 1000 // 1s
	}
}

func (p *PCAP) validate() []error {
	var errors []error

	if p.Sockbuf < 1024 {
		errors = append(errors, fmt.Errorf("PCAP sockbuf must be >= 1024 bytes"))
	}

	if p.Sockbuf > 100*1024*1024 {
		errors = append(errors, fmt.Errorf("PCAP sockbuf too large (max 100MB)"))
	}

	// Should be power of 2 for optimal performance, but not required
	if p.Sockbuf&(p.Sockbuf-1) != 0 {
		flog.Warnf("PCAP sockbuf (%d bytes) is not a power of 2 - consider using values like 4MB, 8MB, or 16MB for better performance", p.Sockbuf)
	}

	if p.SendQueueSize < 1 || p.SendQueueSize > 100000 {
		errors = append(errors, fmt.Errorf("PCAP send_queue_size must be between 1 and 100000"))
	}

	if p.MaxRetries < 0 || p.MaxRetries > 10 {
		errors = append(errors, fmt.Errorf("PCAP max_retries must be between 0 and 10"))
	}

	if p.InitialBackoff < 1 || p.InitialBackoff > 1000 {
		errors = append(errors, fmt.Errorf("PCAP initial_backoff_ms must be between 1 and 1000"))
	}

	if p.MaxBackoff < p.InitialBackoff || p.MaxBackoff > 60000 {
		errors = append(errors, fmt.Errorf("PCAP max_backoff_ms must be between initial_backoff_ms and 60000"))
	}

	return errors
}
