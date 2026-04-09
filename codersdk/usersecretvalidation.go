package codersdk

import (
	"regexp"
	"strings"

	"golang.org/x/xerrors"
)

// UserSecretEnvValidationOptions controls deployment-aware behavior
// in environment variable name validation.
type UserSecretEnvValidationOptions struct {
	// AIGatewayEnabled indicates that the deployment has AI Gateway
	// configured. When true, AI Gateway environment variables
	// (OPENAI_API_KEY, etc.) are reserved to prevent conflicts.
	AIGatewayEnabled bool
}

var (
	// posixEnvNameRegex matches valid POSIX environment variable names:
	// must start with a letter or underscore, followed by letters,
	// digits, or underscores.
	posixEnvNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

	// reservedEnvNames are system environment variables that must not
	// be overridden by user secrets. This list is intentionally
	// aggressive because it is easier to remove entries later than
	// to add them after users have already created conflicting
	// secrets.
	reservedEnvNames = map[string]struct{}{
		// Core POSIX/login variables. Overriding these breaks
		// basic shell and session behavior.
		"PATH":    {},
		"HOME":    {},
		"SHELL":   {},
		"USER":    {},
		"LOGNAME": {},
		"PWD":     {},
		"OLDPWD":  {},

		// Locale and terminal. Agents and IDEs depend on these
		// being set correctly by the system.
		"LANG": {},
		"TERM": {},

		// Shell behavior. Overriding these can silently break
		// word splitting, directory resolution, and script
		// execution in every shell session and agent script.
		"IFS":    {},
		"CDPATH": {},

		// Shell startup files. ENV is sourced by POSIX sh for
		// interactive shells; BASH_ENV is sourced by bash for
		// every non-interactive invocation (scripts, subshells).
		// Allowing users to set these would inject arbitrary
		// code into every shell and script in the workspace.
		"ENV":      {},
		"BASH_ENV": {},

		// Temp directories. Overriding these is a security risk
		// (symlink attacks, world-readable paths).
		"TMPDIR": {},
		"TMP":    {},
		"TEMP":   {},

		// Host identity.
		"HOSTNAME": {},

		// SSH session variables. The Coder agent sets
		// SSH_AUTH_SOCK in agentssh.go; the others are set by
		// sshd and should never be faked.
		"SSH_AUTH_SOCK":  {},
		"SSH_CLIENT":     {},
		"SSH_CONNECTION": {},
		"SSH_TTY":        {},

		// Editor/pager. The Coder agent sets these so that git
		// operations inside workspaces work non-interactively.
		"EDITOR": {},
		"VISUAL": {},
		"PAGER":  {},

		// IDE integration. The agent sets these for code-server
		// and VS Code Remote proxying.
		"VSCODE_PROXY_URI":                    {},
		"CS_DISABLE_GETTING_STARTED_OVERRIDE": {},

		// XDG base directories. Overriding these redirects
		// config, cache, and runtime data for every tool in the
		// workspace.
		"XDG_RUNTIME_DIR": {},
		"XDG_CONFIG_HOME": {},
		"XDG_DATA_HOME":   {},
		"XDG_CACHE_HOME":  {},
		"XDG_STATE_HOME":  {},

		// OIDC token. The Coder agent injects a short-lived
		// OIDC token for cloud auth flows (e.g. GCP workload
		// identity). Overriding it could break provisioner and
		// agent authentication.
		"OIDC_TOKEN": {},
	}

	// aiGatewayReservedEnvNames are reserved only when AI Gateway
	// is enabled on the deployment. When AI Gateway is disabled,
	// users may legitimately want to inject their own API keys
	// via secrets.
	aiGatewayReservedEnvNames = map[string]struct{}{
		"OPENAI_API_KEY":       {},
		"OPENAI_BASE_URL":      {},
		"ANTHROPIC_AUTH_TOKEN": {},
		"ANTHROPIC_BASE_URL":   {},
	}

	// reservedEnvPrefixes are namespace prefixes where every
	// variable in the family is reserved. Checked after the
	// exact-name map. The CODER / CODER_* namespace is handled
	// separately with its own error message (see below).
	reservedEnvPrefixes = []string{
		// The Coder agent sets GIT_SSH_COMMAND, GIT_ASKPASS,
		// GIT_AUTHOR_*, GIT_COMMITTER_*, and several others.
		// Blocking the entire GIT_* namespace avoids an arms
		// race with new git env vars.
		"GIT_",

		// Locale variables. LC_ALL, LC_CTYPE, LC_MESSAGES,
		// etc. control character encoding, sorting, and
		// formatting. Overriding them can break text
		// processing in agents and IDEs.
		"LC_",

		// Dynamic linker variables. Allowing users to set
		// these would let a secret inject arbitrary shared
		// libraries into every process in the workspace.
		"LD_",
		"DYLD_",
	}
)

// UserSecretEnvNameValid validates an environment variable name for
// a user secret. Empty string is allowed (means no env injection).
// The opts parameter controls deployment-aware checks such as AI
// bridge variable reservation.
func UserSecretEnvNameValid(s string, opts UserSecretEnvValidationOptions) error {
	if s == "" {
		return nil
	}

	if !posixEnvNameRegex.MatchString(s) {
		return xerrors.New("must start with a letter or underscore, followed by letters, digits, or underscores")
	}

	upper := strings.ToUpper(s)

	if _, ok := reservedEnvNames[upper]; ok {
		return xerrors.Errorf("%s is a reserved environment variable name", upper)
	}

	if upper == "CODER" || strings.HasPrefix(upper, "CODER_") {
		return xerrors.New("environment variable names starting with CODER_ are reserved for internal use")
	}

	for _, prefix := range reservedEnvPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return xerrors.Errorf("environment variables starting with %s are reserved", prefix)
		}
	}

	if opts.AIGatewayEnabled {
		if _, ok := aiGatewayReservedEnvNames[upper]; ok {
			return xerrors.Errorf("%s is reserved when AI Gateway is enabled", upper)
		}
	}

	return nil
}

// UserSecretFilePathValid validates a file path for a user secret.
// Empty string is allowed (means no file injection). Non-empty paths
// must start with ~/ or /.
func UserSecretFilePathValid(s string) error {
	if s == "" {
		return nil
	}

	if strings.HasPrefix(s, "~/") || strings.HasPrefix(s, "/") {
		return nil
	}

	return xerrors.New("file path must start with ~/ or /")
}
