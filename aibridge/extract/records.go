package extract

import (
	"context"
	"errors"
	"maps"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/recorder"
)

// Records are the recorder records for one upstream response of an
// interception. Nil and empty fields mean there is nothing to record.
type Records struct {
	Prompt        *recorder.PromptUsageRecord
	TokenUsage    *recorder.TokenUsageRecord
	ToolUsages    []*recorder.ToolUsageRecord
	ModelThoughts []*recorder.ModelThoughtRecord
}

// NewRecords converts request and response facts into recorder records,
// with the same field semantics the interceptors use: MsgID is the provider
// response ID, a prompt is recorded only when a response ID is known, and
// tool calls are never marked injected (extractors only observe traffic).
func NewRecords(interceptionID string, req RequestFacts, resp ResponseFacts, now time.Time) Records {
	var out Records
	if req.Prompt != "" && resp.ResponseID != "" {
		out.Prompt = &recorder.PromptUsageRecord{
			InterceptionID: interceptionID,
			MsgID:          resp.ResponseID,
			Prompt:         req.Prompt,
			CreatedAt:      now,
		}
	}
	if u := resp.Usage; u != nil {
		var metadata recorder.Metadata
		if resp.ServiceTier != "" {
			metadata = recorder.Metadata{recorder.MetadataKeyServiceTier: resp.ServiceTier}
		}
		out.TokenUsage = &recorder.TokenUsageRecord{
			InterceptionID:        interceptionID,
			MsgID:                 resp.ResponseID,
			ProviderModel:         resp.Model,
			Input:                 u.Input,
			Output:                u.Output,
			CacheReadInputTokens:  u.CacheReadInput,
			CacheWriteInputTokens: u.CacheWriteInput,
			ExtraTokenTypes:       maps.Clone(u.Extra),
			Metadata:              metadata,
			CreatedAt:             now,
		}
	}
	for _, call := range resp.ToolCalls {
		out.ToolUsages = append(out.ToolUsages, &recorder.ToolUsageRecord{
			InterceptionID: interceptionID,
			MsgID:          resp.ResponseID,
			ItemID:         call.ItemID,
			ToolCallID:     call.CallID,
			Tool:           call.Name,
			Args:           call.Args,
			Injected:       false,
			CreatedAt:      now,
		})
	}
	for _, thought := range resp.Thoughts {
		out.ModelThoughts = append(out.ModelThoughts, &recorder.ModelThoughtRecord{
			InterceptionID: interceptionID,
			Content:        thought.Content,
			Metadata:       recorder.Metadata{"source": thought.Source},
			CreatedAt:      now,
		})
	}
	return out
}

// Record writes every record to rec. It attempts all of them and returns
// the joined errors.
func (r Records) Record(ctx context.Context, rec recorder.Recorder) error {
	var errs []error
	if r.Prompt != nil {
		if err := rec.RecordPromptUsage(ctx, r.Prompt); err != nil {
			errs = append(errs, xerrors.Errorf("record prompt usage: %w", err))
		}
	}
	if r.TokenUsage != nil {
		if err := rec.RecordTokenUsage(ctx, r.TokenUsage); err != nil {
			errs = append(errs, xerrors.Errorf("record token usage: %w", err))
		}
	}
	for _, t := range r.ToolUsages {
		if err := rec.RecordToolUsage(ctx, t); err != nil {
			errs = append(errs, xerrors.Errorf("record tool usage %q: %w", t.Tool, err))
		}
	}
	for _, t := range r.ModelThoughts {
		if err := rec.RecordModelThought(ctx, t); err != nil {
			errs = append(errs, xerrors.Errorf("record model thought: %w", err))
		}
	}
	return errors.Join(errs...)
}
