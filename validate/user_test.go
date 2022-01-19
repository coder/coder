package validate

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Username(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Input string
		Err   error
	}{
		{"1", nil},
		{"12", nil},
		{"123", nil},
		{"12345678901234567890", nil},
		{"123456789012345678901", nil},
		{"a", nil},
		{"a1", nil},
		{"a1b2", nil},
		{"a1b2c3d4e5f6g7h8i9j0", nil},
		{"a1b2c3d4e5f6g7h8i9j0k", nil},
		{"aa", nil},
		{"abc", nil},
		{"abcdefghijklmnopqrst", nil},
		{"abcdefghijklmnopqrstu", nil},
		{"wow-test", nil},

		{"", ErrInvalidUsernameRegex},
		{" ", ErrInvalidUsernameRegex},
		{" a", ErrInvalidUsernameRegex},
		{" a ", ErrInvalidUsernameRegex},
		{" 1", ErrInvalidUsernameRegex},
		{"1 ", ErrInvalidUsernameRegex},
		{" aa", ErrInvalidUsernameRegex},
		{"aa ", ErrInvalidUsernameRegex},
		{" 12", ErrInvalidUsernameRegex},
		{"12 ", ErrInvalidUsernameRegex},
		{" a1", ErrInvalidUsernameRegex},
		{"a1 ", ErrInvalidUsernameRegex},
		{" abcdefghijklmnopqrstu", ErrInvalidUsernameRegex},
		{"abcdefghijklmnopqrstu ", ErrInvalidUsernameRegex},
		{" 123456789012345678901", ErrInvalidUsernameRegex},
		{" a1b2c3d4e5f6g7h8i9j0k", ErrInvalidUsernameRegex},
		{"a1b2c3d4e5f6g7h8i9j0k ", ErrInvalidUsernameRegex},
		{"bananas_wow", ErrInvalidUsernameRegex},
		{"test--now", ErrInvalidUsernameRegex},

		{"123456789012345678901234567890123", ErrUsernameTooLong},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ErrUsernameTooLong},
		{"123456789012345678901234567890123123456789012345678901234567890123", ErrUsernameTooLong},
	}
	for _, testCase := range testCases {
		t.Run(testCase.Input, func(t *testing.T) {
			if testCase.Err == nil {
				require.NoError(t, Username(testCase.Input), fmt.Sprintf("username %q should be valid", testCase.Input))
			} else {
				require.Equal(t, Username(testCase.Input), testCase.Err, fmt.Sprintf("username %q should not be valid", testCase.Input))
			}
		})
	}
}
