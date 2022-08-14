// This test runs slowly on macOS instance, and really
// only needs to run on Linux anyways.
//go:build linux

package userpassword_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/userpassword"
)

func TestUserPassword(t *testing.T) {
	t.Parallel()
	t.Run("Legacy", func(t *testing.T) {
		t.Parallel()
		// Ensures legacy v1 passwords function for v2.
		// This has is manually generated using a print statement from v1 code.
		equal, err := userpassword.Compare("$pbkdf2-sha256$65535$z8c1p1C2ru9EImBP1I+ZNA$pNjE3Yk0oG0PmJ0Je+y7ENOVlSkn/b0BEqqdKsq6Y97wQBq0xT+lD5bWJpyIKJqQICuPZcEaGDKrXJn8+SIHRg", "tomato")
		require.NoError(t, err)
		require.True(t, equal)
	})

	t.Run("Same", func(t *testing.T) {
		t.Parallel()
		hash, err := userpassword.Hash("password")
		require.NoError(t, err)
		equal, err := userpassword.Compare(hash, "password")
		require.NoError(t, err)
		require.True(t, equal)
	})

	t.Run("Different", func(t *testing.T) {
		t.Parallel()
		hash, err := userpassword.Hash("password")
		require.NoError(t, err)
		equal, err := userpassword.Compare(hash, "notpassword")
		require.NoError(t, err)
		require.False(t, equal)
	})

	t.Run("Invalid", func(t *testing.T) {
		t.Parallel()
		equal, err := userpassword.Compare("invalidhash", "password")
		require.False(t, equal)
		require.Error(t, err)
	})

	t.Run("InvalidParts", func(t *testing.T) {
		t.Parallel()
		equal, err := userpassword.Compare("abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz", "test")
		require.False(t, equal)
		require.Error(t, err)
	})
}
