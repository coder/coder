package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func init() {
	// For golden files, always show the flag.
	hideForceUnixSlashes = false
}

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
	}{
		{"windows path", `C:\Program Files\Coder\bin\coder.exe`, false},
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

			escaped, err := sshConfigExecEscape(bin, false)
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

func Test_sshConfigExecEscapeSeparatorForce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		// Behavior is different on Windows
		expWindowsPath string
		expOtherPath   string
		forceUnix      bool
		wantErr        bool
	}{
		{
			name: "windows_keep_forward_slashes_with_spaces",
			// Has a space, expect quotes
			path:           `C:\Program Files\Coder\bin\coder.exe`,
			expWindowsPath: `"C:\Program Files\Coder\bin\coder.exe"`,
			expOtherPath:   `"C:\Program Files\Coder\bin\coder.exe"`,
			forceUnix:      false,
			wantErr:        false,
		},
		{
			name:           "windows_keep_forward_slashes",
			path:           `C:\ProgramFiles\Coder\bin\coder.exe`,
			expWindowsPath: `C:\ProgramFiles\Coder\bin\coder.exe`,
			expOtherPath:   `C:\ProgramFiles\Coder\bin\coder.exe`,
			forceUnix:      false,
			wantErr:        false,
		},
		{
			name:           "windows_force_unix_with_spaces",
			path:           `C:\Program Files\Coder\bin\coder.exe`,
			expWindowsPath: `"C:/Program Files/Coder/bin/coder.exe"`,
			expOtherPath:   `"C:\Program Files\Coder\bin\coder.exe"`,
			forceUnix:      true,
			wantErr:        false,
		},
		{
			name:           "windows_force_unix",
			path:           `C:\ProgramFiles\Coder\bin\coder.exe`,
			expWindowsPath: `C:/ProgramFiles/Coder/bin/coder.exe`,
			expOtherPath:   `C:\ProgramFiles\Coder\bin\coder.exe`,
			forceUnix:      true,
			wantErr:        false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			found, err := sshConfigExecEscape(tt.path, tt.forceUnix)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if runtime.GOOS == "windows" {
				require.Equal(t, tt.expWindowsPath, found, "(Windows) expected path")
			} else {
				// this is a noop on non-windows!
				require.Equal(t, tt.expOtherPath, found, "(Non-Windows) expected path")
			}
		})
	}
}

func Test_sshConfigOptions_addOption(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name        string
		Start       []string
		Add         []string
		Expect      []string
		ExpectError bool
	}{
		{
			Name: "Empty",
		},
		{
			Name: "AddOne",
			Add:  []string{"foo bar"},
			Expect: []string{
				"foo bar",
			},
		},
		{
			Name: "Replace",
			Start: []string{
				"foo bar",
			},
			Add: []string{"Foo baz"},
			Expect: []string{
				"Foo baz",
			},
		},
		{
			Name: "AddAndReplace",
			Start: []string{
				"a b",
				"foo bar",
				"buzz bazz",
			},
			Add: []string{
				"b c",
				"A hello",
				"hello world",
			},
			Expect: []string{
				"foo bar",
				"buzz bazz",
				"b c",
				"A hello",
				"hello world",
			},
		},
		{
			Name:        "Error",
			Add:         []string{"novalue"},
			ExpectError: true,
		},
	}

	for _, tt := range testCases {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()

			o := sshConfigOptions{
				sshOptions: tt.Start,
			}
			err := o.addOptions(tt.Add...)
			if tt.ExpectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			sort.Strings(tt.Expect)
			sort.Strings(o.sshOptions)
			require.Equal(t, tt.Expect, o.sshOptions)
		})
	}
}
