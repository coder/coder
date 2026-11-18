package chatloop

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestTruncateContentKeepsValidUTF8(t *testing.T) {
	t.Parallel()

	short := "héllo"
	require.Equal(t, short, truncateContent(short))

	// Offset by one byte so the limit falls inside a two-byte rune.
	long := "a" + strings.Repeat("é", MaxContentAttributeBytes)
	got := truncateContent(long)
	require.LessOrEqual(t, len(got), MaxContentAttributeBytes)
	require.True(t, utf8.ValidString(got))
	require.True(t, strings.HasPrefix(long, got))
}
