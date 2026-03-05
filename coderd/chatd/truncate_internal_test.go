package chatd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTruncateAtWordBoundary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "ShorterThanLimit",
			input:  "hello world",
			maxLen: 20,
			want:   "hello world",
		},
		{
			name:   "ExactlyAtLimit",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "LongerWithSpace",
			input:  "hello world this is long",
			maxLen: 15,
			want:   "hello world...",
		},
		{
			name:   "LongerNoSpaces",
			input:  "abcdefghijklmnopqrstuvwxyz",
			maxLen: 10,
			want:   "abcdefghij...",
		},
		{
			name:   "EmptyString",
			input:  "",
			maxLen: 10,
			want:   "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := truncateAtWordBoundary(tc.input, tc.maxLen)
			require.Equal(t, tc.want, got)
		})
	}
}
