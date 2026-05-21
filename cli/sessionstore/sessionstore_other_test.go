//go:build !windows && !darwin

package sessionstore_test

import "testing"

func readRawKeychainCredential(t *testing.T, _ string) []byte {
	t.Fatal("not implemented")
	return nil
}
