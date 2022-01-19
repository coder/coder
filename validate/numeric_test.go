package validate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Numeric(t *testing.T) {
	testCases := []struct {
		S        string
		Expected bool
	}{
		{"", false},
		{"a1", false},
		{"1a", false},
		{"1a1", false},
		{"1234", true},
	}
	for _, tc := range testCases {
		actual := Numeric(tc.S)
		require.Equal(t, tc.Expected, actual, tc.S)
	}
}
