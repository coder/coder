package dbpurge

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	agplusage "github.com/coder/coder/v2/coderd/usage"
)

func TestAgentRuntimeBackfillProtectsChats(t *testing.T) {
	t.Parallel()

	cutoff := time.Date(2025, 3, 10, 10, 30, 0, 0, time.UTC)
	before := cutoff.Truncate(time.Hour)
	atOrAfter := before.Add(time.Hour)
	end := atOrAfter.Add(time.Hour)
	completedAt := end.Add(time.Minute)

	stateJSON := func(state agplusage.AgentRuntimeBackfillState) string {
		t.Helper()
		value, err := agplusage.MarshalAgentRuntimeBackfillState(state)
		require.NoError(t, err)
		return value
	}

	for _, tc := range []struct {
		name       string
		checkpoint database.GetAgentRuntimeBackfillCheckpointRow
		want       bool
		wantErr    bool
	}{
		{name: "missing"},
		{
			name: "pending",
			checkpoint: database.GetAgentRuntimeBackfillCheckpointRow{
				Present: true,
				Value:   agplusage.AgentRuntimeBackfillPendingJSON,
			},
			want: true,
		},
		{
			name: "malformed",
			checkpoint: database.GetAgentRuntimeBackfillCheckpointRow{
				Present: true,
				Value:   "not-json",
			},
			want:    true,
			wantErr: true,
		},
		{
			name: "cursor before cutoff",
			checkpoint: database.GetAgentRuntimeBackfillCheckpointRow{
				Present: true,
				Value: stateJSON(agplusage.AgentRuntimeBackfillState{
					Version:      agplusage.AgentRuntimeBackfillVersion,
					Status:       agplusage.AgentRuntimeBackfillStatusRunning,
					NextBucket:   &before,
					EndExclusive: &end,
				}),
			},
			want: true,
		},
		{
			name: "cursor at or after cutoff",
			checkpoint: database.GetAgentRuntimeBackfillCheckpointRow{
				Present: true,
				Value: stateJSON(agplusage.AgentRuntimeBackfillState{
					Version:      agplusage.AgentRuntimeBackfillVersion,
					Status:       agplusage.AgentRuntimeBackfillStatusRunning,
					NextBucket:   &atOrAfter,
					EndExclusive: &end,
				}),
			},
		},
		{
			name: "complete",
			checkpoint: database.GetAgentRuntimeBackfillCheckpointRow{
				Present: true,
				Value: stateJSON(agplusage.AgentRuntimeBackfillState{
					Version:      agplusage.AgentRuntimeBackfillVersion,
					Status:       agplusage.AgentRuntimeBackfillStatusComplete,
					NextBucket:   &end,
					EndExclusive: &end,
					CompletedAt:  &completedAt,
				}),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, _, err := agentRuntimeBackfillProtectsChats(tc.checkpoint, cutoff)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.want, got)
		})
	}
}
