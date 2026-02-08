package conf

import (
	"testing"
)

// TestPCAPConfigValidation tests the PCAP configuration validation
func TestPCAPConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		pcap    PCAP
		wantErr bool
	}{
		{
			name: "valid config",
			pcap: PCAP{
				Sockbuf:        4 * 1024 * 1024,
				SendQueueSize:  1000,
				MaxRetries:     3,
				InitialBackoff: 10,
				MaxBackoff:     1000,
			},
			wantErr: false,
		},
		{
			name: "queue size too small",
			pcap: PCAP{
				Sockbuf:        4 * 1024 * 1024,
				SendQueueSize:  0,
				MaxRetries:     3,
				InitialBackoff: 10,
				MaxBackoff:     1000,
			},
			wantErr: true,
		},
		{
			name: "queue size too large",
			pcap: PCAP{
				Sockbuf:        4 * 1024 * 1024,
				SendQueueSize:  200000,
				MaxRetries:     3,
				InitialBackoff: 10,
				MaxBackoff:     1000,
			},
			wantErr: true,
		},
		{
			name: "max retries too large",
			pcap: PCAP{
				Sockbuf:        4 * 1024 * 1024,
				SendQueueSize:  1000,
				MaxRetries:     20,
				InitialBackoff: 10,
				MaxBackoff:     1000,
			},
			wantErr: true,
		},
		{
			name: "max retries negative",
			pcap: PCAP{
				Sockbuf:        4 * 1024 * 1024,
				SendQueueSize:  1000,
				MaxRetries:     -1,
				InitialBackoff: 10,
				MaxBackoff:     1000,
			},
			wantErr: true,
		},
		{
			name: "max backoff smaller than initial",
			pcap: PCAP{
				Sockbuf:        4 * 1024 * 1024,
				SendQueueSize:  1000,
				MaxRetries:     3,
				InitialBackoff: 100,
				MaxBackoff:     10,
			},
			wantErr: true,
		},
		{
			name: "initial backoff too small",
			pcap: PCAP{
				Sockbuf:        4 * 1024 * 1024,
				SendQueueSize:  1000,
				MaxRetries:     3,
				InitialBackoff: 0,
				MaxBackoff:     1000,
			},
			wantErr: true,
		},
		{
			name: "max backoff too large",
			pcap: PCAP{
				Sockbuf:        4 * 1024 * 1024,
				SendQueueSize:  1000,
				MaxRetries:     3,
				InitialBackoff: 10,
				MaxBackoff:     100000,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.pcap.validate()
			hasErr := len(errs) > 0
			if hasErr != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v, errors: %v", hasErr, tt.wantErr, errs)
			}
		})
	}
}

// TestPCAPSetDefaults tests the PCAP default values
func TestPCAPSetDefaults(t *testing.T) {
	tests := []struct {
		name               string
		role               string
		initial            PCAP
		expectedSockbuf    int
		expectedQueueSize  int
		expectedRetries    int
		expectedInitBackoff int
		expectedMaxBackoff int
	}{
		{
			name:                "server defaults",
			role:                "server",
			initial:             PCAP{},
			expectedSockbuf:     8 * 1024 * 1024,
			expectedQueueSize:   1000,
			expectedRetries:     3,
			expectedInitBackoff: 10,
			expectedMaxBackoff:  1000,
		},
		{
			name:                "client defaults",
			role:                "client",
			initial:             PCAP{},
			expectedSockbuf:     4 * 1024 * 1024,
			expectedQueueSize:   1000,
			expectedRetries:     3,
			expectedInitBackoff: 10,
			expectedMaxBackoff:  1000,
		},
		{
			name: "custom values preserved",
			role: "client",
			initial: PCAP{
				Sockbuf:        16 * 1024 * 1024,
				SendQueueSize:  5000,
				MaxRetries:     5,
				InitialBackoff: 50,
				MaxBackoff:     5000,
			},
			expectedSockbuf:     16 * 1024 * 1024,
			expectedQueueSize:   5000,
			expectedRetries:     5,
			expectedInitBackoff: 50,
			expectedMaxBackoff:  5000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pcap := tt.initial
			pcap.setDefaults(tt.role)

			if pcap.Sockbuf != tt.expectedSockbuf {
				t.Errorf("Sockbuf = %v, want %v", pcap.Sockbuf, tt.expectedSockbuf)
			}
			if pcap.SendQueueSize != tt.expectedQueueSize {
				t.Errorf("SendQueueSize = %v, want %v", pcap.SendQueueSize, tt.expectedQueueSize)
			}
			if pcap.MaxRetries != tt.expectedRetries {
				t.Errorf("MaxRetries = %v, want %v", pcap.MaxRetries, tt.expectedRetries)
			}
			if pcap.InitialBackoff != tt.expectedInitBackoff {
				t.Errorf("InitialBackoff = %v, want %v", pcap.InitialBackoff, tt.expectedInitBackoff)
			}
			if pcap.MaxBackoff != tt.expectedMaxBackoff {
				t.Errorf("MaxBackoff = %v, want %v", pcap.MaxBackoff, tt.expectedMaxBackoff)
			}
		})
	}
}
