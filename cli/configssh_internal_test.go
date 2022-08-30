package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// This test tries to mimic the behavior of OpenSSH
// when executing e.g. a ProxyCommand.
func Test_sshConfigExecEscape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantErr bool
		windows bool
	}{
		{"no spaces", "simple", false, true},
		{"spaces", "path with spaces", false, true},
		{"quotes", "path with \"quotes\"", false, false},
		{"backslashes", "path with \\backslashes", false, false},
		{"tabs", "path with \ttabs", false, false},
		{"newline fails", "path with \nnewline", true, false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if runtime.GOOS == "windows" && !tt.windows {
				t.SkipNow()
			}

			dir := filepath.Join(t.TempDir(), tt.path)
			err := os.MkdirAll(dir, 0o755)
			require.NoError(t, err)
			bin := filepath.Join(dir, "coder")
			contents := []byte("#!/bin/sh\necho yay\n")
			if runtime.GOOS == "windows" {
				contents = []byte("cls\r\n@echo off\r\necho \"yay\"\r\n")
			}
			err = os.WriteFile(bin, contents, 0o755) //nolint:gosec
			require.NoError(t, err)

			escaped, err := sshConfigExecEscape(bin)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			args := []string{"/bin/sh", "-c", escaped}
			if runtime.GOOS == "windows" {
				args = []string{"cmd.exe", "/c", escaped}
			}
			b, err := exec.Command(args[0], args[1:]...).CombinedOutput() //nolint:gosec
			require.NoError(t, err)
			got := strings.TrimSpace(string(b))
			require.Equal(t, "yay", got)
		})
	}
}
