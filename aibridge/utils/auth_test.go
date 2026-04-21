package utils_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/aibridge/utils"
)

func TestExtractBearerToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Empty",
			input:    "",
			expected: "",
		},
		{
			name:     "Whitespace",
			input:    " ",
			expected: "",
		},
		{
			name:     "InvalidFormat",
			input:    "some-token",
			expected: "",
		},
		{
			name:     "BearerOnly",
			input:    "Bearer",
			expected: "",
		},
		{
			name:     "Valid",
			input:    "Bearer my-secret-token",
			expected: "my-secret-token",
		},
		{
			name:     "BearerMixedCase",
			input:    "BeArEr my-secret-token",
			expected: "my-secret-token",
		},
		{
			name:     "LeadingWhitespace",
			input:    "  Bearer my-secret-token",
			expected: "my-secret-token",
		},
		{
			name:     "TrailingWhitespace",
			input:    "Bearer my-secret-token  ",
			expected: "my-secret-token",
		},
		{
			name:     "TooManyParts",
			input:    "Bearer token extra",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := utils.ExtractBearerToken(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
