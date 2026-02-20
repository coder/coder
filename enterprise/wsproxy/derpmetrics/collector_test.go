package derpmetrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"tailscale.com/derp"
	"tailscale.com/types/key"

	"github.com/coder/coder/v2/enterprise/wsproxy/derpmetrics"
	"github.com/coder/coder/v2/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestCollector(t *testing.T) {
	t.Parallel()

	logger := testutil.Logger(t)
	_ = logger

	srv := derp.NewServer(key.NewNode(), func(format string, args ...any) {
		t.Logf(format, args...)
	})
	defer srv.Close()

	c := derpmetrics.NewCollector(srv)

	t.Run("ImplementsCollector", func(t *testing.T) {
		t.Parallel()
		var _ prometheus.Collector = c
	})

	t.Run("RegisterAndCollect", func(t *testing.T) {
		t.Parallel()

		reg := prometheus.NewRegistry()
		err := reg.Register(c)
		require.NoError(t, err)

		// Gather metrics and ensure no errors.
		families, err := reg.Gather()
		require.NoError(t, err)
		require.NotEmpty(t, families)

		// Check that at least some expected metric names are present.
		names := make(map[string]bool)
		for _, f := range families {
			names[f.GetName()] = true
		}

		// These gauges should always be present (even if zero).
		require.True(t, names["coder_wsproxy_derp_current_connections"],
			"expected current_connections metric, got: %v", names)
		require.True(t, names["coder_wsproxy_derp_current_home_connections"],
			"expected current_home_connections metric, got: %v", names)
	})
}
