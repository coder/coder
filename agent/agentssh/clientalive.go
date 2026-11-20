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

// clientAliveTimeout is how long after the last reply OpenSSH closes the
// connection: CountMax unanswered probes, one per interval.
func clientAliveTimeout(interval time.Duration, countMax int) time.Duration {
	return interval * time.Duration(countMax+1)
}

// extractClientAlive strips both variables from env. It returns a zero
// interval unless both are positive integers whose timeout is shorter than
// limit, if limit is set, so a client can only tighten the keep-alive.
func extractClientAlive(env []string, limit time.Duration) (interval time.Duration, countMax int, filteredEnv []string) {
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
	// 16 bits keeps the timeout from overflowing.
	seconds, err := strconv.ParseUint(rawInterval, 10, 16)
	if err != nil || seconds == 0 {
		return 0, 0, filteredEnv
	}
	count, err := strconv.ParseUint(rawCountMax, 10, 16)
	if err != nil || count == 0 {
		return 0, 0, filteredEnv
	}
	interval = max(time.Duration(seconds)*time.Second, minClientAliveInterval)
	if limit > 0 && clientAliveTimeout(interval, int(count)) >= limit {
		return 0, 0, filteredEnv
	}
	return interval, int(count), filteredEnv
}
