package testutil

import (
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/cryptorand"
)

// MustRandString returns a random string of length n.
func MustRandString(t *testing.T, n int) string {
	t.Helper()
	s, err := cryptorand.String(n)
	require.NoError(t, err)
	return s
}

// RandomIPv6 returns a random IPv6 address in the 2001:db8::/32 range.
// 2001:db8::/32 is reserved for documentation and example code.
func RandomIPv6(t testing.TB) string {
	t.Helper()

	buf := make([]byte, 16)
	_, err := rand.Read(buf)
	require.NoError(t, err, "generate random IPv6 address")
	return fmt.Sprintf(
		"2001:db8:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x:%02x%02x",
		buf[0], buf[1], buf[2], buf[3], buf[4], buf[5],
		buf[6], buf[7], buf[8], buf[9], buf[10], buf[11],
	)
}
