package codersdk_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/codersdk"
)

func TestUserSecretEnvNameValid(t *testing.T) {
	t.Parallel()

	// noAIGateway is the default for most tests — AI Gateway disabled.
	noAIGateway := codersdk.UserSecretEnvValidationOptions{}
	withAIGateway := codersdk.UserSecretEnvValidationOptions{AIGatewayEnabled: true}

	tests := []struct {
		name    string
		input   string
		opts    codersdk.UserSecretEnvValidationOptions
		wantErr bool
		errMsg  string
	}{
		// Valid names.
		{name: "SimpleUpper", input: "GITHUB_TOKEN", opts: noAIGateway},
		{name: "SimpleLower", input: "github_token", opts: noAIGateway},
		{name: "StartsWithUnderscore", input: "_FOO", opts: noAIGateway},
		{name: "SingleChar", input: "A", opts: noAIGateway},
		{name: "WithDigits", input: "A1B2", opts: noAIGateway},
		{name: "Empty", input: "", opts: noAIGateway},

		// Invalid POSIX names.
		{name: "StartsWithDigit", input: "1FOO", opts: noAIGateway, wantErr: true, errMsg: "must start with"},
		{name: "ContainsHyphen", input: "FOO-BAR", opts: noAIGateway, wantErr: true, errMsg: "must start with"},
		{name: "ContainsDot", input: "FOO.BAR", opts: noAIGateway, wantErr: true, errMsg: "must start with"},
		{name: "ContainsSpace", input: "FOO BAR", opts: noAIGateway, wantErr: true, errMsg: "must start with"},

		// Reserved system names — core POSIX/login.
		{name: "ReservedPATH", input: "PATH", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedHOME", input: "HOME", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedSHELL", input: "SHELL", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedUSER", input: "USER", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedLOGNAME", input: "LOGNAME", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedPWD", input: "PWD", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedOLDPWD", input: "OLDPWD", opts: noAIGateway, wantErr: true, errMsg: "reserved"},

		// Reserved system names — locale/terminal.
		{name: "ReservedLANG", input: "LANG", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedTERM", input: "TERM", opts: noAIGateway, wantErr: true, errMsg: "reserved"},

		// Reserved system names — shell behavior.
		{name: "ReservedIFS", input: "IFS", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedCDPATH", input: "CDPATH", opts: noAIGateway, wantErr: true, errMsg: "reserved"},

		// Reserved system names — shell startup files.
		{name: "ReservedENV", input: "ENV", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedBASH_ENV", input: "BASH_ENV", opts: noAIGateway, wantErr: true, errMsg: "reserved"},

		// Reserved system names — temp directories.
		{name: "ReservedTMPDIR", input: "TMPDIR", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedTMP", input: "TMP", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedTEMP", input: "TEMP", opts: noAIGateway, wantErr: true, errMsg: "reserved"},

		// Reserved system names — host identity.
		{name: "ReservedHOSTNAME", input: "HOSTNAME", opts: noAIGateway, wantErr: true, errMsg: "reserved"},

		// Reserved system names — SSH.
		{name: "ReservedSSH_AUTH_SOCK", input: "SSH_AUTH_SOCK", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedSSH_CLIENT", input: "SSH_CLIENT", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedSSH_CONNECTION", input: "SSH_CONNECTION", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedSSH_TTY", input: "SSH_TTY", opts: noAIGateway, wantErr: true, errMsg: "reserved"},

		// Reserved system names — editor/pager.
		{name: "ReservedEDITOR", input: "EDITOR", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedVISUAL", input: "VISUAL", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedPAGER", input: "PAGER", opts: noAIGateway, wantErr: true, errMsg: "reserved"},

		// Reserved system names — IDE integration.
		{name: "ReservedVSCODE_PROXY_URI", input: "VSCODE_PROXY_URI", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedCS_DISABLE", input: "CS_DISABLE_GETTING_STARTED_OVERRIDE", opts: noAIGateway, wantErr: true, errMsg: "reserved"},

		// Reserved system names — XDG.
		{name: "ReservedXDG_RUNTIME_DIR", input: "XDG_RUNTIME_DIR", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedXDG_CONFIG_HOME", input: "XDG_CONFIG_HOME", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedXDG_DATA_HOME", input: "XDG_DATA_HOME", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedXDG_CACHE_HOME", input: "XDG_CACHE_HOME", opts: noAIGateway, wantErr: true, errMsg: "reserved"},
		{name: "ReservedXDG_STATE_HOME", input: "XDG_STATE_HOME", opts: noAIGateway, wantErr: true, errMsg: "reserved"},

		// Reserved system names — OIDC.
		{name: "ReservedOIDC_TOKEN", input: "OIDC_TOKEN", opts: noAIGateway, wantErr: true, errMsg: "reserved"},

		// AI Gateway vars — blocked when AI Gateway is enabled.
		{name: "AIGateway/OPENAI_API_KEY/Enabled", input: "OPENAI_API_KEY", opts: withAIGateway, wantErr: true, errMsg: "AI Gateway"},
		{name: "AIGateway/OPENAI_BASE_URL/Enabled", input: "OPENAI_BASE_URL", opts: withAIGateway, wantErr: true, errMsg: "AI Gateway"},
		{name: "AIGateway/ANTHROPIC_AUTH_TOKEN/Enabled", input: "ANTHROPIC_AUTH_TOKEN", opts: withAIGateway, wantErr: true, errMsg: "AI Gateway"},
		{name: "AIGateway/ANTHROPIC_BASE_URL/Enabled", input: "ANTHROPIC_BASE_URL", opts: withAIGateway, wantErr: true, errMsg: "AI Gateway"},

		// AI Gateway vars — allowed when AI Gateway is disabled.
		{name: "AIGateway/OPENAI_API_KEY/Disabled", input: "OPENAI_API_KEY", opts: noAIGateway},
		{name: "AIGateway/OPENAI_BASE_URL/Disabled", input: "OPENAI_BASE_URL", opts: noAIGateway},
		{name: "AIGateway/ANTHROPIC_AUTH_TOKEN/Disabled", input: "ANTHROPIC_AUTH_TOKEN", opts: noAIGateway},
		{name: "AIGateway/ANTHROPIC_BASE_URL/Disabled", input: "ANTHROPIC_BASE_URL", opts: noAIGateway},

		// Case insensitivity.
		{name: "ReservedCaseInsensitive", input: "path", opts: noAIGateway, wantErr: true, errMsg: "reserved"},

		// CODER_ prefix.
		{name: "CoderExact", input: "CODER", opts: noAIGateway, wantErr: true, errMsg: "CODER_"},
		{name: "CoderPrefix", input: "CODER_WORKSPACE_NAME", opts: noAIGateway, wantErr: true, errMsg: "CODER_"},
		{name: "CoderAgentToken", input: "CODER_AGENT_TOKEN", opts: noAIGateway, wantErr: true, errMsg: "CODER_"},
		{name: "CoderLowerCase", input: "coder_foo", opts: noAIGateway, wantErr: true, errMsg: "CODER_"},

		// GIT_* prefix.
		{name: "GitSSHCommand", input: "GIT_SSH_COMMAND", opts: noAIGateway, wantErr: true, errMsg: "GIT_"},
		{name: "GitAskpass", input: "GIT_ASKPASS", opts: noAIGateway, wantErr: true, errMsg: "GIT_"},
		{name: "GitAuthorName", input: "GIT_AUTHOR_NAME", opts: noAIGateway, wantErr: true, errMsg: "GIT_"},
		{name: "GitLowerCase", input: "git_editor", opts: noAIGateway, wantErr: true, errMsg: "GIT_"},

		// LC_* prefix (locale).
		{name: "LcAll", input: "LC_ALL", opts: noAIGateway, wantErr: true, errMsg: "LC_"},
		{name: "LcCtype", input: "LC_CTYPE", opts: noAIGateway, wantErr: true, errMsg: "LC_"},

		// LD_* prefix (dynamic linker).
		{name: "LdPreload", input: "LD_PRELOAD", opts: noAIGateway, wantErr: true, errMsg: "LD_"},
		{name: "LdLibraryPath", input: "LD_LIBRARY_PATH", opts: noAIGateway, wantErr: true, errMsg: "LD_"},

		// DYLD_* prefix (macOS dynamic linker).
		{name: "DyldInsert", input: "DYLD_INSERT_LIBRARIES", opts: noAIGateway, wantErr: true, errMsg: "DYLD_"},
		{name: "DyldLibraryPath", input: "DYLD_LIBRARY_PATH", opts: noAIGateway, wantErr: true, errMsg: "DYLD_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := codersdk.UserSecretEnvNameValid(tt.input, tt.opts)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := codersdk.UserSecretFilePathValid(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "must start with")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
