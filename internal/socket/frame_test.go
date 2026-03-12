package socket

import "testing"

func TestFrameRoundTrip(t *testing.T) {
	payload := []byte("hello-paqet")
	encoded := encodeFrame(payload)

	decoded, err := decodeFrame(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(decoded) != string(payload) {
		t.Fatalf("payload mismatch: got %q want %q", decoded, payload)
	}
}

func TestFrameChecksumValidation(t *testing.T) {
	payload := []byte("hello")
	encoded := encodeFrame(payload)
	encoded[len(encoded)-1] ^= 0xff

	if _, err := decodeFrame(encoded); err == nil {
		t.Fatal("expected checksum error")
	}
}
