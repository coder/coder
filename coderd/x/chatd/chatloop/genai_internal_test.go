package chatloop

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestTruncateBytesKeepsValidUTF8(t *testing.T) {
	t.Parallel()

	const limit = 9
	short := "héllo"
	require.Equal(t, short, truncateBytes(short, limit))

	// Offset by one byte so the limit falls inside a two-byte rune.
	long := "a" + strings.Repeat("é", limit)
	got := truncateBytes(long, limit)
	require.LessOrEqual(t, len(got), limit)
	require.True(t, utf8.ValidString(got))
	require.True(t, strings.HasPrefix(long, got))
}
