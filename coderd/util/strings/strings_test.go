package strings_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/strings"
)

func TestJoinWithConjunction(t *testing.T) {
	t.Parallel()
	require.Equal(t, "foo", strings.JoinWithConjunction([]string{"foo"}))
	require.Equal(t, "foo and bar", strings.JoinWithConjunction([]string{"foo", "bar"}))
	require.Equal(t, "foo, bar and baz", strings.JoinWithConjunction([]string{"foo", "bar", "baz"}))
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		s        string
		n        int
		expected string
	}{
		{"foo", 4, "foo"},
		{"foo", 3, "foo"},
		{"foo", 2, "fo"},
		{"foo", 1, "f"},
		{"foo", 0, ""},
		{"foo", -1, ""},
	} {
		tt := tt
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			actual := strings.Truncate(tt.s, tt.n)
			require.Equal(t, tt.expected, actual)
		})
	}
}
