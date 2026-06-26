//go:build !windows && !darwin
// +build !windows,!darwin

package usershell

import (
	"os"
	"strings"

	"golang.org/x/xerrors"
)

// get resolves the user's shell from /etc/passwd, falling back to
// $SHELL. It backs SystemEnvInfo.Shell. Callers resolve the shell
// through an EnvInfoer.
func get(username string) (string, error) {
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
	if s := os.Getenv("SHELL"); s != "" {
		return s, nil
	}
	return "", xerrors.Errorf("shell for user %q not found in /etc/passwd or $SHELL", username)
}
