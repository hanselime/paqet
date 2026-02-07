package obfs

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
)

// PaddingObfuscator adds random padding to defeat length-based traffic analysis
// Frame format: [2 bytes: real length (XOR'd with key)] [N bytes: data] [0-255 bytes: random padding]
type PaddingObfuscator struct {
	key    []byte
	minPad int
	maxPad int
}

// NewPaddingObfuscator creates a padding-based obfuscator
// key: used to XOR the length field (at least 2 bytes required)
func NewPaddingObfuscator(key []byte) (Obfuscator, error) {
	if len(key) < 2 {
		return nil, errors.New("padding obfuscator requires key of at least 2 bytes")
	}
	return &PaddingObfuscator{
		key:    key,
		minPad: 16,  // Minimum padding bytes
		maxPad: 128, // Maximum padding bytes
	}, nil
}

func (o *PaddingObfuscator) Name() string {
	return "padding"
}

func (o *PaddingObfuscator) Wrap(data []byte) ([]byte, error) {
	dataLen := len(data)
	if dataLen > 65535 {
		return nil, ErrBufferTooSmall
	}

	// Generate random padding length
	padLen := o.minPad
	if o.maxPad > o.minPad {
		padLen += int(cryptoRandUint32() % uint32(o.maxPad-o.minPad+1))
	}

	// Allocate buffer: 2 bytes length + data + padding
	totalLen := 2 + dataLen + padLen
	result := make([]byte, totalLen)

	// Write obfuscated length (XOR with key)
	lengthBytes := uint16(dataLen)
	result[0] = byte(lengthBytes>>8) ^ o.key[0]
	result[1] = byte(lengthBytes) ^ o.key[1]

	// Copy data
	copy(result[2:2+dataLen], data)

	// Fill random padding
	if padLen > 0 {
		_, err := rand.Read(result[2+dataLen:])
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (o *PaddingObfuscator) Unwrap(data []byte) ([]byte, error) {
	if len(data) < 2 {
		return nil, ErrInvalidData
	}

	// Decode length (XOR with key)
	lengthBytes := uint16(data[0]^o.key[0])<<8 | uint16(data[1]^o.key[1])
	dataLen := int(lengthBytes)

	// Validate length
	if 2+dataLen > len(data) {
		return nil, ErrInvalidData
	}

	// Extract actual data (skip padding)
	result := make([]byte, dataLen)
	copy(result, data[2:2+dataLen])

	return result, nil
}

func (o *PaddingObfuscator) Overhead() int {
	return 2 + o.maxPad // Length field + max padding
}

// cryptoRandUint32 generates a cryptographically secure random uint32
func cryptoRandUint32() uint32 {
	var b [4]byte
	_, err := rand.Read(b[:])
	if err != nil {
		// Fallback to less secure but still random
		return uint32(binary.BigEndian.Uint32(b[:]))
	}
	return binary.BigEndian.Uint32(b[:])
}
