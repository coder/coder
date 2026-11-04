package agent

import (
	"fmt"
	"strconv"
	"strings"
)

// GitCredentialHelperCommand returns the credential.helper value that runs
// "coder gitcredential" from the agent binary at executablePath. The leading
// "!" makes Git run it through the shell, so a quoted path with spaces works.
// Git on Windows resolves UNIX-style paths, as with GIT_SSH_COMMAND.
func GitCredentialHelperCommand(executablePath string) string {
	unixPath := strings.ReplaceAll(executablePath, "\\", "/")
	return fmt.Sprintf("!'%s' gitcredential", strings.ReplaceAll(unixPath, "'", `'\''`))
}

// appendGitConfigEnv adds key=value to the Git configuration passed through
// GIT_CONFIG_COUNT, GIT_CONFIG_KEY_<n>, and GIT_CONFIG_VALUE_<n> (Git 2.31+).
// It appends after any entries already present instead of replacing them,
// because templates, users, and tools such as Claude Code use the same
// variables. Git reads these entries after every config file, so multi-valued
// keys like credential.helper land after the user's own helpers.
//
// It leaves envs unchanged if the entry already exists (for example when the
// agent's own environment was inherited) or if GIT_CONFIG_COUNT is not a valid
// count, which Git would reject anyway.
func appendGitConfigEnv(envs map[string]string, key, value string) {
	count := 0
	if raw, ok := envs["GIT_CONFIG_COUNT"]; ok && raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return
		}
		count = n
	}
	for i := range count {
		if envs[fmt.Sprintf("GIT_CONFIG_KEY_%d", i)] == key && envs[fmt.Sprintf("GIT_CONFIG_VALUE_%d", i)] == value {
			return
		}
	}
	envs[fmt.Sprintf("GIT_CONFIG_KEY_%d", count)] = key
	envs[fmt.Sprintf("GIT_CONFIG_VALUE_%d", count)] = value
	envs["GIT_CONFIG_COUNT"] = strconv.Itoa(count + 1)
}
