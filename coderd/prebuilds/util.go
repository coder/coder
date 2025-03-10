package prebuilds

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"time"
)

// GenerateName generates a 20-byte prebuild name which should safe to use without truncation in most situations.
// UUIDs may be too long for a resource name in cloud providers (since this ID will be used in the prebuild's name).
//
// We're generating a 9-byte suffix (72 bits of entry):
// 1 - e^(-1e9^2 / (2 * 2^72)) = ~0.01% likelihood of collision in 1 billion IDs.
// See https://en.wikipedia.org/wiki/Birthday_attack.
func GenerateName() (string, error) {
	b := make([]byte, 9)

	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	// Encode the bytes to Base32 (A-Z2-7), strip any '=' padding
	return fmt.Sprintf("prebuild-%s", strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))), nil
}

// DurationToInterval converts a given duration to microseconds, which is the unit PG represents intervals in.
func DurationToInterval(d time.Duration) int32 {
	// Convert duration to seconds (as an example)
	return int32(d.Microseconds())
}
