// Package agentgitsign configures git commit and tag signing with the
// user's Coder SSH key through GIT_CONFIG_* environment variables, so no git
// config file is read or written. It requires git 2.31 or newer.
package agentgitsign

import (
	"fmt"
	"strconv"
	"strings"
)

// keys are the git config keys Append sets, in order. Strip relies on the
// order to recognize the entries Append added.
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

// Strip removes the entries Append added from a KEY=VALUE environment and
// leaves everything else, including earlier GIT_CONFIG_* entries, as is. It
// returns env unchanged when the last entries are not the ones Append adds.
func Strip(env []string) []string {
	values := make(map[string]string, len(env))
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		values[k] = v
	}
	n, err := strconv.Atoi(values["GIT_CONFIG_COUNT"])
	if err != nil || n < len(keys) {
		return env
	}
	first := n - len(keys)
	remove := make(map[string]bool, 2*len(keys)+1)
	for i, key := range keys {
		k := fmt.Sprintf("GIT_CONFIG_KEY_%d", first+i)
		if values[k] != key {
			return env
		}
		remove[k] = true
		remove[fmt.Sprintf("GIT_CONFIG_VALUE_%d", first+i)] = true
	}
	remove["GIT_CONFIG_COUNT"] = true

	out := make([]string, 0, len(env))
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if !remove[k] {
			out = append(out, kv)
		}
	}
	if first > 0 {
		out = append(out, "GIT_CONFIG_COUNT="+strconv.Itoa(first))
	}
	return out
}
