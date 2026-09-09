package aibridged

import (
	"context"

	"github.com/coder/coder/v2/aibridge"
)

var _ aibridge.Recorder = &LogRecorder{}

type LogRecorder struct{}

func (t *LogRecorder) RecordInterception(ctx context.Context, req *aibridge.InterceptionRecord) error {
	return nil
}

func (t *LogRecorder) RecordInterceptionEnded(ctx context.Context, req *aibridge.InterceptionRecordEnded) error {
	return nil
}

func (t *LogRecorder) RecordPromptUsage(ctx context.Context, req *aibridge.PromptUsageRecord) error {
	return nil
}

func (t *LogRecorder) RecordTokenUsage(ctx context.Context, req *aibridge.TokenUsageRecord) error {
	return nil
}

func (t *LogRecorder) RecordToolUsage(ctx context.Context, req *aibridge.ToolUsageRecord) error {
	return nil
}

func (t *LogRecorder) RecordModelThought(ctx context.Context, req *aibridge.ModelThoughtRecord) error {
	return nil
}
