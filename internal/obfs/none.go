package obfs

// NoneObfuscator is a passthrough obfuscator that does nothing
// Used for backward compatibility and testing
type NoneObfuscator struct{}

// NewNoneObfuscator creates a passthrough obfuscator
func NewNoneObfuscator(key []byte) (Obfuscator, error) {
	return &NoneObfuscator{}, nil
}

func (o *NoneObfuscator) Name() string {
	return "none"
}

func (o *NoneObfuscator) Wrap(data []byte) ([]byte, error) {
	return data, nil
}

func (o *NoneObfuscator) Unwrap(data []byte) ([]byte, error) {
	return data, nil
}

func (o *NoneObfuscator) Overhead() int {
	return 0
}
