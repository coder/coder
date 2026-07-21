package codersdk

import (
	"regexp"
	"strings"

	"golang.org/x/xerrors"
)

const (
	// maxFilePathLength is the maximum length of a file path for
	// a user secret. Matches Linux PATH_MAX, which is the common
	// case since workspace agents almost always run on Linux.
	// This does not catch all Windows path length edge cases
	// (legacy MAX_PATH is 260), but the agent will surface a
	// runtime error if the write fails.
	maxFilePathLength = 4096
)

// MaxUserSecretsPerUserCount caps the number of secrets a single user
// may own.
//
// Why a cap exists at all: user_secrets is user-scoped, so every
// workspace the user owns loads the same set into its agent
// manifest, and env-injected ones land in the workspace agent's
// process env. Without a cap, a user can overflow one of three
// external limits by accumulating enough secrets, or by making
// them large enough. The failure surfaces at workspace start (or
// as a truncated env), not at create-time.
//
// What drives each cap, and the rough math:
//
//   - Count (50): backstops row-count growth from many small
//     secrets. The total-bytes cap binds first for large secrets;
//     this cap binds first for typical-sized ones (~few KB).
//
//   - Total bytes (200 KiB): sized to cover realistic credential
//     storage (API keys, SSH keys, kubeconfigs, cert bundles)
//     with headroom. Well under the 4 MiB DRPC agent manifest
//     budget (codersdk/drpcsdk.MaxMessageSize).
//
//   - Env bytes (24 KiB): an approximate budget for the value
//     bytes of env-injected secrets. Leaves ~8 KiB of headroom
//     under the ~32 KiB Windows process env block
//     (CreateProcessW's lpEnvironment is capped at 32,767
//     characters) for what this aggregate does not count:
//     env_name bytes, per-entry overhead, agent-injected vars
//     (CODER_*, PATH, HOME, ...), and template-defined env. Not
//     a strict overflow guarantee. Linux/macOS ARG_MAX (~2 MiB)
//     is far above this, so one Windows-safe cap works
//     everywhere.
//
// Byte caps measure stored bytes (octet_length of encrypted+base64).
// Plaintext is slightly tighter in encrypted deployments. That is
// fine: the limits we defend all measure transmitted bytes, and
// stored bytes upper-bound those.
//
// The Postgres trigger enforce_user_secrets_per_user_limits is the
// source of truth; the HTTP handler maps its check_violation to a
// 400. TestUserSecretLimits in coderd/usersecrets_test.go exercises
// off-by-one at each cap across POST and PATCH, so any drift
// between these constants and the trigger's literals fails an
// assertion.
const MaxUserSecretsPerUserCount = 50

// MaxUserSecretsTotalValueBytes caps the sum of stored value bytes
// per user. See MaxUserSecretsPerUserCount for the full rationale and
// math behind all three caps.
const MaxUserSecretsTotalValueBytes = 200 * 1024 // 200 KiB

// MaxUserSecretValueBytes is the maximum number of bytes for a
// single secret value. It is enforced in two places:
//
//   - The HTTP handler validates the raw (plaintext) value with
//     UserSecretValueValid before the row is written.
//   - The Postgres trigger enforce_user_secrets_per_user_limits
//     enforces the same number as an aggregate on stored bytes
//     across a user's env-injected secrets. This defends the
//     ~32 KiB Windows process env block.
//
// On deployments with secret encryption enabled, stored bytes
// exceed plaintext by ~1.33x (AES-GCM + base64), so the trigger's
// env-aggregate budget can be reached at less plaintext than the
// handler's per-value check would suggest. The trigger is
// authoritative; the handler's check is a fast pre-flight that
// catches the common "one value is too big" case before the row
// is encrypted and sent to the DB.
//
// One number serves both roles because the per-value cap can't
// usefully exceed the smallest aggregate cap any single row could
// trip: a value bigger than the env aggregate would be rejected
// the moment its env_name was set, so allowing it at the per-value
// layer would just move the failure later.
//
// See MaxUserSecretsPerUserCount for the rationale behind the other
// two caps (count, total bytes).
const MaxUserSecretValueBytes = 24 * 1024 // 24 KiB

