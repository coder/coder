package testutil

import (
	"time"

	"github.com/coder/coder/v2/aibridge/recorder"
)

// WithoutCreatedAt returns copies of records with CreatedAt cleared, so
// records compare by content.
func WithoutCreatedAt[T any](records []*T) []T {
	out := make([]T, 0, len(records))
	for _, r := range records {
		v := *r
		switch rec := any(&v).(type) {
		case *recorder.PromptUsageRecord:
			rec.CreatedAt = time.Time{}
		case *recorder.TokenUsageRecord:
			rec.CreatedAt = time.Time{}
		case *recorder.ToolUsageRecord:
			rec.CreatedAt = time.Time{}
		case *recorder.ModelThoughtRecord:
			rec.CreatedAt = time.Time{}
		}
		out = append(out, v)
	}
	return out
}
