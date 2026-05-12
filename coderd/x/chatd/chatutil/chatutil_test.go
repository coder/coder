package chatutil_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatutil"
)

func TestNormalizedStringPointer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value *string
		want  *string
	}{
		{name: "Nil"},
		{name: "Empty", value: ptr("")},
		{name: "WhitespaceOnly", value: ptr(" \t\n ")},
		{name: "Trimmed", value: ptr(" value "), want: ptr("value")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatutil.NormalizedStringPointer(tt.value)
			if tt.want == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, *tt.want, *got)
		})
	}
}

func TestNormalizedEnumValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		allowed []string
		want    *string
	}{
		{
			name:    "MatchFound",
			value:   "medium",
			allowed: []string{"Low", "Medium", "High"},
			want:    ptr("Medium"),
		},
		{
			name:    "MatchMissing",
			value:   "maximum",
			allowed: []string{"Low", "Medium", "High"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatutil.NormalizedEnumValue(tt.value, tt.allowed...)
			if tt.want == nil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			require.Equal(t, *tt.want, *got)
		})
	}
}

func ptr[T any](value T) *T {
	return &value
}
