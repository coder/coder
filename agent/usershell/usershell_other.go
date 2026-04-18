//go:build !windows && !darwin
// +build !windows,!darwin

package usershell

import (
	"os"
	"strings"

	"golang.org/x/xerrors"
)

// Get returns the login shell of the username provided. This is taken
// from /etc/passwd, falling back to $SHELL if the /etc/passwd entry
// is a nologin shell or there is no entry for the given username.
// Deprecated: use SystemEnvInfo.UserShell instead.
func Get(username string) (string, error) {
	envShell := os.Getenv("SHELL")
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

		shell := parts[6]
		if strings.HasSuffix(shell, "nologin") {
			if envShell == "" {
				return "", xerrors.Errorf("user %q has invalid shell: %q", username, shell)
			}
			if strings.HasSuffix(envShell, "nologin") || envShell == "/bin/false" {
				return "", xerrors.Errorf("user %q has invalid shell in /etc/passwd: %q as well as $SHELL: %q", username, shell, envShell)
			}
			return envShell, nil
		}
		return shell, nil
	}
	if envShell == "" {
		return "", xerrors.Errorf("shell for user %q not found in /etc/passwd or $SHELL", username)
	}
	if strings.HasSuffix(envShell, "nologin") {
		return "", xerrors.Errorf("shell for user %q not found in /etc/passwd and invalid shell %q was defined in $SHELL", username, envShell)
	}
	return envShell, nil
}
