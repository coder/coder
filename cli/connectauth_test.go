package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeyIDFromToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{
			name:     "NormalToken",
			token:    "abcdefghij-secretsecretsecretsecre",
			expected: "abcdefghij",
		},
		{
			name:     "NoSeparator",
			token:    "nodashhere",
			expected: "nodashhere",
		},
		{
			name:     "EmptyToken",
			token:    "",
			expected: "",
		},
		{
			name:     "MultipleDashes",
			token:    "abcdefghij-secret-with-dashes",
			expected: "abcdefghij",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, KeyIDFromToken(tt.token))
		})
	}
}
