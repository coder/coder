//go:build !windows && !darwin
// +build !windows,!darwin

package usershell

import (
	"os"
	"strings"

	"golang.org/x/xerrors"
)

// Get returns the /etc/passwd entry for the username provided.
func Get(username string) (string, error) {
	contents, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return "", xerrors.Errorf("read /etc/passwd: %w", err)
	}
	lines := strings.Split(string(contents), "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, username+":") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			return "", xerrors.Errorf("malformed user entry: %q", line)
		}
		return parts[6], nil
	}
	return "", xerrors.Errorf("user %q not found in /etc/passwd", username)
}
