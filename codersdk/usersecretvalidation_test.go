package codersdk_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/codersdk"
)

func TestUserSecretEnvNameValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		// Valid names.
		{name: "SimpleUpper", input: "GITHUB_TOKEN"},
		{name: "SimpleLower", input: "github_token"},
		{name: "StartsWithUnderscore", input: "_FOO"},
		{name: "SingleChar", input: "A"},
		{name: "WithDigits", input: "A1B2"},
		{name: "Empty", input: ""},

		// Invalid POSIX names.
		{name: "StartsWithDigit", input: "1FOO", wantErr: true, errMsg: "must start with"},
		{name: "ContainsHyphen", input: "FOO-BAR", wantErr: true, errMsg: "must start with"},
		{name: "ContainsDot", input: "FOO.BAR", wantErr: true, errMsg: "must start with"},
		{name: "ContainsSpace", input: "FOO BAR", wantErr: true, errMsg: "must start with"},

		// Reserved system names — core POSIX/login.
		{name: "ReservedPATH", input: "PATH", wantErr: true, errMsg: "reserved"},
		{name: "ReservedHOME", input: "HOME", wantErr: true, errMsg: "reserved"},
		{name: "ReservedSHELL", input: "SHELL", wantErr: true, errMsg: "reserved"},
		{name: "ReservedUSER", input: "USER", wantErr: true, errMsg: "reserved"},
		{name: "ReservedLOGNAME", input: "LOGNAME", wantErr: true, errMsg: "reserved"},
		{name: "ReservedPWD", input: "PWD", wantErr: true, errMsg: "reserved"},
		{name: "ReservedOLDPWD", input: "OLDPWD", wantErr: true, errMsg: "reserved"},

		// Reserved system names — locale/terminal.
		{name: "ReservedLANG", input: "LANG", wantErr: true, errMsg: "reserved"},
		{name: "ReservedTERM", input: "TERM", wantErr: true, errMsg: "reserved"},

		// Reserved system names — shell behavior.
		{name: "ReservedIFS", input: "IFS", wantErr: true, errMsg: "reserved"},
		{name: "ReservedCDPATH", input: "CDPATH", wantErr: true, errMsg: "reserved"},

		// Reserved system names — shell startup files.
		{name: "ReservedENV", input: "ENV", wantErr: true, errMsg: "reserved"},
		{name: "ReservedBASH_ENV", input: "BASH_ENV", wantErr: true, errMsg: "reserved"},

		// Reserved system names — temp directories.
		{name: "ReservedTMPDIR", input: "TMPDIR", wantErr: true, errMsg: "reserved"},
		{name: "ReservedTMP", input: "TMP", wantErr: true, errMsg: "reserved"},
		{name: "ReservedTEMP", input: "TEMP", wantErr: true, errMsg: "reserved"},

		// Reserved system names — host identity.
		{name: "ReservedHOSTNAME", input: "HOSTNAME", wantErr: true, errMsg: "reserved"},

		// Reserved system names — SSH.
		{name: "ReservedSSH_AUTH_SOCK", input: "SSH_AUTH_SOCK", wantErr: true, errMsg: "reserved"},
		{name: "ReservedSSH_CLIENT", input: "SSH_CLIENT", wantErr: true, errMsg: "reserved"},
		{name: "ReservedSSH_CONNECTION", input: "SSH_CONNECTION", wantErr: true, errMsg: "reserved"},
		{name: "ReservedSSH_TTY", input: "SSH_TTY", wantErr: true, errMsg: "reserved"},

		// Reserved system names — editor/pager.
		{name: "ReservedEDITOR", input: "EDITOR", wantErr: true, errMsg: "reserved"},
		{name: "ReservedVISUAL", input: "VISUAL", wantErr: true, errMsg: "reserved"},
		{name: "ReservedPAGER", input: "PAGER", wantErr: true, errMsg: "reserved"},

		// Reserved system names — IDE integration.
		{name: "ReservedVSCODE_PROXY_URI", input: "VSCODE_PROXY_URI", wantErr: true, errMsg: "reserved"},
		{name: "ReservedCS_DISABLE", input: "CS_DISABLE_GETTING_STARTED_OVERRIDE", wantErr: true, errMsg: "reserved"},

		// Reserved system names — XDG.
		{name: "ReservedXDG_RUNTIME_DIR", input: "XDG_RUNTIME_DIR", wantErr: true, errMsg: "reserved"},
		{name: "ReservedXDG_CONFIG_HOME", input: "XDG_CONFIG_HOME", wantErr: true, errMsg: "reserved"},
		{name: "ReservedXDG_DATA_HOME", input: "XDG_DATA_HOME", wantErr: true, errMsg: "reserved"},
		{name: "ReservedXDG_CACHE_HOME", input: "XDG_CACHE_HOME", wantErr: true, errMsg: "reserved"},
		{name: "ReservedXDG_STATE_HOME", input: "XDG_STATE_HOME", wantErr: true, errMsg: "reserved"},

		// Case insensitivity.
		{name: "ReservedCaseInsensitive", input: "path", wantErr: true, errMsg: "reserved"},

		// CODER_ prefix.
		{name: "CoderExact", input: "CODER", wantErr: true, errMsg: "CODER_"},
		{name: "CoderPrefix", input: "CODER_WORKSPACE_NAME", wantErr: true, errMsg: "CODER_"},
		{name: "CoderAgentToken", input: "CODER_AGENT_TOKEN", wantErr: true, errMsg: "CODER_"},
		{name: "CoderLowerCase", input: "coder_foo", wantErr: true, errMsg: "CODER_"},

		// GIT_* prefix.
		{name: "GitSSHCommand", input: "GIT_SSH_COMMAND", wantErr: true, errMsg: "GIT_"},
		{name: "GitAskpass", input: "GIT_ASKPASS", wantErr: true, errMsg: "GIT_"},
		{name: "GitAuthorName", input: "GIT_AUTHOR_NAME", wantErr: true, errMsg: "GIT_"},
		{name: "GitLowerCase", input: "git_editor", wantErr: true, errMsg: "GIT_"},

		// LC_* prefix (locale).
		{name: "LcAll", input: "LC_ALL", wantErr: true, errMsg: "LC_"},
		{name: "LcCtype", input: "LC_CTYPE", wantErr: true, errMsg: "LC_"},

		// LD_* prefix (dynamic linker).
		{name: "LdPreload", input: "LD_PRELOAD", wantErr: true, errMsg: "LD_"},
		{name: "LdLibraryPath", input: "LD_LIBRARY_PATH", wantErr: true, errMsg: "LD_"},

		// DYLD_* prefix (macOS dynamic linker).
		{name: "DyldInsert", input: "DYLD_INSERT_LIBRARIES", wantErr: true, errMsg: "DYLD_"},
		{name: "DyldLibraryPath", input: "DYLD_LIBRARY_PATH", wantErr: true, errMsg: "DYLD_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := codersdk.UserSecretEnvNameValid(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUserSecretFilePathValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid paths.
		{name: "TildePath", input: "~/foo"},
		{name: "TildeSSH", input: "~/.ssh/id_rsa"},
		{name: "AbsolutePath", input: "/home/coder/.ssh/id_rsa"},
		{name: "RootPath", input: "/"},
		{name: "Empty", input: ""},

		// Invalid paths.
		{name: "BareRelative", input: "foo/bar", wantErr: true},
		{name: "DotRelative", input: ".ssh/id_rsa", wantErr: true},
		{name: "JustFilename", input: "credentials", wantErr: true},
		{name: "TildeNoSlash", input: "~foo", wantErr: true},
		{name: "NullByte", input: "/home/\x00coder", wantErr: true},
		{name: "TooLong", input: "/" + strings.Repeat("a", 4096), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := codersdk.UserSecretFilePathValid(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUserSecretValueValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "NormalString", input: "my-secret-token"},
		{name: "Empty", input: ""},
		{name: "WithNewlines", input: "line1\nline2\nline3"},
		{name: "WithTabs", input: "key\tvalue"},
		{name: "NullByte", input: "before\x00after", wantErr: true},
		{name: "ExactlyAtLimit", input: strings.Repeat("a", codersdk.MaxSecretValueSize)},
		{name: "OverLimit", input: strings.Repeat("a", codersdk.MaxSecretValueSize+1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := codersdk.UserSecretValueValid(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
