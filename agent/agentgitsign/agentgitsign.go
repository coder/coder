// Package agentgitsign signs git commits and tags in a workspace with the
// user's Coder SSH key. It configures git through GIT_CONFIG_* environment
// variables, so no git config file is read or written, and signs in memory.
// SSH signing requires git 2.34 or newer.
package agentgitsign

import (
	"fmt"
	"strconv"
	"strings"
)

// keys are the git config keys Append sets, in order.
var keys = []string{
	"commit.gpgsign",
	"tag.gpgsign",
	"gpg.format",
	"gpg.ssh.program",
	"user.signingkey",
}

// Append adds the signing config to envs after any GIT_CONFIG_COUNT entries
// the user or template already set. program is run by git as
// gpg.ssh.program, and publicKey is the user's SSH public key.
func Append(envs map[string]string, program, publicKey string) {
	// An unparsable or negative count is replaced, since git rejects it anyway.
	n := 0
	if count, err := strconv.Atoi(envs["GIT_CONFIG_COUNT"]); err == nil && count > 0 {
		n = count
	}
	values := []string{"true", "true", "ssh", program, "key::" + strings.TrimSpace(publicKey)}
	for i, key := range keys {
		envs[fmt.Sprintf("GIT_CONFIG_KEY_%d", n)] = key
		envs[fmt.Sprintf("GIT_CONFIG_VALUE_%d", n)] = values[i]
		n++
	}
	envs["GIT_CONFIG_COUNT"] = strconv.Itoa(n)
}
