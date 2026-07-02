package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
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

// This test tries to mimic the behavior of OpenSSH when executing e.g. a ProxyCommand.
// nolint:paralleltest
func Test_sshConfigProxyCommandEscape(t *testing.T) {
	// Don't run this test, or any of its subtests in parallel. The test works by writing a file and then immediately
	// executing it.  Other tests might also exec a subprocess, and if they do in parallel, there is a small race
	// condition where our file is open when they fork, and remains open while we attempt to execute it, causing
	// a "text file busy" error.

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
	// nolint:paralleltest // Fixes a flake
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			escaped, err := sshConfigProxyCommandEscape(bin, false)
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

// This test tries to mimic the behavior of OpenSSH
// when executing e.g. a match exec command.
// nolint:tparallel
func Test_sshConfigMatchExecEscape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		path           string
		wantErrOther   bool
		wantErrWindows bool
	}{
		{"no spaces", "simple", false, false},
		{"spaces", "path with spaces", false, false},
		{"quotes", "path with \"quotes\"", true, true},
		{"backslashes", "path with\\backslashes", false, false},
		{"tabs", "path with \ttabs", false, true},
		{"newline fails", "path with \nnewline", true, true},
	}
	// nolint:paralleltest // Fixes a flake
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := "/bin/sh"
			arg := "-c"
			contents := []byte("#!/bin/sh\necho yay\n")
			if runtime.GOOS == "windows" {
				cmd = "cmd.exe"
				arg = "/c"
				contents = []byte("@echo yay\n")
			}

			dir := filepath.Join(t.TempDir(), tt.path)
			bin := filepath.Join(dir, "coder.bat") // Windows will treat it as batch, Linux doesn't care
			escaped, err := sshConfigMatchExecEscape(bin)
			if (runtime.GOOS == "windows" && tt.wantErrWindows) || (runtime.GOOS != "windows" && tt.wantErrOther) {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			err = os.MkdirAll(dir, 0o755)
			require.NoError(t, err)

			err = os.WriteFile(bin, contents, 0o755) //nolint:gosec
			require.NoError(t, err)

			// OpenSSH processes %% escape sequences into %
			escaped = strings.ReplaceAll(escaped, "%%", "%")
			b, err := exec.Command(cmd, arg, escaped).CombinedOutput() //nolint:gosec
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
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			found, err := sshConfigProxyCommandEscape(tt.path, tt.forceUnix)
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

func Test_mergeSSHOptions_RejectsUnsafeServerConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		coderd  codersdk.SSHConfigResponse
		wantErr string
	}{
		{
			name: "HostnameSuffix",
			coderd: codersdk.SSHConfigResponse{
				HostnameSuffix: "coder\nHost *",
			},
			wantErr: "workspace hostname suffix",
		},
		{
			name: "HostnamePrefix",
			coderd: codersdk.SSHConfigResponse{
				HostnamePrefix: "coder.\nHost *",
			},
			wantErr: "workspace hostname prefix",
		},
		{
			name: "ProxyCommand",
			coderd: codersdk.SSHConfigResponse{
				SSHConfigOptions: map[string]string{"ProxyCommand": "ssh -W %h:%p bastion"},
			},
			wantErr: `ssh config option "ProxyCommand" is not allowed`,
		},
		{
			name: "PermitLocalCommand",
			coderd: codersdk.SSHConfigResponse{
				SSHConfigOptions: map[string]string{"PermitLocalCommand": "yes"},
			},
			wantErr: `ssh config option "PermitLocalCommand" is not allowed`,
		},
		{
			name: "KnownHostsCommand",
			coderd: codersdk.SSHConfigResponse{
				SSHConfigOptions: map[string]string{"KnownHostsCommand": "echo key"},
			},
			wantErr: `ssh config option "KnownHostsCommand" is not allowed`,
		},
		{
			name: "PKCS11Provider",
			coderd: codersdk.SSHConfigResponse{
				SSHConfigOptions: map[string]string{"PKCS11Provider": "/tmp/evil.so"},
			},
			wantErr: `ssh config option "PKCS11Provider" is not allowed`,
		},
		{
			name: "NewlineInValue",
			coderd: codersdk.SSHConfigResponse{
				SSHConfigOptions: map[string]string{"UserKnownHostsFile": "/tmp/known_hosts\nHost *"},
			},
			wantErr: `ssh config option "UserKnownHostsFile" must not contain carriage return, newline, or NUL characters`,
		},
		{
			name: "SmartcardDevice",
			coderd: codersdk.SSHConfigResponse{
				SSHConfigOptions: map[string]string{"SmartcardDevice": "/path/to/lib"},
			},
			wantErr: `not allowed`,
		},
		{
			name: "XAuthLocation",
			coderd: codersdk.SSHConfigResponse{
				SSHConfigOptions: map[string]string{"XAuthLocation": "/usr/bin/xauth"},
			},
			wantErr: `not allowed`,
		},
		{
			name: "ProxyJump",
			coderd: codersdk.SSHConfigResponse{
				SSHConfigOptions: map[string]string{"ProxyJump": "bastion.example.com"},
			},
			wantErr: `conflicts with`,
		},
		{
			name: "HostnameSuffixGlob",
			coderd: codersdk.SSHConfigResponse{
				HostnameSuffix: "*",
			},
			wantErr: `glob`,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := mergeSSHOptions(sshConfigOptions{}, tt.coderd, t.TempDir(), "/tmp/coder")
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func Test_mergeSSHOptions_UserOptionsOverrideServerConfig(t *testing.T) {
	t.Parallel()

	user := sshConfigOptions{
		userHostPrefix: "dev.",
		hostnameSuffix: "local",
	}
	got, err := mergeSSHOptions(user, codersdk.SSHConfigResponse{
		HostnamePrefix: "coder.",
		HostnameSuffix: "coder",
	}, t.TempDir(), "/tmp/coder")
	require.NoError(t, err)
	require.Equal(t, "dev.", got.userHostPrefix)
	require.Equal(t, "local", got.hostnameSuffix)
}

func Test_mergeSSHOptions_AllowsSafeServerConfig(t *testing.T) {
	t.Parallel()

	got, err := mergeSSHOptions(sshConfigOptions{}, codersdk.SSHConfigResponse{
		HostnamePrefix: "coder.",
		HostnameSuffix: "coder",
		SSHConfigOptions: map[string]string{
			"HostName":           "example.com",
			"User":               "coder",
			"Port":               "22",
			"SetEnv":             "FOO=bar BAZ=qux",
			"UserKnownHostsFile": "/tmp/coder_known_hosts",
		},
	}, t.TempDir(), "/tmp/coder")
	require.NoError(t, err)
	require.Equal(t, "coder.", got.userHostPrefix)
	require.Equal(t, "coder", got.hostnameSuffix)
	require.Contains(t, got.sshOptions, "HostName example.com")
	require.Contains(t, got.sshOptions, "SetEnv FOO=bar BAZ=qux")
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
			Name: "AddTwo",
			Start: []string{
				"foo bar",
			},
			Add: []string{"Foo baz"},
			Expect: []string{
				"foo bar",
				"Foo baz",
			},
		},
		{
			Name: "AddAndRemove",
			Start: []string{
				"foo bar",
				"buzz bazz",
			},
			Add: []string{
				"b c",
				"a ", // Empty value, means remove all following entries that start with "a", i.e. next line.
				"A hello",
				"hello world",
			},
			Expect: []string{
				"foo bar",
				"buzz bazz",
				"b c",
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
			slices.Sort(tt.Expect)
			slices.Sort(o.sshOptions)
			require.Equal(t, tt.Expect, o.sshOptions)
		})
	}
}

