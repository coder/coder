package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	base := Config{
		Messages: 100, PayloadSize: 1024, Subjects: 1, Publishers: 1, Subscribers: 1, Replicas: 1,
		Timeout: testutil.WaitShort,
	}
	require.NoError(t, base.validate())

	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"NoPayload", func(c *Config) { c.PayloadSize = 0 }},
		{"OversizedPayload", func(c *Config) { c.PayloadSize = 2 << 20 }},
		{"NoSubjects", func(c *Config) { c.Subjects = 0 }},
		{"NoPublishers", func(c *Config) { c.Publishers = 0 }},
		{"NoSubscribers", func(c *Config) { c.Subscribers = 0 }},
		{"NoReplicas", func(c *Config) { c.Replicas = 0 }},
		{"NegativeMessages", func(c *Config) { c.Messages = -1 }},
		{"NoTimeout", func(c *Config) { c.Timeout = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := base
			tc.mutate(&cfg)
			require.Error(t, cfg.validate())
		})
	}
}

func TestRunSingleNode(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := testutil.Logger(t)

	cfg := Config{
		Messages:    1000,
		PayloadSize: 1024,
		Subjects:    2,
		Publishers:  4,
		Subscribers: 8,
		Replicas:    1,
		InProcess:   true,
		Timeout:     testutil.WaitLong,
	}
	res, err := Run(ctx, logger, cfg)
	require.NoError(t, err)

	pl := buildPlan(cfg)
	require.EqualValues(t, cfg.Messages, res.Published)
	require.EqualValues(t, pl.totalExpected, res.Expected)
	require.EqualValues(t, pl.totalExpected, res.Delivered)
	require.Greater(t, res.Delivered, res.Published, "fan-out must exceed publishes")
	require.Zero(t, res.Drops)
	// The exact count assertions above are the authoritative correctness
	// checks; a run that published or delivered nothing fails there. The
	// rates below are derived (count / duration), and Windows' monotonic
	// clock advances in coarse (up to 15.6ms) ticks, so a sub-tick phase
	// measures a 0 duration and must yield a 0 rate rather than a
	// fabricated one. On fine-grained clocks the strict branch applies.
	if res.PublishDuration > 0 {
		require.Greater(t, res.PubsPerSec, 0.0)
	} else {
		require.Zero(t, res.PubsPerSec)
	}
	if res.DeliverDuration > 0 {
		require.Greater(t, res.DeliveriesPerSec, 0.0)
	} else {
		require.Zero(t, res.DeliveriesPerSec)
	}
	require.GreaterOrEqual(t, res.DeliverDuration, res.PublishDuration)
}

func TestRunCluster(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}

	ctx := testutil.Context(t, testutil.WaitSuperLong)
	logger := testutil.Logger(t)

	// Random node placement across 3 replicas makes cross-node
	// delivery the common case, exercising route propagation through
	// the readiness gate.
	cfg := Config{
		Messages:    600,
		PayloadSize: 512,
		Subjects:    2,
		Publishers:  4,
		Subscribers: 6,
		Replicas:    3,
		Timeout:     testutil.WaitLong,
	}
	res, err := Run(ctx, logger, cfg)
	require.NoError(t, err)

	pl := buildPlan(cfg)
	require.EqualValues(t, cfg.Messages, res.Published)
	require.EqualValues(t, pl.totalExpected, res.Expected)
	require.EqualValues(t, pl.totalExpected, res.Delivered)
	require.Zero(t, res.Drops)
	// See TestRunSingleNode: counts above are the correctness checks;
	// coarse clocks can quantize a phase duration, and thus its rate, to 0.
	if res.PublishDuration > 0 {
		require.Greater(t, res.PubsPerSec, 0.0)
	} else {
		require.Zero(t, res.PubsPerSec)
	}
	if res.DeliverDuration > 0 {
		require.Greater(t, res.DeliveriesPerSec, 0.0)
	} else {
		require.Zero(t, res.DeliveriesPerSec)
	}
}
