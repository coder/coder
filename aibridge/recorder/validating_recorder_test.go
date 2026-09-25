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
		// wantRejected is true when the record must not be delegated.
		wantRejected bool
	}{
		{name: "InterceptionValid", record: func(ctx context.Context, r recorder.Recorder) error {
			return r.RecordInterception(ctx, validInterception())
		}},
		{name: "InterceptionUnsetStartedAt", wantRejected: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validInterception()
			req.StartedAt = time.Time{}
			return r.RecordInterception(ctx, req)
		}},
		{name: "InterceptionIDNotUUID", wantRejected: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validInterception()
			req.ID = "not-a-uuid"
			return r.RecordInterception(ctx, req)
		}},
		{name: "InterceptionInitiatorNotUUID", wantRejected: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validInterception()
			req.InitiatorID = ""
			return r.RecordInterception(ctx, req)
		}},
		{name: "InterceptionEmptyProvider", wantRejected: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validInterception()
			req.Provider = ""
			return r.RecordInterception(ctx, req)
		}},
		{
			// The model comes from the client's request body, so the provider,
			// not the gateway, decides whether the request is acceptable.
			name: "InterceptionEmptyModelIsRecorded",
			record: func(ctx context.Context, r recorder.Recorder) error {
				req := validInterception()
				req.Model = ""
				return r.RecordInterception(ctx, req)
			},
		},

		{name: "EndedValid", record: func(ctx context.Context, r recorder.Recorder) error {
			return r.RecordInterceptionEnded(ctx, validEnded())
		}},
		{name: "EndedUnsetEndedAt", wantRejected: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validEnded()
			req.EndedAt = time.Time{}
			return r.RecordInterceptionEnded(ctx, req)
		}},
		{name: "EndedIDNotUUID", wantRejected: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validEnded()
			req.ID = ""
			return r.RecordInterceptionEnded(ctx, req)
		}},

		{name: "TokenUsageValid", record: func(ctx context.Context, r recorder.Recorder) error {
			return r.RecordTokenUsage(ctx, validToken())
		}},
		{name: "TokenUsageUnsetCreatedAt", wantRejected: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validToken()
			req.CreatedAt = time.Time{}
			return r.RecordTokenUsage(ctx, req)
		}},
		{name: "TokenUsageInterceptionIDNotUUID", wantRejected: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validToken()
			req.InterceptionID = "not-a-uuid"
			return r.RecordTokenUsage(ctx, req)
		}},
		{name: "TokenUsageNegativeCount", wantRejected: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validToken()
			req.Output = -1
			return r.RecordTokenUsage(ctx, req)
		}},
		{name: "TokenUsageImplausibleCount", wantRejected: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validToken()
			req.CacheReadInputTokens = recorder.MaxTokenUsage + 1
			return r.RecordTokenUsage(ctx, req)
		}},
		{
			// An empty message ID costs correlation, not correctness, so the
			// record is still delegated.
			name: "TokenUsageEmptyMsgIDIsRecorded",
			record: func(ctx context.Context, r recorder.Recorder) error {
				req := validToken()
				req.MsgID = ""
				return r.RecordTokenUsage(ctx, req)
			},
		},

		{name: "PromptUsageValid", record: func(ctx context.Context, r recorder.Recorder) error {
			return r.RecordPromptUsage(ctx, validPrompt())
		}},
		{name: "PromptUsageUnsetCreatedAt", wantRejected: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validPrompt()
			req.CreatedAt = time.Time{}
			return r.RecordPromptUsage(ctx, req)
		}},
		{
			// Empty prompts occur in production, so dropping them would lose
			// records that are recorded today.
			name: "PromptUsageEmptyPromptIsRecorded",
			record: func(ctx context.Context, r recorder.Recorder) error {
				req := validPrompt()
				req.Prompt = ""
				return r.RecordPromptUsage(ctx, req)
			},
		},

		{name: "ToolUsageValid", record: func(ctx context.Context, r recorder.Recorder) error {
			return r.RecordToolUsage(ctx, validTool())
		}},
		{name: "ToolUsageUnsetCreatedAt", wantRejected: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validTool()
			req.CreatedAt = time.Time{}
			return r.RecordToolUsage(ctx, req)
		}},
		{name: "ToolUsageEmptyTool", wantRejected: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validTool()
			req.Tool = ""
			return r.RecordToolUsage(ctx, req)
		}},
		{
			// Hosted Responses tools are executed by the provider and carry no
			// call ID, so it must stay optional.
			name: "ToolUsageWithoutToolCallIDIsRecorded",
			record: func(ctx context.Context, r recorder.Recorder) error {
				req := validTool()
				req.ToolCallID = ""
				req.ItemID = ""
				return r.RecordToolUsage(ctx, req)
			},
		},

		{name: "ModelThoughtValid", record: func(ctx context.Context, r recorder.Recorder) error {
			return r.RecordModelThought(ctx, validThought())
		}},
		{name: "ModelThoughtUnsetCreatedAt", wantRejected: true, record: func(ctx context.Context, r recorder.Recorder) error {
			req := validThought()
			req.CreatedAt = time.Time{}
			return r.RecordModelThought(ctx, req)
		}},
		{name: "ModelThoughtEmptyContentIsRecorded", record: func(ctx context.Context, r recorder.Recorder) error {
			req := validThought()
			req.Content = ""
			return r.RecordModelThought(ctx, req)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			term := &acceptingRecorder{}
			// Rejections are logged at error level, which slogtest fails on
			// unless ignored.
			logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
			rec := recorder.NewValidatingRecorder(logger, term)

			err := tc.record(t.Context(), rec)

			if tc.wantRejected {
				require.ErrorIs(t, err, recorder.ErrInvalidRecord)
				require.Zero(t, term.delegated, "an invalid record must not be delegated")
				return
			}
			require.NoError(t, err)
			require.Equal(t, 1, term.delegated, "a valid record must be delegated")
		})
	}
}
