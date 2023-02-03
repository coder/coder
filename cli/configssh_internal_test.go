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

func Test_sshConfigSplitOnCoderSection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name    string
		Input   string
		Before  string
		Section string
		After   string
		Err     bool
	}{
		{
			Name:    "Empty",
			Input:   "",
			Before:  "",
			Section: "",
			After:   "",
			Err:     false,
		},
		{
			Name:    "JustSection",
			Input:   strings.Join([]string{sshStartToken, sshEndToken}, "\n"),
			Before:  "",
			Section: strings.Join([]string{sshStartToken, sshEndToken}, "\n"),
			After:   "",
			Err:     false,
		},
		{
			Name:    "NoSection",
			Input:   strings.Join([]string{"# Some content"}, "\n"),
			Before:  "# Some content",
			Section: "",
			After:   "",
			Err:     false,
		},
		{
			Name: "Normal",
			Input: strings.Join([]string{
				"# Content before the section",
				sshStartToken,
				sshEndToken,
				"# Content after the section",
			}, "\n"),
			Before:  "# Content before the section",
			Section: strings.Join([]string{"", sshStartToken, sshEndToken, ""}, "\n"),
			After:   "# Content after the section",
			Err:     false,
		},
		{
			Name: "OutOfOrder",
			Input: strings.Join([]string{
				"# Content before the section",
				sshEndToken,
				sshStartToken,
				"# Content after the section",
			}, "\n"),
			Err: true,
		},
		{
			Name: "MissingStart",
			Input: strings.Join([]string{
				"# Content before the section",
				sshEndToken,
				"# Content after the section",
			}, "\n"),
			Err: true,
		},
		{
			Name: "MissingEnd",
			Input: strings.Join([]string{
				"# Content before the section",
				sshEndToken,
				"# Content after the section",
			}, "\n"),
			Err: true,
		},
		{
			Name: "ExtraStart",
			Input: strings.Join([]string{
				"# Content before the section",
				sshStartToken,
				sshEndToken,
				sshStartToken,
				"# Content after the section",
			}, "\n"),
			Err: true,
		},
		{
			Name: "ExtraEnd",
			Input: strings.Join([]string{
				"# Content before the section",
				sshStartToken,
				sshEndToken,
				sshEndToken,
				"# Content after the section",
			}, "\n"),
			Err: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			before, section, after, err := sshConfigSplitOnCoderSection([]byte(tc.Input))
			if tc.Err {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.Before, string(before), "before")
			require.Equal(t, tc.Section, string(section), "section")
			require.Equal(t, tc.After, string(after), "after")
		})
	}
}

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

			if runtime.GOOS == "windows" {
				t.Skip("Windows doesn't typically execute via /bin/sh or cmd.exe, so this test is not applicable.")
			}

			dir := filepath.Join(t.TempDir(), tt.path)
			err := os.MkdirAll(dir, 0o755)
			require.NoError(t, err)
			bin := filepath.Join(dir, "coder")
			contents := []byte("#!/bin/sh\necho yay\n")
			err = os.WriteFile(bin, contents, 0o755) //nolint:gosec
			require.NoError(t, err)

			escaped, err := sshConfigExecEscape(bin)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			b, err := exec.Command("/bin/sh", "-c", escaped).CombinedOutput() //nolint:gosec
			require.NoError(t, err)
			got := strings.TrimSpace(string(b))
			require.Equal(t, "yay", got)
		})
	}
}
