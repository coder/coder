package namesgenerator

import (
	"testing"

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

func TestGetRandomName(t *testing.T) {
	t.Parallel()
	for range len(adjectives) {
		name := GetRandomName()
		assert.LessOrEqual(t, len(name), maxNameLen)
	}
}

func TestGetRandomNameHyphenated(t *testing.T) {
	t.Parallel()
	for range len(adjectives) {
		name := GetRandomNameHyphenated()
		assert.LessOrEqual(t, len(name), maxNameLen)
		assert.NotContains(t, name, "_")
	}
}
