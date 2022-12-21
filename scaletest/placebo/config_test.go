package placebo_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/scaletest/placebo"
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
				Sleep:         0,
				Jitter:        0,
				FailureChance: 0,
			},
		},
		{
			name: "Sleep",
			config: placebo.Config{
				Sleep:         httpapi.Duration(1 * time.Second),
				Jitter:        0,
				FailureChance: 0,
			},
		},
		{
			name: "SleepAndJitter",
			config: placebo.Config{
				Sleep:         httpapi.Duration(1 * time.Second),
				Jitter:        httpapi.Duration(1 * time.Second),
				FailureChance: 0,
			},
		},
		{
			name: "FailureChance",
			config: placebo.Config{
				Sleep:         0,
				Jitter:        0,
				FailureChance: 0.5,
			},
		},
		{
			name: "NegativeSleep",
			config: placebo.Config{
				Sleep:         httpapi.Duration(-1 * time.Second),
				Jitter:        0,
				FailureChance: 0,
			},
			errContains: "sleep must be set to a positive value",
		},
		{
			name: "NegativeJitter",
			config: placebo.Config{
				Sleep:         0,
				Jitter:        httpapi.Duration(-1 * time.Second),
				FailureChance: 0,
			},
			errContains: "jitter must be set to a positive value",
		},
		{
			name: "JitterWithoutSleep",
			config: placebo.Config{
				Sleep:         0,
				Jitter:        httpapi.Duration(1 * time.Second),
				FailureChance: 0,
			},
			errContains: "jitter must be 0 if sleep is 0",
		},
		{
			name: "NegativeFailureChance",
			config: placebo.Config{
				Sleep:         0,
				Jitter:        0,
				FailureChance: -0.1,
			},
			errContains: "failure_chance must be between 0 and 1",
		},
		{
			name: "FailureChanceTooLarge",
			config: placebo.Config{
				Sleep:         0,
				Jitter:        0,
				FailureChance: 1.1,
			},
			errContains: "failure_chance must be between 0 and 1",
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
