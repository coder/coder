package usage_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/usage"
)

func TestAgentRuntimeBackfillState(t *testing.T) {
	t.Parallel()

	next := time.Date(2025, 1, 2, 3, 0, 0, 0, time.UTC)
	end := next.Add(time.Hour)
	completedAt := end.Add(time.Minute)

	for _, tc := range []struct {
		name    string
		value   string
		want    usage.AgentRuntimeBackfillState
		wantErr bool
	}{
		{
			name:  "pending",
			value: usage.AgentRuntimeBackfillPendingJSON,
			want: usage.AgentRuntimeBackfillState{
				Version: usage.AgentRuntimeBackfillVersion,
				Status:  usage.AgentRuntimeBackfillStatusPending,
			},
		},
		{
			name:  "running",
			value: `{"version":1,"status":"running","next_bucket":"2025-01-02T03:00:00Z","end_exclusive":"2025-01-02T04:00:00Z"}`,
			want: usage.AgentRuntimeBackfillState{
				Version:      usage.AgentRuntimeBackfillVersion,
				Status:       usage.AgentRuntimeBackfillStatusRunning,
				NextBucket:   &next,
				EndExclusive: &end,
			},
		},
		{
			name:  "complete",
			value: `{"version":1,"status":"complete","next_bucket":"2025-01-02T04:00:00Z","end_exclusive":"2025-01-02T04:00:00Z","completed_at":"2025-01-02T04:01:00Z"}`,
			want: usage.AgentRuntimeBackfillState{
				Version:      usage.AgentRuntimeBackfillVersion,
				Status:       usage.AgentRuntimeBackfillStatusComplete,
				NextBucket:   &end,
				EndExclusive: &end,
				CompletedAt:  &completedAt,
			},
		},
		{name: "unknown field", value: `{"version":1,"status":"pending","extra":true}`, wantErr: true},
		{name: "trailing value", value: usage.AgentRuntimeBackfillPendingJSON + `{}`, wantErr: true},
		{name: "wrong version", value: `{"version":2,"status":"pending"}`, wantErr: true},
		{name: "invalid status", value: `{"version":1,"status":"wat"}`, wantErr: true},
		{name: "running missing bounds", value: `{"version":1,"status":"running"}`, wantErr: true},
		{name: "non-hour bound", value: `{"version":1,"status":"running","next_bucket":"2025-01-02T03:01:00Z","end_exclusive":"2025-01-02T04:00:00Z"}`, wantErr: true},
		{name: "next after end", value: `{"version":1,"status":"running","next_bucket":"2025-01-02T05:00:00Z","end_exclusive":"2025-01-02T04:00:00Z"}`, wantErr: true},
		{name: "complete missing time", value: `{"version":1,"status":"complete","next_bucket":"2025-01-02T04:00:00Z","end_exclusive":"2025-01-02T04:00:00Z"}`, wantErr: true},
		{name: "complete before end", value: `{"version":1,"status":"complete","next_bucket":"2025-01-02T03:00:00Z","end_exclusive":"2025-01-02T04:00:00Z","completed_at":"2025-01-02T04:01:00Z"}`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := usage.ParseAgentRuntimeBackfillState(tc.value)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			marshaled, err := usage.MarshalAgentRuntimeBackfillState(got)
			require.NoError(t, err)
			require.JSONEq(t, tc.value, marshaled)
		})
	}
}
