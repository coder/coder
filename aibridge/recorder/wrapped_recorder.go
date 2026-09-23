package recorder

import (
	"context"
	"time"

	"golang.org/x/xerrors"
)

var _ Recorder = &WrappedRecorder{}

// WrappedRecorder is a convenience struct which implements Recorder and resolves a client before calling each method.
// It also sets the start/creation time of each record.
type WrappedRecorder struct {
	clientFn func(context.Context) (Recorder, error)
}

func (r *WrappedRecorder) RecordInterception(ctx context.Context, req *InterceptionRecord) error {
	client, err := r.clientFn(ctx)
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.StartedAt = time.Now()
	return client.RecordInterception(ctx, req)
}

func (r *WrappedRecorder) RecordInterceptionEnded(ctx context.Context, req *InterceptionRecordEnded) error {
	client, err := r.clientFn(ctx)
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.EndedAt = time.Now().UTC()
	return client.RecordInterceptionEnded(ctx, req)
}

func (r *WrappedRecorder) RecordPromptUsage(ctx context.Context, req *PromptUsageRecord) error {
	client, err := r.clientFn(ctx)
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.CreatedAt = time.Now()
	return client.RecordPromptUsage(ctx, req)
}

func (r *WrappedRecorder) RecordTokenUsage(ctx context.Context, req *TokenUsageRecord) error {
	client, err := r.clientFn(ctx)
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.CreatedAt = time.Now()
	return client.RecordTokenUsage(ctx, req)
}

func (r *WrappedRecorder) RecordToolUsage(ctx context.Context, req *ToolUsageRecord) error {
	client, err := r.clientFn(ctx)
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.CreatedAt = time.Now()
	return client.RecordToolUsage(ctx, req)
}

func (r *WrappedRecorder) RecordModelThought(ctx context.Context, req *ModelThoughtRecord) error {
	client, err := r.clientFn(ctx)
	if err != nil {
		return xerrors.Errorf("acquire client: %w", err)
	}

	req.CreatedAt = time.Now()
	return client.RecordModelThought(ctx, req)
}

// NewWrappedRecorder creates a [WrappedRecorder]. clientFn receives the
// context of the call it serves.
func NewWrappedRecorder(clientFn func(context.Context) (Recorder, error)) *WrappedRecorder {
	return &WrappedRecorder{clientFn: clientFn}
}
