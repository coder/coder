//go:build slim

package chatdebug

import (
	"context"

	"github.com/google/uuid"
)

// DefaultStaleThreshold matches the !slim default so callers that read the
// constant resolve the same value in either build.
const DefaultStaleThreshold = 0

// Service is a no-op stub used in slim builds. Workspace agents always pass
// a nil DebugSvc to chatloop, and chatloop guards every call site, so the
// methods here are unreachable in practice. They exist so chatloop compiles
// without conditional plumbing.
type Service struct{}

// CreateRunParams matches the !slim shape so chatloop and any other slim
// callers compile without a build-tag-aware import.
type CreateRunParams struct {
	ChatID              uuid.UUID
	RootChatID          uuid.UUID
	ParentChatID        uuid.UUID
	ModelConfigID       uuid.UUID
	TriggerMessageID    int64
	HistoryTipMessageID int64
	Kind                RunKind
	Status              Status
	Provider            string
	Model               string
	Summary             any
}

// FinalizeRunParams matches the !slim shape so chatloop compiles.
type FinalizeRunParams struct {
	RunID       uuid.UUID
	ChatID      uuid.UUID
	Status      Status
	SeedSummary map[string]any
}

// CreateCompactionRun is the slim no-op used by chatloop's compaction path.
// It is unreachable: chatloop never calls into a non-nil *Service in slim
// builds because the agent runtime constructs CompactionOptions without a
// DebugSvc.
func (*Service) CreateCompactionRun(context.Context, CreateRunParams) (uuid.UUID, error) {
	return uuid.Nil, nil
}

// FinalizeRun is a slim no-op for the same reason as CreateCompactionRun.
func (*Service) FinalizeRun(context.Context, FinalizeRunParams) error {
	return nil
}