func TestSSHConfigOptions_writeToBuffer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    sshConfigOptions
		want    []string // substrings that must appear
		notWant []string // substrings that must not appear
	}{
		{
			name: "wildcard suffix",
			opts: sshConfigOptions{
				coderBinaryPath:  "/usr/bin/coder",
				globalConfigPath: "/tmp/coder",
				hostnameSuffix:   "coder",
				waitEnum:         "auto",
			},
			want:    []string{"Host *.coder\n", "ProxyCommand", "--hostname-suffix coder %h"},
			notWant: []string{"Host workspace"},
		},
		{
			name: "wildcard prefix",
			opts: sshConfigOptions{
				coderBinaryPath:  "/usr/bin/coder",
				globalConfigPath: "/tmp/coder",
				userHostPrefix:   "coder.",
				waitEnum:         "auto",
			},
			want:    []string{"Host coder.*\n", "ProxyCommand", "--ssh-host-prefix coder. %h"},
			notWant: []string{"Host coder.workspace"},
		},
		{
			name: "no-wildcard suffix with workspaces",
			opts: sshConfigOptions{
				coderBinaryPath:  "/usr/bin/coder",
				globalConfigPath: "/tmp/coder",
				hostnameSuffix:   "coder",
				noWildcard:       true,
				workspaceNames:   []string{"workspace1", "workspace2"},
				waitEnum:         "auto",
			},
			want: []string{
				"Host workspace1.coder\n",
				"Host workspace2.coder\n",
				"Match host workspace1.coder !exec",
				"Match host workspace2.coder !exec",
				"--hostname-suffix coder %h",
			},
			notWant: []string{"Host *.coder", "Match host *.coder"},
		},
		{
			name: "no-wildcard suffix with zero workspaces produces no host entries",
			opts: sshConfigOptions{
				coderBinaryPath:  "/usr/bin/coder",
				globalConfigPath: "/tmp/coder",
				hostnameSuffix:   "coder",
				noWildcard:       true,
				workspaceNames:   nil,
				waitEnum:         "auto",
			},
			notWant: []string{"Host", "ProxyCommand", "Match"},
		},
		{
			name: "no-wildcard prefix with workspaces",
			opts: sshConfigOptions{
				coderBinaryPath:  "/usr/bin/coder",
				globalConfigPath: "/tmp/coder",
				userHostPrefix:   "coder.",
				noWildcard:       true,
				workspaceNames:   []string{"workspace1", "workspace2"},
				waitEnum:         "auto",
			},
			want: []string{
				"Host coder.workspace1\n",
				"Host coder.workspace2\n",
				"--ssh-host-prefix coder. %h",
			},
			notWant: []string{"Host coder.*"},
		},
		{
			name: "no-wildcard suffix skips proxy command when skipProxyCommand is set",
			opts: sshConfigOptions{
				coderBinaryPath:  "/usr/bin/coder",
				globalConfigPath: "/tmp/coder",
				hostnameSuffix:   "coder",
				noWildcard:       true,
				workspaceNames:   []string{"workspace1"},
				skipProxyCommand: true,
				waitEnum:         "auto",
			},
			want:    []string{"Host workspace1.coder\n"},
			notWant: []string{"ProxyCommand", "Match host", "Host *.coder"},
		},
		{
			name: "no-wildcard prefix skips proxy command when skipProxyCommand is set",
			opts: sshConfigOptions{
				coderBinaryPath:  "/usr/bin/coder",
				globalConfigPath: "/tmp/coder",
				userHostPrefix:   "coder.",
				noWildcard:       true,
				workspaceNames:   []string{"workspace1"},
				skipProxyCommand: true,
				waitEnum:         "auto",
			},
			want:    []string{"Host coder.workspace1\n"},
			notWant: []string{"ProxyCommand", "Host coder.*"},
		},
		{
			name: "no-wildcard suffix SSH options appear in every workspace entry",
			opts: sshConfigOptions{
				coderBinaryPath:  "/usr/bin/coder",
				globalConfigPath: "/tmp/coder",
				hostnameSuffix:   "coder",
				noWildcard:       true,
				workspaceNames:   []string{"workspace1", "workspace2"},
				sshOptions:       []string{"ForwardAgent=yes", "LogLevel=DEBUG"},
				waitEnum:         "auto",
			},
			want: []string{
				"Host workspace1.coder\n",
				"\tForwardAgent=yes\n",
				"\tLogLevel=DEBUG\n",
				"Host workspace2.coder\n",
			},
		},
		{
			name: "wildcard suffix SSH options appear in host block",
			opts: sshConfigOptions{
				coderBinaryPath:  "/usr/bin/coder",
				globalConfigPath: "/tmp/coder",
				hostnameSuffix:   "coder",
				sshOptions:       []string{"ForwardAgent=yes"},
				waitEnum:         "auto",
			},
			want: []string{
				"Host *.coder\n",
				"\tForwardAgent=yes\n",
			},
		},
		{
			name: "no-wildcard with both prefix and suffix generates entries for both",
			opts: sshConfigOptions{
				coderBinaryPath:  "/usr/bin/coder",
				globalConfigPath: "/tmp/coder",
				userHostPrefix:   "coder.",
				hostnameSuffix:   "testy",
				noWildcard:       true,
				workspaceNames:   []string{"workspace1"},
				waitEnum:         "auto",
			},
			want: []string{
				"Host coder.workspace1\n",
				"Host workspace1.testy\n",
				"Match host workspace1.testy !exec",
			},
			notWant: []string{"Host coder.*", "Host *.testy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := tt.opts.writeToBuffer(&buf)
			require.NoError(t, err)

			got := buf.String()
			for _, w := range tt.want {
				assert.Contains(t, got, w, "expected substring not found")
			}
			for _, nw := range tt.notWant {
				assert.NotContains(t, got, nw, "unexpected substring found")
			}
		})
	}
}
