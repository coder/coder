// This test runs slowly on macOS instance, and really
// only needs to run on Linux anyways.
//go:build linux

package userpassword_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/userpassword"
)

func TestUserPasswordValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"Invalid - Too short password", "pass", true},
		{"Invalid - Too long password", strings.Repeat("a", 65), true},
		{"Ok", "CorrectPassword", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := userpassword.Validate(tt.password)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUserPasswordCompare(t *testing.T) {
	tests := []struct {
		name      string
		hash      string
		password  string
		wantErr   bool
		wantEqual bool
	}{
		{"Legacy", "$pbkdf2-sha256$65535$z8c1p1C2ru9EImBP1I+ZNA$pNjE3Yk0oG0PmJ0Je+y7ENOVlSkn/b0BEqqdKsq6Y97wQBq0xT+lD5bWJpyIKJqQICuPZcEaGDKrXJn8+SIHRg", "tomato", false, true},
		{"Same", "", "password", false, true},
		{"Different", "", "password", false, false},
		{"Invalid", "invalidhash", "password", true, false},
		{"InvalidParts", "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz", "test", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.hash == "" {
				hash, err := userpassword.Hash(tt.password)
				require.NoError(t, err)
				tt.hash = hash
			}
			equal, err := userpassword.Compare(tt.hash, tt.password)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantEqual, equal)
		})
	}
}
