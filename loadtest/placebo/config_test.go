package placebo_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/loadtest/placebo"
)

func Test_Config(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		config      placebo.Config
		errContains string
	}{
		{
			name: "Empty",
			config: placebo.Config{
				Sleep:  0,
				Jitter: 0,
			},
		},
		{
			name: "Sleep",
			config: placebo.Config{
				Sleep:  1 * time.Second,
				Jitter: 0,
			},
		},
		{
			name: "SleepAndJitter",
			config: placebo.Config{
				Sleep:  1 * time.Second,
				Jitter: 1 * time.Second,
			},
		},
		{
			name: "NegativeSleep",
			config: placebo.Config{
				Sleep:  -1 * time.Second,
				Jitter: 0,
			},
			errContains: "sleep must be set to a positive value",
		},
		{
			name: "NegativeJitter",
			config: placebo.Config{
				Sleep:  0,
				Jitter: -1 * time.Second,
			},
			errContains: "jitter must be set to a positive value",
		},
		{
			name: "JitterWithoutSleep",
			config: placebo.Config{
				Sleep:  0,
				Jitter: 1 * time.Second,
			},
			errContains: "jitter must be 0 if sleep is 0",
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			err := c.config.Validate()
			if c.errContains != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), c.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
