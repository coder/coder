package agentssh

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExtractClientAlive(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		env           []string
		agentInterval time.Duration
		wantInterval  time.Duration
		wantCountMax  int
	}{
		{"Unset", nil, 0, 0, 0},
		{"Valid", []string{ClientAliveIntervalEnvironmentVariable + "=10", ClientAliveCountMaxEnvironmentVariable + "=3"}, 0, 10 * time.Second, 3},
		{"RaisedToMinimum", []string{ClientAliveIntervalEnvironmentVariable + "=1", ClientAliveCountMaxEnvironmentVariable + "=3"}, 0, minClientAliveInterval, 3},
		{"MissingCountMax", []string{ClientAliveIntervalEnvironmentVariable + "=10"}, 0, 0, 0},
		{"Zero", []string{ClientAliveIntervalEnvironmentVariable + "=0", ClientAliveCountMaxEnvironmentVariable + "=3"}, 0, 0, 0},
		{"Invalid", []string{ClientAliveIntervalEnvironmentVariable + "=10s", ClientAliveCountMaxEnvironmentVariable + "=3"}, 0, 0, 0},
		{"OutOfRange", []string{ClientAliveIntervalEnvironmentVariable + "=99999999999", ClientAliveCountMaxEnvironmentVariable + "=3"}, 0, 0, 0},
		{"ShorterThanAgent", []string{ClientAliveIntervalEnvironmentVariable + "=10", ClientAliveCountMaxEnvironmentVariable + "=3"}, 11 * time.Second, 10 * time.Second, 3},
		{"NotShorterThanAgent", []string{ClientAliveIntervalEnvironmentVariable + "=10", ClientAliveCountMaxEnvironmentVariable + "=3"}, 10 * time.Second, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			interval, countMax, env := extractClientAlive(append([]string{"FOO=bar"}, tc.env...), tc.agentInterval)
			require.Equal(t, tc.wantInterval, interval)
			require.Equal(t, tc.wantCountMax, countMax)
			require.Equal(t, []string{"FOO=bar"}, env)
		})
	}
}
