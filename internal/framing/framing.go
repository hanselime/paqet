package framing

// Framer handles splitting and coalescing data into frames
// This helps defeat statistical length analysis by DPI systems
type Framer interface {
	// Name returns the framer identifier
	Name() string
	
	// Frame splits data into one or more frames
	// Returns array of frame data
	Frame(data []byte) ([][]byte, error)
	
	// Coalesce combines frames back into original data
	// This is typically handled at the application layer
	Coalesce(frames [][]byte) ([]byte, error)
}

// NewFunc is a constructor function for creating framers
type NewFunc func(minSize, maxSize, jitter int) Framer

// Registry maps framer names to constructor functions
var Registry = map[string]NewFunc{
	"fixed":  NewFixedFramer,
	"random": NewRandomFramer,
}

// New creates a framer by name with the given parameters
func New(name string, minSize, maxSize, jitter int) Framer {
	fn, ok := Registry[name]
	if !ok {
		// Default to fixed framing
		return NewFixedFramer(minSize, maxSize, jitter)
	}
	return fn(minSize, maxSize, jitter)
}
