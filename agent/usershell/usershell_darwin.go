package usershell
import (
	"fmt"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)
// Get returns the $SHELL environment variable.
// Deprecated: use SystemEnvInfo.UserShell instead.
func Get(username string) (string, error) {
	// This command will output "UserShell: /bin/zsh" if successful, we
	// can ignore the error since we have fallback behavior.
	if !filepath.IsLocal(username) {
		return "", fmt.Errorf("username is nonlocal path: %s", username)
	}
	//nolint: gosec // input checked above
	out, _ := exec.Command("dscl", ".", "-read", filepath.Join("/Users", username), "UserShell").Output() //nolint:gocritic
	s, ok := strings.CutPrefix(string(out), "UserShell: ")
	if ok {
		return strings.TrimSpace(s), nil
	}
	if s = os.Getenv("SHELL"); s != "" {
		return s, nil
	}
	return "", fmt.Errorf("shell for user %q not found via dscl or in $SHELL", username)
}
