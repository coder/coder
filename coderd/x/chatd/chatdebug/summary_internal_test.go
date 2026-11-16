package chatdebug

import (
	"encoding/json"
	"maps"
	"testing"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/testutil"
)

func TestSummarizeRunSteps(t *testing.T) {
	t.Parallel()

	raw := func(s string) pqtype.NullRawMessage {
		return pqtype.NullRawMessage{RawMessage: json.RawMessage(s), Valid: true}
	}
	withUsage := func(s string) database.ChatDebugStep {
		return database.ChatDebugStep{Status: string(StatusCompleted), Usage: raw(s)}
	}
	withAttempts := func(s string) database.ChatDebugStep {
		return database.ChatDebugStep{Status: string(StatusCompleted), Attempts: json.RawMessage(s)}
	}

	tests := []struct {
		name  string
		base  map[string]any
		steps []database.ChatDebugStep
		want  map[string]any
	}{
		{
			name: "NoStepsKeepsSeededValues",
			base: map[string]any{"first_message": "hello world"},
			want: map[string]any{
				"first_message":       "hello world",
				"step_count":          0,
				"total_input_tokens":  int64(0),
				"total_output_tokens": int64(0),
			},
		},
		{
			name:  "NilBaseOmitsZeroOptionalTotals",
			steps: []database.ChatDebugStep{withUsage(`{"input_tokens":10,"output_tokens":5,"reasoning_tokens":0,"cache_creation_tokens":0,"cache_read_tokens":0}`)},
			want: map[string]any{
				"step_count":          1,
				"total_input_tokens":  int64(10),
				"total_output_tokens": int64(5),
			},
		},
		{
			name: "ClearsStaleDerivedKeys",
			base: map[string]any{
				"first_message":               "hello world",
				"step_count":                  9,
				"total_input_tokens":          999,
				"total_output_tokens":         888,
				"total_reasoning_tokens":      777,
				"total_cache_creation_tokens": 100,
				"total_cache_read_tokens":     200,
				"has_error":                   true,
				"endpoint_label":              "POST /stale",
			},
			steps: []database.ChatDebugStep{withUsage(`{"input_tokens":20,"output_tokens":10}`)},
			want: map[string]any{
				"first_message":       "hello world",
				"step_count":          1,
				"total_input_tokens":  int64(20),
				"total_output_tokens": int64(10),
			},
		},
		{
			name: "SumsUsageAcrossSteps",
			steps: []database.ChatDebugStep{
				withUsage(`{"input_tokens":10,"output_tokens":5,"reasoning_tokens":20,"cache_creation_tokens":2,"cache_read_tokens":3}`),
				withUsage(`{"input_tokens":15,"output_tokens":7,"reasoning_tokens":30,"cache_creation_tokens":1,"cache_read_tokens":4}`),
			},
			want: map[string]any{
				"step_count":                  2,
				"total_input_tokens":          int64(25),
				"total_output_tokens":         int64(12),
				"total_reasoning_tokens":      int64(50),
				"total_cache_creation_tokens": int64(3),
				"total_cache_read_tokens":     int64(7),
			},
		},
		{
			name: "AbsentAndMalformedUsageAddNothing",
			steps: []database.ChatDebugStep{
				withUsage(`{"input_tokens":10,"output_tokens":5}`),
				{Status: string(StatusCompleted)},
				{Status: string(StatusCompleted), Usage: pqtype.NullRawMessage{Valid: true}},
				withUsage(`{"input_tokens":"not-a-number","output_tokens":7}`),
			},
			want: map[string]any{
				"step_count":          4,
				"total_input_tokens":  int64(10),
				"total_output_tokens": int64(5),
			},
		},
		{
			name:  "ErrorStatusSetsHasError",
			steps: []database.ChatDebugStep{{Status: string(StatusError)}},
			want:  map[string]any{"step_count": 1, "total_input_tokens": int64(0), "total_output_tokens": int64(0), "has_error": true},
		},
		{
			name:  "ErrorPayloadSetsHasError",
			steps: []database.ChatDebugStep{{Status: string(StatusCompleted), Error: raw(`{"message":"boom"}`)}},
			want:  map[string]any{"step_count": 1, "total_input_tokens": int64(0), "total_output_tokens": int64(0), "has_error": true},
		},
		{
			name: "InterruptedAndClearedErrorsIgnored",
			steps: []database.ChatDebugStep{
				{Status: string(StatusInterrupted), Error: raw(`{"message":"user canceled"}`)},
				{Status: string(StatusCompleted), Error: raw(`null`)},
				{Status: string(StatusCompleted), Error: pqtype.NullRawMessage{Valid: true}},
			},
			want: map[string]any{"step_count": 3, "total_input_tokens": int64(0), "total_output_tokens": int64(0)},
		},
		{
			name: "EndpointFromFirstCompletedAttemptWithPath",
			steps: []database.ChatDebugStep{
				{Status: string(StatusCompleted)},
				withAttempts(`{"status":"completed"`),
				withAttempts(`[{"status":"failed","method":"POST","path":"/failed"},{"status":"completed","method":"POST"},{"status":"completed","method":"POST","path":"/v1/messages"},{"status":"completed","method":"GET","path":"/later"}]`),
				withAttempts(`[{"status":"completed","method":"POST","path":"/next-step"}]`),
			},
			want: map[string]any{"step_count": 4, "total_input_tokens": int64(0), "total_output_tokens": int64(0), "endpoint_label": "POST /v1/messages"},
		},
		{
			name:  "EndpointPathOnlyWithoutMethod",
			steps: []database.ChatDebugStep{withAttempts(`[{"status":"completed","path":"/v1/messages"}]`)},
			want:  map[string]any{"step_count": 1, "total_input_tokens": int64(0), "total_output_tokens": int64(0), "endpoint_label": "/v1/messages"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			svc := &Service{log: testutil.Logger(t)}
			base := maps.Clone(tc.base)

			got := svc.summarizeRunSteps(ctx, uuid.New(), tc.steps, tc.base)
			require.Equal(t, tc.want, got)
			require.Equal(t, base, tc.base, "base summary must not be mutated")
		})
	}
}
