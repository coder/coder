package recorder_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	aibcontext "github.com/coder/coder/v2/aibridge/context"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/testutil"
)

// Record lines carry interception_id once. When the context already logs it,
// the context value wins over the record's.
func TestLogRecorderLogsInterceptionIDOnce(t *testing.T) {
	t.Parallel()
	const (
		recordID  = "record-interception"
		contextID = "context-interception"
	)
	for _, record := range []struct {
		name   string
		record func(context.Context, *recorder.LogRecorder) error
		// resultMessage is the message of the logResult line.
		resultMessage string
		// resultLogsID is set when the logResult log line has the
		// `interception_id` field. Start and end logResult lines
		// log it as `id`.
		resultLogsID bool
	}{
		{
			name: "InterceptionStart",
			record: func(ctx context.Context, rec *recorder.LogRecorder) error {
				return rec.RecordInterception(ctx, &recorder.InterceptionRecord{ID: recordID})
			},
			resultMessage: "recorded interception",
		},
		{
			name: "InterceptionEnd",
			record: func(ctx context.Context, rec *recorder.LogRecorder) error {
				return rec.RecordInterceptionEnded(ctx, &recorder.InterceptionRecordEnded{ID: recordID})
			},
			resultMessage: "recorded interception ended",
		},
		{
			name: "PromptUsage",
			record: func(ctx context.Context, rec *recorder.LogRecorder) error {
				return rec.RecordPromptUsage(ctx, &recorder.PromptUsageRecord{InterceptionID: recordID})
			},
			resultMessage: "recorded prompt usage",
			resultLogsID:  true,
		},
		{
			name: "TokenUsage",
			record: func(ctx context.Context, rec *recorder.LogRecorder) error {
				return rec.RecordTokenUsage(ctx, &recorder.TokenUsageRecord{InterceptionID: recordID})
			},
			resultMessage: "recorded token usage",
			resultLogsID:  true,
		},
		{
			name: "ToolUsage",
			record: func(ctx context.Context, rec *recorder.LogRecorder) error {
				return rec.RecordToolUsage(ctx, &recorder.ToolUsageRecord{InterceptionID: recordID})
			},
			resultMessage: "recorded tool usage",
			resultLogsID:  true,
		},
		{
			name: "ModelThought",
			record: func(ctx context.Context, rec *recorder.LogRecorder) error {
				return rec.RecordModelThought(ctx, &recorder.ModelThoughtRecord{InterceptionID: recordID})
			},
			resultMessage: "recorded model thought",
			resultLogsID:  true,
		},
	} {
		for _, tc := range []struct {
			name      string
			inContext bool
			wantID    string
		}{
			{name: "NoContextID", wantID: recordID},
			{name: "ContextIDWins", inContext: true, wantID: contextID},
		} {
			t.Run(record.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				sink := testutil.NewFakeSink(t)
				rec := recorder.NewLogRecorder(sink.Logger(), true, nil)
				// RecordInterception logs the actor's API key ID.
				ctx := aibcontext.AsActor(t.Context(), aibcontext.Actor{APIKeyID: "api-key"})
				if tc.inContext {
					ctx = aibcontext.WithInterceptionID(ctx, contextID)
				}
				require.NoError(t, record.record(ctx, rec))

				var sawRecordID bool
				var sawResultID bool
				for _, entry := range sink.Entries() {
					var ids []any
					for _, field := range entry.Fields {
						if field.Name == "interception_id" {
							ids = append(ids, field.Value)
						}
					}
					if len(ids) == 0 {
						continue
					}
					require.Equal(t, []any{tc.wantID}, ids, "wrong interception id %q", entry.Message)
					switch entry.Message {
					case recorder.InterceptionLogMarker:
						require.False(t, sawRecordID, "record line logged more than once")
						sawRecordID = true
					case record.resultMessage:
						require.False(t, sawResultID, "logResult line logged more than once")
						sawResultID = true
					default:
						t.Fatalf("unexpected line %q logs interception_id", entry.Message)
					}
				}
				require.True(t, sawRecordID, "record line must log interception_id")
				require.Equal(t, tc.inContext || record.resultLogsID, sawResultID, "logResult line logs interception_id")
			})
		}
	}
}