// MaxUserSecretEnvNameLength caps the length of an env_name when one
// is provided. 256 is a generous round number that should allow any
// realistic env name while still bounding inputs.
//
// This is a per-row syntactic check, not an aggregate. It does not
// interact with the env_bytes aggregate (which is itself an
// approximate budget; see MaxUserSecretsPerUserCount).
const MaxUserSecretEnvNameLength = 256

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

// UserSecret*Field constants are the canonical ValidationError.Field values
// for user secret fields. UserSecretNameField is also the chi URL parameter
// name used in coderd route segments.
const (
	UserSecretNameField     = "name"
	UserSecretValueField    = "value"
	UserSecretEnvNameField  = "env_name"
	UserSecretFilePathField = "file_path"
)

// ValidateCreateUserSecretRequest validates a single create-secret request.
func ValidateCreateUserSecretRequest(req CreateUserSecretRequest) []ValidationError {
	var validations []ValidationError
	if err := UserSecretNameValid(req.Name); err != nil {
		validations = append(validations, ValidationError{Field: UserSecretNameField, Detail: err.Error()})
	}
	if req.Value == "" {
		validations = append(validations, ValidationError{Field: UserSecretValueField, Detail: "Value is required."})
	} else if err := UserSecretValueValid(req.Value); err != nil {
		validations = append(validations, ValidationError{Field: UserSecretValueField, Detail: err.Error()})
	}
	if err := UserSecretEnvNameValid(req.EnvName); err != nil {
		validations = append(validations, ValidationError{Field: UserSecretEnvNameField, Detail: err.Error()})
	}
	if err := UserSecretFilePathValid(req.FilePath); err != nil {
		validations = append(validations, ValidationError{Field: UserSecretFilePathField, Detail: err.Error()})
	}
	return validations
}

// UserSecretNameValid validates a user secret name. Names are used in
// API route path segments, so they must not include route separators.
func UserSecretNameValid(s string) error {
	if strings.TrimSpace(s) == "" {
		return xerrors.New("Name is required.")
	}

	if strings.TrimSpace(s) != s {
		return xerrors.New("Name must not have leading or trailing whitespace.")
	}

	if strings.ContainsAny(s, "/?#") {
		return xerrors.New("Name must not contain /, ?, or #.")
	}

	return nil
}

// UserSecretEnvNameValid validates an environment variable name for
// a user secret. Empty string is allowed (means no env injection).
func UserSecretEnvNameValid(s string) error {
	if s == "" {
		return nil
	}

	if len(s) > MaxUserSecretEnvNameLength {
		return xerrors.Errorf(
			"environment variable name must not exceed %d bytes",
			MaxUserSecretEnvNameLength,
		)
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

	return nil
}

// UserSecretFilePathValid validates a file path for a user secret.
// Empty string is allowed (means no file injection). Non-empty paths
// must start with ~/ or /, must not contain null bytes, and must not
// exceed 4096 bytes.
func UserSecretFilePathValid(s string) error {
	if s == "" {
		return nil
	}

	if !strings.HasPrefix(s, "~/") && !strings.HasPrefix(s, "/") {
		return xerrors.New("file path must start with ~/ or /")
	}

	if strings.Contains(s, "\x00") {
		return xerrors.New("file path must not contain null bytes")
	}

	if len(s) > maxFilePathLength {
		return xerrors.Errorf("file path must not exceed %d bytes", maxFilePathLength)
	}

	return nil
}

// UserSecretValueValid validates a user secret value as bytes
// submitted by the user (plaintext). The value must not contain
// null bytes and must not exceed MaxUserSecretValueBytes. The DB
// trigger separately enforces a stored-bytes env aggregate at the
// same numeric cap; under encryption the trigger may reject values
// that pass this check. See MaxUserSecretValueBytes for the
// dual-enforcement explanation.
func UserSecretValueValid(value string) error {
	if strings.Contains(value, "\x00") {
		return xerrors.New("secret value must not contain null bytes")
	}

	if len(value) > MaxUserSecretValueBytes {
		return xerrors.Errorf("secret value must not exceed %d bytes", MaxUserSecretValueBytes)
	}

	return nil
}
