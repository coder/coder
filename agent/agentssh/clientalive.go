package agentssh

import (
	"strconv"
	"strings"
	"time"
)

const (
	// ClientAliveIntervalEnvironmentVariable and
	// ClientAliveCountMaxEnvironmentVariable let a client that reconnects on
	// its own ask the agent to close its connection after CountMax unanswered
	// keep-alives, one every Interval seconds, like OpenSSH's ClientAlive*.
	ClientAliveIntervalEnvironmentVariable = "CODER_SSH_CLIENT_ALIVE_INTERVAL"
	ClientAliveCountMaxEnvironmentVariable = "CODER_SSH_CLIENT_ALIVE_COUNT_MAX"

	minClientAliveInterval = 5 * time.Second
)

// extractClientAlive strips both variables from env. It returns a zero
// interval unless both are positive integers and the interval is shorter than
// agentInterval, if set, so a client can only probe more often than the agent.
func extractClientAlive(env []string, agentInterval time.Duration) (interval time.Duration, countMax int, filteredEnv []string) {
	var rawInterval, rawCountMax string
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, ClientAliveIntervalEnvironmentVariable+"="); ok {
			rawInterval = v
		} else if v, ok := strings.CutPrefix(kv, ClientAliveCountMaxEnvironmentVariable+"="); ok {
			rawCountMax = v
		} else {
			filteredEnv = append(filteredEnv, kv)
		}
	}
	// 16 bits keeps the duration from overflowing.
	seconds, err := strconv.ParseUint(rawInterval, 10, 16)
	if err != nil || seconds == 0 {
		return 0, 0, filteredEnv
	}
	count, err := strconv.ParseUint(rawCountMax, 10, 16)
	if err != nil || count == 0 {
		return 0, 0, filteredEnv
	}
	interval = max(time.Duration(seconds)*time.Second, minClientAliveInterval)
	if agentInterval > 0 && interval >= agentInterval {
		return 0, 0, filteredEnv
	}
	return interval, int(count), filteredEnv
}
