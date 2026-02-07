package framing

// FixedFramer passes data through without modification
// This is the default behavior (current paqet behavior)
type FixedFramer struct {
	minSize int
	maxSize int
	jitter  int
}

// NewFixedFramer creates a fixed-size framer (passthrough)
func NewFixedFramer(minSize, maxSize, jitter int) Framer {
	return &FixedFramer{
		minSize: minSize,
		maxSize: maxSize,
		jitter:  jitter,
	}
}

func (f *FixedFramer) Name() string {
	return "fixed"
}

func (f *FixedFramer) Frame(data []byte) ([][]byte, error) {
	// Passthrough - return data as single frame
	return [][]byte{data}, nil
}

func (f *FixedFramer) Coalesce(frames [][]byte) ([]byte, error) {
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
