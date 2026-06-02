package agentcontext_test

import (
	"runtime"
	"testing"
)

// switchHomeEnv overrides the platform-specific environment variable
// consulted by os.UserHomeDir for the duration of the test. Windows
// reads USERPROFILE; Linux and macOS read HOME.
func switchHomeEnv(t *testing.T, dir string) {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("USERPROFILE", dir)
	default:
		t.Setenv("HOME", dir)
	}
}
