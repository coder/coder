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
		{name: "Invalid - Too short password", password: "pass", wantErr: true},
		{name: "Invalid - Too long password", password: strings.Repeat("a", 65), wantErr: true},
		{name: "Invalid - easy password", password: "password", wantErr: true},
		{name: "Ok", password: "PasswordSecured123!", wantErr: false},
	}

	for _, tt := range tests {
		tt := tt
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
	t.Parallel()
	tests := []struct {
		name               string
		passwordToValidate string
		password           string
		shouldHash         bool
		wantErr            bool
		wantEqual          bool
	}{
		{
			name:               "Legacy",
			passwordToValidate: "$pbkdf2-sha256$65535$z8c1p1C2ru9EImBP1I+ZNA$pNjE3Yk0oG0PmJ0Je+y7ENOVlSkn/b0BEqqdKsq6Y97wQBq0xT+lD5bWJpyIKJqQICuPZcEaGDKrXJn8+SIHRg",
			password:           "tomato",
			shouldHash:         false,
			wantErr:            false,
			wantEqual:          true,
		},
		{
			name:               "Same",
			passwordToValidate: "password",
			password:           "password",
			shouldHash:         true,
			wantErr:            false,
			wantEqual:          true,
		},
		{
			name:               "Different",
			passwordToValidate: "password",
			password:           "notpassword",
			shouldHash:         true,
			wantErr:            false,
			wantEqual:          false,
		},
		{
			name:               "Invalid",
			passwordToValidate: "invalidhash",
			password:           "password",
			shouldHash:         false,
			wantErr:            true,
			wantEqual:          false,
		},
		{
			name:               "InvalidParts",
			passwordToValidate: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz",
			password:           "test",
			shouldHash:         false,
			wantErr:            true,
			wantEqual:          false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.shouldHash {
				hash, err := userpassword.Hash(tt.passwordToValidate)
				require.NoError(t, err)
				tt.passwordToValidate = hash
			}
			equal, err := userpassword.Compare(tt.passwordToValidate, tt.password)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantEqual, equal)
		})
	}
}
