package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_sshConfigExecEscape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"no spaces", "simple", false},
		{"spaces", "path with spaces", false},
		{"quotes", "path with \"quotes\"", false},
		{"backslashes", "path with \\backslashes", false},
		{"tabs", "path with \ttabs", false},
		{"newline fails", "path with \nnewline", true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join(t.TempDir(), tt.path)
			err := os.MkdirAll(dir, 0o755)
			require.NoError(t, err)
			bin := filepath.Join(dir, "coder")
			err = os.WriteFile(bin, []byte("#!/bin/sh\necho yay\n"), 0o755) //nolint:gosec
			require.NoError(t, err)

			escaped, err := sshConfigExecEscape(bin)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			b, err := exec.Command("/bin/sh", "-c", escaped).Output()
			require.NoError(t, err)
			got := strings.TrimSpace(string(b))
			require.Equal(t, "yay", got)
		})
	}
}
