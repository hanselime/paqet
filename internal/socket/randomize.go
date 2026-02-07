package socket

import (
	"crypto/rand"
	"encoding/binary"
	"math"
)

// CryptoRandUint32 generates a cryptographically secure random uint32
func CryptoRandUint32() uint32 {
	var b [4]byte
	_, err := rand.Read(b[:])
	if err != nil {
		// Should never happen, but fallback to timestamp-based
		panic("crypto/rand unavailable: " + err.Error())
	}
	return binary.BigEndian.Uint32(b[:])
}

// CryptoRandUint16 generates a cryptographically secure random uint16
func CryptoRandUint16() uint16 {
	var b [2]byte
	_, err := rand.Read(b[:])
	if err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return binary.BigEndian.Uint16(b[:])
}

// RandInRange returns a random value in [min, max] inclusive
func RandInRange(min, max uint32) uint32 {
	if min >= max {
		return min
	}
	diff := max - min + 1
	return min + (CryptoRandUint32() % diff)
}

// RandInRange16 returns a random uint16 value in [min, max] inclusive
func RandInRange16(min, max uint16) uint16 {
	if min >= max {
		return min
	}
	diff := uint32(max) - uint32(min) + 1
	return uint16(uint32(min) + (CryptoRandUint32() % diff))
}

// RandInRange8 returns a random uint8 value in [min, max] inclusive
func RandInRange8(min, max uint8) uint8 {
	if min >= max {
		return min
	}
	diff := uint32(max) - uint32(min) + 1
	return uint8(uint32(min) + (CryptoRandUint32() % diff))
}

// GenerateRealisticTOS returns TOS values commonly seen in real traffic
// Mimics HTTPS/HTTP2 traffic patterns to blend in
func GenerateRealisticTOS() uint8 {
	// Common TOS values in real HTTPS traffic:
	// 0x00 (0)   - Default, most common
	// 0x20 (32)  - CS1 (Low priority)
	// 0x28 (40)  - CS5 (Video)
	// 0xB8 (184) - EF (Expedited Forwarding) - less common but used
	tosValues := []uint8{0, 0, 0, 0, 32, 40} // Weighted toward 0
	idx := CryptoRandUint32() % uint32(len(tosValues))
	return tosValues[idx]
}

// GenerateRealisticTTL returns TTL values mimicking real OS behavior
// Different operating systems use different default TTLs
func GenerateRealisticTTL() uint8 {
	// Common OS TTL values:
	// 64  - Linux/Unix/macOS
	// 128 - Windows
	// 255 - Some network devices
	// We'll randomize around 64 (48-64) to mimic Linux/macOS with some network hops
	return RandInRange8(48, 64)
}

// GenerateRealisticWindow returns window sizes mimicking real TCP stacks
// Modern browsers and systems use various window sizes
func GenerateRealisticWindow() uint16 {
	// Common window sizes in real traffic:
	// 65535 (64KB) - Maximum for non-scaled windows
	// 32768 (32KB) - Common
	// 16384 (16KB) - Common
	// We'll use a range that looks realistic
	windowSizes := []uint16{
		65535, 65535, // Most common
		32768, 32768,
		29200, // Chrome/Firefox common value
		16384,
	}
	idx := CryptoRandUint32() % uint32(len(windowSizes))
	return windowSizes[idx]
}

// GenerateRealisticMSS returns MSS values commonly seen in real traffic
func GenerateRealisticMSS() uint16 {
	// Common MSS values:
	// 1460 - Most common (1500 MTU - 40 bytes)
	// 1440 - PPPoE
	// 1380 - VPN
	mssValues := []uint16{1460, 1460, 1460, 1440, 1380}
	idx := CryptoRandUint32() % uint32(len(mssValues))
	return mssValues[idx]
}

// GenerateRealisticWindowScale returns window scale values (0-14)
func GenerateRealisticWindowScale() uint8 {
	// Common window scale values: 7, 8, 9
	scales := []uint8{7, 8, 8, 9}
	idx := CryptoRandUint32() % uint32(len(scales))
	return scales[idx]
}

// AddJitter adds random jitter to a uint32 value within a percentage range
func AddJitter(value uint32, jitterPercent float64) uint32 {
	if jitterPercent <= 0 {
		return value
	}
	
	maxJitter := uint32(float64(value) * jitterPercent / 100.0)
	if maxJitter == 0 {
		return value
	}
	
	// Random jitter in range [-maxJitter, +maxJitter]
	jitter := CryptoRandUint32() % (2 * maxJitter + 1)
	jitter = jitter - maxJitter // Convert to signed range
	
	// Prevent underflow
	if jitter > value && jitter > math.MaxUint32-value {
		return value
	}
	
	return value + jitter
}

// ShuffleTCPOptions randomly shuffles TCP options order
// This helps avoid fingerprinting based on option ordering
func ShuffleTCPOptions(options []interface{}) []interface{} {
	n := len(options)
	if n <= 1 {
		return options
	}
	
	result := make([]interface{}, n)
	copy(result, options)
	
	// Fisher-Yates shuffle
	for i := n - 1; i > 0; i-- {
		j := int(CryptoRandUint32() % uint32(i+1))
		result[i], result[j] = result[j], result[i]
	}
	
	return result
}
