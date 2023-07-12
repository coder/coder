package usershell

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
)

// Get returns the $SHELL environment variable.
func Get(username string) (string, error) {
	// This command will output "UserShell: /bin/zsh" if successful, we
	// can ignore the error since we have fallback behavior.
	out, _ := exec.Command("dscl", ".", "-read", filepath.Join("/Users", username), "UserShell").Output()
	s, ok := strings.CutPrefix(string(out), "UserShell: ")
	if ok {
		return strings.TrimSpace(s), nil
	}
	if s = os.Getenv("SHELL"); s != "" {
		return s, nil
	}
	return "", xerrors.Errorf("shell for user %q not found via dscl or in $SHELL", username)
}
