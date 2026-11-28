package recorder_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogjson"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// failingRecorder terminates a chain under test, rejecting every record.
type failingRecorder struct{}

var errRecordFailed = xerrors.New("record failed")

func (failingRecorder) RecordInterception(context.Context, *recorder.InterceptionRecord) error {
	return errRecordFailed
}

func (failingRecorder) RecordInterceptionEnded(context.Context, *recorder.InterceptionRecordEnded) error {
	return errRecordFailed
}

func (failingRecorder) RecordTokenUsage(context.Context, *recorder.TokenUsageRecord) error {
	return errRecordFailed
}

func (failingRecorder) RecordPromptUsage(context.Context, *recorder.PromptUsageRecord) error {
	return errRecordFailed
}

func (failingRecorder) RecordToolUsage(context.Context, *recorder.ToolUsageRecord) error {
	return errRecordFailed
}

func (failingRecorder) RecordModelThought(context.Context, *recorder.ModelThoughtRecord) error {
	return errRecordFailed
}

// TestLogRecorderOmitsContent asserts that prompt, tool argument and model
// thought content never reaches the result logs. Those lines are emitted
// regardless of structured logging (the success line at debug level, the
// failure lines always), so any content in them reaches every log pipeline.
func TestLogRecorderOmitsContent(t *testing.T) {
	t.Parallel()

	const (
		promptCanary  = "canary-prompt-7f3e2a91"
		argsCanary    = "canary-args-4b81c0d6"
		thoughtCanary = "canary-thought-9d27e5f3"
	)

	cases := []struct {
		name    string
		wrapped recorder.Recorder
		wantErr bool
		// wantMessages are the log messages that must still be emitted, since
		// deployments alert on them.
		wantMessages []string
	}{
		{
			name:    "Success",
			wrapped: &acceptingRecorder{},
			wantMessages: []string{
				"recorded prompt usage",
				"recorded tool usage",
				"recorded model thought",
			},
		},
		{
			name:    "Failure",
			wrapped: failingRecorder{},
			wantErr: true,
			wantMessages: []string{
				"failed to record prompt usage",
				"failed to record tool usage",
				"failed to record model thought",
				"failed to record usage",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			logger := slog.Make(slogjson.Sink(&buf)).Leveled(slog.LevelDebug)
			// Structured logging off: the default, and the configuration
			// deployments use to keep content out of their logs.
			rec := recorder.NewLogRecorder(logger, false, tc.wrapped)

			ctx := context.Background()
			interceptionID := uuid.NewString()
			now := time.Now().UTC()

			errs := []error{
				rec.RecordPromptUsage(ctx, &recorder.PromptUsageRecord{
					InterceptionID: interceptionID,
					MsgID:          "msg-1",
					Prompt:         promptCanary,
					CreatedAt:      now,
				}),
				rec.RecordToolUsage(ctx, &recorder.ToolUsageRecord{
					InterceptionID: interceptionID,
					MsgID:          "msg-1",
					Tool:           "bash",
					ToolCallID:     "call-1",
					Args:           map[string]any{"command": argsCanary},
					CreatedAt:      now,
				}),
				rec.RecordModelThought(ctx, &recorder.ModelThoughtRecord{
					InterceptionID: interceptionID,
					Content:        thoughtCanary,
					CreatedAt:      now,
				}),
			}
			for _, err := range errs {
				if tc.wantErr {
					require.ErrorIs(t, err, errRecordFailed)
				} else {
					require.NoError(t, err)
				}
			}

			out := buf.String()
			for _, canary := range []string{promptCanary, argsCanary, thoughtCanary} {
				require.NotContains(t, out, canary, "record content reached the log output")
			}
			for _, msg := range tc.wantMessages {
				require.Contains(t, out, msg)
			}
			// Sizes replace content, so a log reader can still tell an empty
			// prompt or call from a populated one.
			for _, field := range []string{"prompt_bytes", "args_bytes", "content_bytes"} {
				require.Contains(t, out, field)
			}
			// The failure line keeps the identifiers it carried in the payload.
			require.Contains(t, out, interceptionID)
		})
	}
}
