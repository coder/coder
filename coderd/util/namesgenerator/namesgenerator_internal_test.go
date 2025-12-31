package namesgenerator

import (
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/assert"
)

func TestTruncate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "no truncation needed",
			input:  "foo1",
			maxLen: 10,
			want:   "foo1",
		},
		{
			name:   "exact fit",
			input:  "foo1",
			maxLen: 4,
			want:   "foo1",
		},
		{
			name:   "truncate base",
			input:  "foobar42",
			maxLen: 5,
			want:   "foo42",
		},
		{
			name:   "truncate more",
			input:  "foobar3",
			maxLen: 3,
			want:   "fo3",
		},
		{
			name:   "long suffix",
			input:  "foo123456",
			maxLen: 8,
			want:   "fo123456",
		},
		{
			name:   "realistic name",
			input:  "condescending_proskuriakova999999",
			maxLen: 32,
			want:   "condescending_proskuriakov999999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := truncate(tt.input, tt.maxLen)
			assert.Equal(t, tt.want, got)
			assert.LessOrEqual(t, len(got), tt.maxLen)
		})
	}
}

func TestGetRandomNameLength(t *testing.T) {
	t.Parallel()

	const (
		initSeed = 1
		delim    = "-"
		// 10000 iterations arbitrary, but it will exercise many generated names.
		iter = 10000
	)
	for i := range iter {
		//nolint:gosec // not worried about overflow when when the iteration count is known/small.
		name := getRandomName(delim, initSeed+uint64(i))
		assert.LessOrEqual(t, len(name), maxNameLen)
		assert.Contains(t, name, delim)
		assert.Equal(t, name, strings.ToLower(name))
		verifyNoWhitespace(t, name)
	}
}

func verifyNoWhitespace(t *testing.T, s string) {
	t.Helper()
	for _, r := range s {
		if unicode.IsSpace(r) {
			t.Fatalf("found whitespace in string %q: %v", s, r)
		}
	}
}
