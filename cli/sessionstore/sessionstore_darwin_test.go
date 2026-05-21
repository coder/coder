//go:build darwin

package sessionstore_test

import (
	"encoding/base64"
	"os/exec"
	"testing"
)

const (
	execPathKeychain = "/usr/bin/security"
	fixedUsername    = "coder-login-credentials"
)

func readRawKeychainCredential(t *testing.T, service string) []byte {
	t.Helper()

	out, err := exec.Command(
		execPathKeychain,
		"find-generic-password",
		"-s", service,
		"-wa", fixedUsername).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	dst := make([]byte, base64.StdEncoding.DecodedLen(len(out)))
	n, err := base64.StdEncoding.Decode(dst, out)
	if err != nil {
		t.Fatal(err)
	}
	return dst[:n]
}
