package socket

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

const (
	frameMagic     uint16 = 0x504B // "PK"
	frameVersion   byte   = 0x01
	frameHeaderLen        = 12
)

func encodeFrame(payload []byte) []byte {
	buf := make([]byte, frameHeaderLen+len(payload))
	binary.BigEndian.PutUint16(buf[0:2], frameMagic)
	buf[2] = frameVersion
	buf[3] = 0
	binary.BigEndian.PutUint32(buf[4:8], uint32(len(payload)))
	binary.BigEndian.PutUint32(buf[8:12], crc32.ChecksumIEEE(payload))
	copy(buf[frameHeaderLen:], payload)
	return buf
}

func decodeFrame(raw []byte) ([]byte, error) {
	if len(raw) < frameHeaderLen {
		return nil, fmt.Errorf("frame too short")
	}
	if binary.BigEndian.Uint16(raw[0:2]) != frameMagic {
		return nil, fmt.Errorf("invalid frame magic")
	}
	if raw[2] != frameVersion {
		return nil, fmt.Errorf("unsupported frame version: %d", raw[2])
	}

	size := int(binary.BigEndian.Uint32(raw[4:8]))
	if size < 0 || size > len(raw)-frameHeaderLen {
		return nil, fmt.Errorf("invalid frame size")
	}

	payload := raw[frameHeaderLen : frameHeaderLen+size]
	checksum := binary.BigEndian.Uint32(raw[8:12])
	if crc32.ChecksumIEEE(payload) != checksum {
		return nil, fmt.Errorf("frame checksum mismatch")
	}

	return payload, nil
}
