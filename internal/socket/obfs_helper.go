package socket

import (
	"crypto/sha256"
	"fmt"
	"paqet/internal/conf"
	"paqet/internal/obfs"
	
	"golang.org/x/crypto/pbkdf2"
)

// NewPacketConnWithObfs creates a PacketConn with obfuscation configured
func NewPacketConnWithObfs(cfg *conf.Network, obfsCfg *conf.Obfuscation, kcpKey string) (*PacketConn, error) {
	// Create base packet connection
	conn, err := New(nil, cfg)
	if err != nil {
		return nil, err
	}
	
	// Configure obfuscation if enabled
	if obfsCfg.Mode != "none" && obfsCfg.Mode != "" {
		// Derive key from KCP key for obfuscation
		key := pbkdf2.Key([]byte(kcpKey), []byte("paqet-obfs"), 100_000, 32, sha256.New)
		
		obfuscator, err := obfs.New(obfsCfg.Mode, key)
		if err != nil {
			return nil, fmt.Errorf("failed to create obfuscator: %w", err)
		}
		
		conn.obfuscator = obfuscator
	}
	
	return conn, nil
}

// SetObfuscator sets the obfuscator for this connection
func (c *PacketConn) SetObfuscator(o obfs.Obfuscator) {
	c.obfuscator = o
}
