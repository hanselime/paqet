package framing

import (
	"crypto/rand"
	"encoding/binary"
)

// RandomFramer splits data into random-sized frames
// This defeats statistical length analysis by DPI systems
type RandomFramer struct {
	minSize int
	maxSize int
	jitter  int
}

// NewRandomFramer creates a random-size framer
func NewRandomFramer(minSize, maxSize, jitter int) Framer {
	if minSize < 64 {
		minSize = 64
	}
	if maxSize < minSize {
		maxSize = 1400
	}
	
	return &RandomFramer{
		minSize: minSize,
		maxSize: maxSize,
		jitter:  jitter,
	}
}

func (f *RandomFramer) Name() string {
	return "random"
}

func (f *RandomFramer) Frame(data []byte) ([][]byte, error) {
	dataLen := len(data)
	if dataLen == 0 {
		return [][]byte{}, nil
	}
	
	// For small data, return as single frame
	if dataLen <= f.minSize {
		return [][]byte{data}, nil
	}
	
	var frames [][]byte
	offset := 0
	
	for offset < dataLen {
		remaining := dataLen - offset
		
		// Determine frame size
		var frameSize int
		if remaining <= f.maxSize {
			frameSize = remaining
		} else {
			// Random size between minSize and maxSize
			frameSize = f.minSize + int(cryptoRandUint32()%uint32(f.maxSize-f.minSize+1))
			if frameSize > remaining {
				frameSize = remaining
			}
		}
		
		// Extract frame
		frame := make([]byte, frameSize)
		copy(frame, data[offset:offset+frameSize])
		frames = append(frames, frame)
		
		offset += frameSize
	}
	
	return frames, nil
}

func (f *RandomFramer) Coalesce(frames [][]byte) ([]byte, error) {
	if len(frames) == 0 {
		return nil, nil
	}
	if len(frames) == 1 {
		return frames[0], nil
	}
	
	// Concatenate all frames
	totalLen := 0
	for _, frame := range frames {
		totalLen += len(frame)
	}
	
	result := make([]byte, totalLen)
	offset := 0
	for _, frame := range frames {
		copy(result[offset:], frame)
		offset += len(frame)
	}
	
	return result, nil
}

// cryptoRandUint32 generates a cryptographically secure random uint32
func cryptoRandUint32() uint32 {
	var b [4]byte
	_, err := rand.Read(b[:])
	if err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return binary.BigEndian.Uint32(b[:])
}
