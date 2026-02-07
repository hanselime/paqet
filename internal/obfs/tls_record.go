package obfs

import (
	"crypto/rand"
	"encoding/binary"
)

// TLSRecordObfuscator wraps traffic in TLS 1.2/1.3 Application Data record format
// This makes traffic look like encrypted HTTPS to evade DPI
// Frame format: [1 byte: 0x17] [2 bytes: version 0x0303] [2 bytes: length] [data]
type TLSRecordObfuscator struct {
	key []byte
}

const (
	tlsRecordTypeApplicationData = 0x17
	tlsVersion12                 = 0x0303 // TLS 1.2
	tlsVersion13                 = 0x0303 // TLS 1.3 also uses 0x0303 for records
	tlsRecordHeaderSize          = 5
	tlsMaxRecordSize             = 16384 // 16KB max per TLS record
)

// NewTLSRecordObfuscator creates a TLS record layer obfuscator
func NewTLSRecordObfuscator(key []byte) (Obfuscator, error) {
	return &TLSRecordObfuscator{
		key: key,
	}, nil
}

func (o *TLSRecordObfuscator) Name() string {
	return "tls"
}

func (o *TLSRecordObfuscator) Wrap(data []byte) ([]byte, error) {
	return o.wrapWithLength(data)
}

func (o *TLSRecordObfuscator) Unwrap(data []byte) ([]byte, error) {
	if len(data) < tlsRecordHeaderSize {
		return nil, ErrInvalidData
	}

	// Validate TLS record header
	if data[0] != tlsRecordTypeApplicationData {
		return nil, ErrInvalidData
	}

	version := binary.BigEndian.Uint16(data[1:3])
	if version != tlsVersion12 && version != tlsVersion13 {
		return nil, ErrInvalidData
	}

	recordLen := binary.BigEndian.Uint16(data[3:5])
	if int(recordLen) > len(data)-tlsRecordHeaderSize {
		return nil, ErrInvalidData
	}

	// Extract data (we need to determine actual data length vs padding)
	// For now, we'll use a simple approach: store actual length in first 2 bytes of payload
	payload := data[tlsRecordHeaderSize : tlsRecordHeaderSize+recordLen]
	
	if len(payload) < 2 {
		return nil, ErrInvalidData
	}

	// Decode actual data length (XOR'd with key for obfuscation)
	var actualLen uint16
	if len(o.key) >= 2 {
		actualLen = binary.BigEndian.Uint16(payload[0:2]) ^ binary.BigEndian.Uint16(o.key[0:2])
	} else {
		actualLen = binary.BigEndian.Uint16(payload[0:2])
	}

	if int(actualLen) > len(payload)-2 {
		return nil, ErrInvalidData
	}

	result := make([]byte, actualLen)
	copy(result, payload[2:2+actualLen])

	return result, nil
}

func (o *TLSRecordObfuscator) Overhead() int {
	return tlsRecordHeaderSize + 2 + 15 // Header + length encoding + max padding
}

// Improved Wrap that includes length encoding
func (o *TLSRecordObfuscator) wrapWithLength(data []byte) ([]byte, error) {
	dataLen := len(data)
	
	// Add 2 bytes for length encoding
	if dataLen+2 > tlsMaxRecordSize {
		return nil, ErrBufferTooSmall
	}

	// Random padding (0-15 bytes)
	padLen := int(cryptoRandUint32() % 16)
	if dataLen+2+padLen > tlsMaxRecordSize {
		padLen = tlsMaxRecordSize - dataLen - 2
	}

	totalDataLen := 2 + dataLen + padLen
	result := make([]byte, tlsRecordHeaderSize+totalDataLen)

	// TLS Record Header
	result[0] = tlsRecordTypeApplicationData
	binary.BigEndian.PutUint16(result[1:3], tlsVersion12)
	binary.BigEndian.PutUint16(result[3:5], uint16(totalDataLen))

	// Encode actual data length (XOR with key for obfuscation)
	lengthField := uint16(dataLen)
	if len(o.key) >= 2 {
		lengthField ^= binary.BigEndian.Uint16(o.key[0:2])
	}
	binary.BigEndian.PutUint16(result[tlsRecordHeaderSize:tlsRecordHeaderSize+2], lengthField)

	// Copy actual data
	copy(result[tlsRecordHeaderSize+2:tlsRecordHeaderSize+2+dataLen], data)

	// Random padding
	if padLen > 0 {
		_, err := rand.Read(result[tlsRecordHeaderSize+2+dataLen:])
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}
