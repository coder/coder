package derpmetrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	ptestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/derp"
	"tailscale.com/types/key"

	"github.com/coder/coder/v2/tailnet/derpmetrics"
)

func TestDERPExpvarCollector(t *testing.T) {
	t.Parallel()

	t.Run("RegistersAndCollects", func(t *testing.T) {
		t.Parallel()

		server := derp.NewServer(key.NewNode(), func(format string, args ...any) {})
		defer server.Close()

		reg := prometheus.NewRegistry()
		collector := derpmetrics.NewDERPExpvarCollector(server)
		require.NoError(t, reg.Register(collector))

		// Verify we can gather without error.
		metrics, err := reg.Gather()
		require.NoError(t, err)
		require.NotEmpty(t, metrics, "expected at least one metric family")

		// Verify expected metric names are present.
		names := make(map[string]struct{})
		for _, m := range metrics {
			names[m.GetName()] = struct{}{}
		}

		expectedCounters := []string{
			"coder_derp_server_accepts_total",
			"coder_derp_server_bytes_received_total",
			"coder_derp_server_bytes_sent_total",
			"coder_derp_server_packets_received_total",
			"coder_derp_server_packets_sent_total",
			"coder_derp_server_packets_dropped_total",
			"coder_derp_server_packets_forwarded_in_total",
			"coder_derp_server_packets_forwarded_out_total",
			"coder_derp_server_home_moves_in_total",
			"coder_derp_server_home_moves_out_total",
			"coder_derp_server_got_ping_total",
			"coder_derp_server_sent_pong_total",
			"coder_derp_server_peer_gone_disconnected_total",
			"coder_derp_server_peer_gone_not_here_total",
			"coder_derp_server_unknown_frames_total",
		}
		expectedGauges := []string{
			"coder_derp_server_connections",
			"coder_derp_server_home_connections",
			"coder_derp_server_clients",
			"coder_derp_server_clients_local",
			"coder_derp_server_clients_remote",
			"coder_derp_server_watchers",
			"coder_derp_server_average_queue_duration_ms",
		}
		expectedLabeled := []string{
			"coder_derp_server_packets_dropped_reason_total",
			"coder_derp_server_packets_dropped_type_total",
			"coder_derp_server_packets_received_kind_total",
		}

		for _, name := range expectedCounters {
			assert.Contains(t, names, name, "missing counter %s", name)
		}
		for _, name := range expectedGauges {
			assert.Contains(t, names, name, "missing gauge %s", name)
		}
		for _, name := range expectedLabeled {
			assert.Contains(t, names, name, "missing labeled counter %s", name)
		}
	})

	t.Run("CounterTypes", func(t *testing.T) {
		t.Parallel()

		server := derp.NewServer(key.NewNode(), func(format string, args ...any) {})
		defer server.Close()

		reg := prometheus.NewRegistry()
		collector := derpmetrics.NewDERPExpvarCollector(server)
		require.NoError(t, reg.Register(collector))

		// Counters should report as counter type.
		count := ptestutil.CollectAndCount(collector)
		assert.Greater(t, count, 0, "expected metrics to be collected")

		// Verify a known counter starts at zero.
		metrics, err := reg.Gather()
		require.NoError(t, err)
		for _, m := range metrics {
			if m.GetName() == "coder_derp_server_bytes_received_total" {
				require.Len(t, m.GetMetric(), 1)
				assert.Equal(t, float64(0), m.GetMetric()[0].GetCounter().GetValue())
				return
			}
		}
		t.Fatal("coder_derp_server_bytes_received_total not found")
	})

	t.Run("GaugeTypes", func(t *testing.T) {
		t.Parallel()

		server := derp.NewServer(key.NewNode(), func(format string, args ...any) {})
		defer server.Close()

		reg := prometheus.NewRegistry()
		collector := derpmetrics.NewDERPExpvarCollector(server)
		require.NoError(t, reg.Register(collector))

		metrics, err := reg.Gather()
		require.NoError(t, err)
		for _, m := range metrics {
			if m.GetName() == "coder_derp_server_connections" {
				require.Len(t, m.GetMetric(), 1)
				// Gauge type check — GetGauge should be non-nil.
				assert.NotNil(t, m.GetMetric()[0].GetGauge())
				assert.Equal(t, float64(0), m.GetMetric()[0].GetGauge().GetValue())
				return
			}
		}
		t.Fatal("coder_derp_server_connections not found")
	})

	t.Run("LabeledCounters", func(t *testing.T) {
		t.Parallel()

		server := derp.NewServer(key.NewNode(), func(format string, args ...any) {})
		defer server.Close()

		reg := prometheus.NewRegistry()
		collector := derpmetrics.NewDERPExpvarCollector(server)
		require.NoError(t, reg.Register(collector))

		metrics, err := reg.Gather()
		require.NoError(t, err)

		for _, m := range metrics {
			if m.GetName() == "coder_derp_server_packets_dropped_reason_total" {
				// Should have labeled sub-metrics (one per reason).
				require.NotEmpty(t, m.GetMetric(), "expected labeled metrics for drop reasons")
				// Each metric should have a "reason" label.
				for _, metric := range m.GetMetric() {
					labels := metric.GetLabel()
					require.Len(t, labels, 1)
					assert.Equal(t, "reason", labels[0].GetName())
				}
				return
			}
		}
		t.Fatal("coder_derp_server_packets_dropped_reason_total not found")
	})

	t.Run("NoDuplicateRegistration", func(t *testing.T) {
		t.Parallel()

		server := derp.NewServer(key.NewNode(), func(format string, args ...any) {})
		defer server.Close()

		reg := prometheus.NewRegistry()
		c1 := derpmetrics.NewDERPExpvarCollector(server)
		require.NoError(t, reg.Register(c1))

		c2 := derpmetrics.NewDERPExpvarCollector(server)
		err := reg.Register(c2)
		assert.Error(t, err, "registering a second collector should fail")
	})
}
