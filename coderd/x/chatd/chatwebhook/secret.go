package chatwebhook

import (
	"crypto/rand"
	"encoding/base64"

	"golang.org/x/xerrors"
)

// GenerateSecret returns a new "whsec_" secret over 32 random bytes, inside
// the 24 to 64 bytes that MCP event servers accept.
func GenerateSecret() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", xerrors.Errorf("read random bytes: %w", err)
	}
	return secretPrefix + base64.StdEncoding.EncodeToString(key), nil
}
