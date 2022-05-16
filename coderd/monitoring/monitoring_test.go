package monitoring_test

import (
	"testing"

	"github.com/coder/coder/coderd/monitoring"
	"github.com/stretchr/testify/require"
)

func TestParseTelemetry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value     string
		telemetry monitoring.Telemetry
	}{
		{
			value:     "all",
			telemetry: monitoring.TelemetryAll,
		},
		{
			value:     "core",
			telemetry: monitoring.TelemetryCore,
		},
		{
			value:     "none",
			telemetry: monitoring.TelemetryNone,
		},
	}

	for _, tt := range tests {
		telemetry, err := monitoring.ParseTelemetry(tt.value)
		require.NoError(t, err)
		require.Equal(t, tt.telemetry, telemetry)
	}

	_, err := monitoring.ParseTelemetry("invalid")
	require.Error(t, err)
}
