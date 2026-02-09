package conf

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

type QUIC struct {
	// Connection settings
	MaxIdleTimeout       int  `yaml:"max_idle_timeout"`        // Maximum idle timeout in seconds (default: 30)
	MaxIncomingStreams   int  `yaml:"max_incoming_streams"`    // Maximum number of concurrent incoming streams (default: 1000)
	MaxIncomingUniStreams int `yaml:"max_incoming_uni_streams"` // Maximum number of concurrent incoming unidirectional streams (default: 1000)
	
	// Flow control settings (optimized for high bandwidth)
	InitialStreamReceiveWindow     int64 `yaml:"initial_stream_receive_window"`     // Initial stream receive window (default: 6 MB)
	MaxStreamReceiveWindow         int64 `yaml:"max_stream_receive_window"`         // Maximum stream receive window (default: 24 MB)
	InitialConnectionReceiveWindow int64 `yaml:"initial_connection_receive_window"` // Initial connection receive window (default: 15 MB)
	MaxConnectionReceiveWindow     int64 `yaml:"max_connection_receive_window"`     // Maximum connection receive window (default: 60 MB)
	
	// Performance settings
	EnableDatagrams bool `yaml:"enable_datagrams"` // Enable QUIC datagram support (default: false)
	Enable0RTT      bool `yaml:"enable_0rtt"`      // Enable 0-RTT for faster reconnections (default: true)
	
	// Keep-alive settings
	KeepAlivePeriod int `yaml:"keep_alive_period"` // Keep-alive period in seconds (default: 10)
	
	// TLS settings
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"` // Skip TLS verification (default: false, set true for testing)
	ServerName         string `yaml:"server_name"`           // Server name for TLS verification
	
	// Internal TLS config (not exposed to YAML)
	TLSConfig *tls.Config `yaml:"-"`
}

func (q *QUIC) setDefaults(role string) {
	if q.MaxIdleTimeout == 0 {
		q.MaxIdleTimeout = 30
	}
	
	if q.MaxIncomingStreams == 0 {
		if role == "server" {
			q.MaxIncomingStreams = 10000 // High limit for servers
		} else {
			q.MaxIncomingStreams = 1000
		}
	}
	
	if q.MaxIncomingUniStreams == 0 {
		if role == "server" {
			q.MaxIncomingUniStreams = 10000
		} else {
			q.MaxIncomingUniStreams = 1000
		}
	}
	
	// Flow control defaults optimized for high bandwidth
	if q.InitialStreamReceiveWindow == 0 {
		if role == "server" {
			q.InitialStreamReceiveWindow = 6 * 1024 * 1024 // 6 MB
		} else {
			q.InitialStreamReceiveWindow = 6 * 1024 * 1024 // 6 MB
		}
	}
	
	if q.MaxStreamReceiveWindow == 0 {
		if role == "server" {
			q.MaxStreamReceiveWindow = 24 * 1024 * 1024 // 24 MB
		} else {
			q.MaxStreamReceiveWindow = 24 * 1024 * 1024 // 24 MB
		}
	}
	
	if q.InitialConnectionReceiveWindow == 0 {
		if role == "server" {
			q.InitialConnectionReceiveWindow = 15 * 1024 * 1024 // 15 MB
		} else {
			q.InitialConnectionReceiveWindow = 15 * 1024 * 1024 // 15 MB
		}
	}
	
	if q.MaxConnectionReceiveWindow == 0 {
		if role == "server" {
			q.MaxConnectionReceiveWindow = 60 * 1024 * 1024 // 60 MB
		} else {
			q.MaxConnectionReceiveWindow = 60 * 1024 * 1024 // 60 MB
		}
	}
	
	if q.KeepAlivePeriod == 0 {
		q.KeepAlivePeriod = 10
	}
	
	// Enable 0-RTT by default for performance
	// (Note: In YAML, if not set, the zero value is false, so we set it in code)
	// We'll check for explicit configuration in validate
}

func (q *QUIC) validate() []error {
	var errors []error
	
	if q.MaxIdleTimeout < 1 || q.MaxIdleTimeout > 600 {
		errors = append(errors, fmt.Errorf("QUIC max_idle_timeout must be between 1-600 seconds"))
	}
	
	if q.MaxIncomingStreams < 1 || q.MaxIncomingStreams > 100000 {
		errors = append(errors, fmt.Errorf("QUIC max_incoming_streams must be between 1-100000"))
	}
	
	if q.MaxIncomingUniStreams < 1 || q.MaxIncomingUniStreams > 100000 {
		errors = append(errors, fmt.Errorf("QUIC max_incoming_uni_streams must be between 1-100000"))
	}
	
	if q.InitialStreamReceiveWindow < 1024*1024 {
		errors = append(errors, fmt.Errorf("QUIC initial_stream_receive_window must be >= 1 MB"))
	}
	
	if q.MaxStreamReceiveWindow < q.InitialStreamReceiveWindow {
		errors = append(errors, fmt.Errorf("QUIC max_stream_receive_window must be >= initial_stream_receive_window"))
	}
	
	if q.InitialConnectionReceiveWindow < 1024*1024 {
		errors = append(errors, fmt.Errorf("QUIC initial_connection_receive_window must be >= 1 MB"))
	}
	
	if q.MaxConnectionReceiveWindow < q.InitialConnectionReceiveWindow {
		errors = append(errors, fmt.Errorf("QUIC max_connection_receive_window must be >= initial_connection_receive_window"))
	}
	
	if q.KeepAlivePeriod < 1 || q.KeepAlivePeriod > 60 {
		errors = append(errors, fmt.Errorf("QUIC keep_alive_period must be between 1-60 seconds"))
	}
	
	return errors
}

// Certificate validity period for self-signed certificates
const certValidityDays = 365

// GenerateTLSConfig generates a TLS configuration for QUIC
func (q *QUIC) GenerateTLSConfig(role string) (*tls.Config, error) {
	if role == "server" {
		// Generate self-signed certificate for server
		cert, err := generateSelfSignedCert()
		if err != nil {
			return nil, fmt.Errorf("failed to generate self-signed certificate: %w", err)
		}
		
		return &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{"paqet-quic"},
			MinVersion:   tls.VersionTLS13, // QUIC requires TLS 1.3
		}, nil
	}
	
	// Client configuration
	tlsConfig := &tls.Config{
		NextProtos:         []string{"paqet-quic"},
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: q.InsecureSkipVerify,
	}
	
	if q.ServerName != "" {
		tlsConfig.ServerName = q.ServerName
	}
	
	return tlsConfig, nil
}

func generateSelfSignedCert() (tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(certValidityDays * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, err
	}
	
	return tlsCert, nil
}
