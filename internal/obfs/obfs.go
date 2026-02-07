package obfs

import "errors"

var (
	ErrInvalidData   = errors.New("invalid obfuscated data")
	ErrBufferTooSmall = errors.New("buffer too small for obfuscation")
)

// Obfuscator wraps/unwraps data with obfuscation layer to evade DPI detection
type Obfuscator interface {
	// Name returns the obfuscator identifier
	Name() string
	
	// Wrap adds obfuscation layer to plaintext data
	// Returns obfuscated data or error
	Wrap(data []byte) ([]byte, error)
	
	// Unwrap removes obfuscation layer from obfuscated data
	// Returns plaintext data or error
	Unwrap(data []byte) ([]byte, error)
	
	// Overhead returns maximum bytes added by obfuscation
	Overhead() int
}

// NewFunc is a constructor function for creating obfuscators
type NewFunc func(key []byte) (Obfuscator, error)

// Registry maps obfuscator names to constructor functions
var Registry = map[string]NewFunc{
	"none":    NewNoneObfuscator,
	"padding": NewPaddingObfuscator,
	"tls":     NewTLSRecordObfuscator,
}

// New creates an obfuscator by name with the given key
func New(name string, key []byte) (Obfuscator, error) {
	fn, ok := Registry[name]
	if !ok {
		return nil, errors.New("unknown obfuscator: " + name)
	}
	return fn(key)
}
