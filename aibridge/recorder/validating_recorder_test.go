package recorder_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// acceptingRecorder terminates a chain under test, counting what reached it.
type acceptingRecorder struct {
	delegated int
}

func (r *acceptingRecorder) RecordInterception(context.Context, *recorder.InterceptionRecord) error {
	r.delegated++
	return nil
}

func (r *acceptingRecorder) RecordInterceptionEnded(context.Context, *recorder.InterceptionRecordEnded) error {
	r.delegated++
	return nil
}

func (r *acceptingRecorder) RecordTokenUsage(context.Context, *recorder.TokenUsageRecord) error {
	r.delegated++
	return nil
}

func (r *acceptingRecorder) RecordPromptUsage(context.Context, *recorder.PromptUsageRecord) error {
	r.delegated++
	return nil
}

func (r *acceptingRecorder) RecordToolUsage(context.Context, *recorder.ToolUsageRecord) error {
	r.delegated++
	return nil
}

func (r *acceptingRecorder) RecordModelThought(context.Context, *recorder.ModelThoughtRecord) error {
	r.delegated++
	return nil
}

func TestValidatingRecorder(t *testing.T) {
	t.Parallel()

	var (
		interceptionID = uuid.NewString()
		initiatorID    = uuid.NewString()
		now            = time.Now().UTC()
	)

	// validInterception and the helpers below are the records every case
	// mutates, so a case states only the rule it exercises.
	validInterception := func() *recorder.InterceptionRecord {
		return &recorder.InterceptionRecord{
			ID:          interceptionID,
			InitiatorID: initiatorID,
			Provider:    "anthropic",
			Model:       "claude",
			StartedAt:   now,
		}
	}
	validEnded := func() *recorder.InterceptionRecordEnded {
		return &recorder.InterceptionRecordEnded{ID: interceptionID, EndedAt: now}
	}
	validToken := func() *recorder.TokenUsageRecord {
		return &recorder.TokenUsageRecord{
			InterceptionID: interceptionID,
			MsgID:          "msg-id",
			Input:          10,
			Output:         5,
			CreatedAt:      now,
		}
	}
	validPrompt := func() *recorder.PromptUsageRecord {
		return &recorder.PromptUsageRecord{
			InterceptionID: interceptionID,
			MsgID:          "msg-id",
			Prompt:         "why is the sky blue?",
			CreatedAt:      now,
		}
	}
	validTool := func() *recorder.ToolUsageRecord {
		return &recorder.ToolUsageRecord{
			InterceptionID: interceptionID,
			MsgID:          "msg-id",
			Tool:           "coder_whoami",
			CreatedAt:      now,
		}
	}
	validThought := func() *recorder.ModelThoughtRecord {
		return &recorder.ModelThoughtRecord{
			InterceptionID: interceptionID,
			Content:        "thinking",
			CreatedAt:      now,
		}
	}

	for _, tc := range []struct {
		name string
		// record makes the call under test.
		record func(context.Context, recorder.Recorder) error
		// wantRefused is true when the record must not be delegated.
		wantRefused bool
	}{
		{name: "InterceptionValid", record: func(ctx context.Context, r recorder.Recorder) error {
			return r.RecordInterception(ctx, validInterception())
		}},
		{name: "InterceptionUnsetStartedAt", wantRefused: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validInterception()
			req.StartedAt = time.Time{}
			return r.RecordInterception(ctx, req)
		}},

		{name: "EndedValid", record: func(ctx context.Context, r recorder.Recorder) error {
			return r.RecordInterceptionEnded(ctx, validEnded())
		}},
		{name: "EndedUnsetEndedAt", wantRefused: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validEnded()
			req.EndedAt = time.Time{}
			return r.RecordInterceptionEnded(ctx, req)
		}},

		{name: "TokenUsageValid", record: func(ctx context.Context, r recorder.Recorder) error {
			return r.RecordTokenUsage(ctx, validToken())
		}},
		{name: "TokenUsageUnsetCreatedAt", wantRefused: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validToken()
			req.CreatedAt = time.Time{}
			return r.RecordTokenUsage(ctx, req)
		}},

		{name: "PromptUsageValid", record: func(ctx context.Context, r recorder.Recorder) error {
			return r.RecordPromptUsage(ctx, validPrompt())
		}},
		{name: "PromptUsageUnsetCreatedAt", wantRefused: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validPrompt()
			req.CreatedAt = time.Time{}
			return r.RecordPromptUsage(ctx, req)
		}},

		{name: "ToolUsageValid", record: func(ctx context.Context, r recorder.Recorder) error {
			return r.RecordToolUsage(ctx, validTool())
		}},
		{name: "ToolUsageUnsetCreatedAt", wantRefused: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validTool()
			req.CreatedAt = time.Time{}
			return r.RecordToolUsage(ctx, req)
		}},

		{name: "ModelThoughtValid", record: func(ctx context.Context, r recorder.Recorder) error {
			return r.RecordModelThought(ctx, validThought())
		}},
		{name: "ModelThoughtUnsetCreatedAt", wantRefused: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validThought()
			req.CreatedAt = time.Time{}
			return r.RecordModelThought(ctx, req)
		}},

		{
			// Everything other than the timestamp is the database's to
			// enforce, so that this recorder cannot diverge from it.
			name: "RecordWithoutTimestampRuleIsDelegated",
			record: func(ctx context.Context, r recorder.Recorder) error {
				req := validInterception()
				req.ID = "not-a-uuid"
				req.InitiatorID = ""
				req.Provider = ""
				req.Model = ""
				return r.RecordInterception(ctx, req)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			term := &acceptingRecorder{}
			// Refusals are logged at error level, which slogtest fails on
			// unless ignored.
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			rec := recorder.NewValidatingRecorder(logger, term)

			err := tc.record(t.Context(), rec)

			if tc.wantRefused {
				require.ErrorIs(t, err, recorder.ErrInvalidRecord)
				require.Zero(t, term.delegated, "a record with an unset timestamp must not be delegated")
				return
			}
			require.NoError(t, err)
			require.Equal(t, 1, term.delegated, "a valid record must be delegated")
		})
	}
}
