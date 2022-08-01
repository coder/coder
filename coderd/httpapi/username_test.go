package httpapi_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/httpapi"
)

func TestValid(t *testing.T) {
	t.Parallel()
	// Tests whether usernames are valid or not.
	testCases := []struct {
		Username string
		Valid    bool
	}{
		{"1", true},
		{"12", true},
		{"123", true},
		{"12345678901234567890", true},
		{"123456789012345678901", true},
		{"a", true},
		{"a1", true},
		{"a1b2", true},
		{"a1b2c3d4e5f6g7h8i9j0", true},
		{"a1b2c3d4e5f6g7h8i9j0k", true},
		{"aa", true},
		{"abc", true},
		{"abcdefghijklmnopqrst", true},
		{"abcdefghijklmnopqrstu", true},
		{"wow-test", true},

		{"", false},
		{" ", false},
		{" a", false},
		{" a ", false},
		{" 1", false},
		{"1 ", false},
		{" aa", false},
		{"aa ", false},
		{" 12", false},
		{"12 ", false},
		{" a1", false},
		{"a1 ", false},
		{" abcdefghijklmnopqrstu", false},
		{"abcdefghijklmnopqrstu ", false},
		{" 123456789012345678901", false},
		{" a1b2c3d4e5f6g7h8i9j0k", false},
		{"a1b2c3d4e5f6g7h8i9j0k ", false},
		{"bananas_wow", false},
		{"test--now", false},

		{"123456789012345678901234567890123", false},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"123456789012345678901234567890123123456789012345678901234567890123", false},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.Username, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, testCase.Valid, httpapi.UsernameValid(testCase.Username))
		})
	}
}

func TestFrom(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		From  string
		Match string
	}{
		{"1", "1"},
		{"kyle@kwc.io", "kyle"},
		{"kyle+wow@kwc.io", "kylewow"},
		{"kyle+testing", "kyletesting"},
		{"kyle-testing", "kyle-testing"},
		{"much.”more unusual”@example.com", "muchmoreunusual"},

		// Cases where an invalid string is provided, and the result is a random name.
		{"123456789012345678901234567890123", ""},
		{"very.unusual.”@”.unusual.com@example.com", ""},
		{"___@ok.com", ""},
		{" something with spaces ", ""},
		{"--test--", ""},
		{"", ""},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.From, func(t *testing.T) {
			t.Parallel()
			converted := httpapi.UsernameFrom(testCase.From)
			t.Log(converted)
			require.True(t, httpapi.UsernameValid(converted))
			if testCase.Match == "" {
				require.NotEqual(t, testCase.From, converted)
			} else {
				require.Equal(t, testCase.Match, converted)
			}
		})
	}
}
