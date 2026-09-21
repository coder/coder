package chatd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// memoryConsolidationInterval bounds how often a full project pays for a
	// consolidation call, including runs that failed or changed nothing.
	memoryConsolidationInterval        = time.Hour
	memoryConsolidationWorkTimeout     = 2 * time.Minute
	memoryConsolidationModelTimeout    = 60 * time.Second
	memoryConsolidationMaxOutputTokens = 2048
	memoryConsolidationMergeSeparator  = "\n\n"
)

// The model only chooses which memories go; it never writes memory text.
// Merged bodies are concatenated verbatim, so nothing the user said can be
// paraphrased away or invented.
const memoryConsolidationPrompt = "A project's memory index has reached its limit. Every memory is listed below by name and description. " +
	"Return names to delete only for memories that are clearly superseded, one-off, or exact duplicates. " +
	"Return merge groups when several memories cover the same topic: keep the name that best describes the topic and drop the rest; " +
	"their bodies are appended to the kept memory verbatim. " +
	"Prefer fewer changes. Return nothing when no change is clearly warranted."

type memoryConsolidation struct {
	Delete []string                   `json:"delete"`
	Merge  []memoryConsolidationMerge `json:"merge"`
}

type memoryConsolidationMerge struct {
	Keep string   `json:"keep"`
	Drop []string `json:"drop"`
}

func (p *Server) maybeConsolidateMemoriesAsync(ctx context.Context, logger slog.Logger, chat database.Chat) {
	if !p.experiments.Enabled(codersdk.ExperimentChatProjects) || chat.ParentChatID.Valid || !chat.ProjectID.Valid {
		return
	}
	consolidationCtx, cancel := p.inflightContext(ctx)
	if err := p.goInflight(func() {
		defer cancel()
		p.consolidateMemories(consolidationCtx, logger, chat)
	}); err != nil {
		cancel()
		logger.Debug(ctx, "skipped memory consolidation", slog.F("chat_id", chat.ID), slog.Error(err))
	}
}

// consolidateMemories frees room in a project's memory once it reaches the
// cap. Below the cap duplicates are harmless, so nothing runs.
func (p *Server) consolidateMemories(ctx context.Context, logger slog.Logger, chat database.Chat) {
	ctx, cancel := context.WithTimeout(ctx, memoryConsolidationWorkTimeout)
	defer cancel()
	//nolint:gocritic // Consolidation is detached internal chatd work.
	ctx = dbauthz.AsChatd(ctx)
	logger = logger.With(slog.F("chat_id", chat.ID), slog.F("project_id", chat.ProjectID.UUID))

	chat, err := p.db.GetChatByID(ctx, chat.ID)
	if err != nil {
		logger.Debug(ctx, "failed to re-read chat for memory consolidation", slog.Error(err))
		return
	}
	store, _, status := p.resolveMemoryScope(ctx, chat)
	if status != memoryScopeAvailable {
		return
	}
	count, err := store.Count(ctx)
	if err != nil {
		logger.Debug(ctx, "failed to count memories for consolidation", slog.Error(err))
		return
	}
	if count < chattool.MaxMemories {
		return
	}

	now := p.clock.Now()
	claimed, err := p.claimMemoryConsolidation(ctx, chat.ProjectID.UUID, now)
	if err != nil {
		logger.Warn(ctx, "failed to claim memory consolidation", slog.Error(err))
		return
	}
	if !claimed {
		return
	}

	entries, err := store.List(ctx)
	if err != nil {
		logger.Warn(ctx, "failed to list memories for consolidation", slog.Error(err))
		return
	}
	apiKeyID, err := p.ensureSyntheticAPIKeyID(ctx, chat.OwnerID)
	if err != nil {
		logger.Debug(ctx, "failed to ensure synthetic API key for memory consolidation", slog.Error(err))
		return
	}
	resolved, err := p.resolveModelCall(ctx, modelCallSpec{purpose: "memory_consolidation", chat: chat, buildOptions: modelBuildOptions{ActiveAPIKeyID: apiKeyID}})
	if err != nil {
		logger.Debug(ctx, "failed to resolve model for memory consolidation", slog.Error(err))
		return
	}
	call := resolved.newObjectCall("memory_consolidation", "Choose memories to delete or merge.", memoryConsolidationMaxOutputTokens)
	call.Prompt = quickgenPrompt(memoryConsolidationPrompt, chattool.FormatMemoryIndexForTool(entries))
	modelCtx, cancelModel := context.WithTimeout(ctx, memoryConsolidationModelTimeout)
	result, err := generateQuickgenObject[memoryConsolidation](modelCtx, resolved.model.LanguageModel(), call)
	cancelModel()
	if err != nil {
		logger.Warn(ctx, "memory consolidation model call failed", slog.Error(err))
		return
	}

	plan := planMemoryConsolidation(result.Object, entries)
	if len(plan.Delete) == 0 && len(plan.Merge) == 0 {
		logger.Info(ctx, "memory consolidation proposed no changes", slog.F("memories", count))
		return
	}
	var deleted, merged int
	if err := store.InTx(func(tx chattool.MemoryStore) error {
		if err := tx.Lock(ctx); err != nil {
			return xerrors.Errorf("lock memory scope: %w", err)
		}
		var err error
		deleted, merged, err = applyMemoryConsolidation(ctx, tx, plan)
		return err
	}); err != nil {
		logger.Warn(ctx, "failed to apply memory consolidation", slog.Error(err))
		return
	}
	logger.Info(ctx, "consolidated project memory",
		slog.F("memories_before", count),
		slog.F("deleted", deleted),
		slog.F("merged", merged),
		slog.F("plan", fmt.Sprintf("%+v", plan)),
	)
}

// claimMemoryConsolidation records the attempt before any model call so a
// failing run is rate-limited like a successful one. The advisory lock keeps
// two replicas from claiming the same project at once.
func (p *Server) claimMemoryConsolidation(ctx context.Context, projectID uuid.UUID, now time.Time) (bool, error) {
	claimed := false
	err := p.db.InTx(func(tx database.Store) error {
		locked, err := tx.TryAcquireLock(ctx, database.GenLockID("chat-memory-consolidation:"+projectID.String()))
		if err != nil {
			return xerrors.Errorf("try acquire lock: %w", err)
		}
		if !locked {
			return nil
		}
		project, err := tx.GetChatProjectByID(ctx, projectID)
		if err != nil {
			return xerrors.Errorf("get project: %w", err)
		}
		if project.MemoryConsolidatedAt.Valid && now.Sub(project.MemoryConsolidatedAt.Time) < memoryConsolidationInterval {
			return nil
		}
		if err := tx.UpdateChatProjectMemoryConsolidatedAt(ctx, database.UpdateChatProjectMemoryConsolidatedAtParams{ID: projectID, MemoryConsolidatedAt: now}); err != nil {
			return xerrors.Errorf("record consolidation: %w", err)
		}
		claimed = true
		return nil
	}, nil)
	return claimed, err
}

// planMemoryConsolidation keeps the proposals that name memories in the
// index, touching each memory at most once.
func planMemoryConsolidation(proposed memoryConsolidation, entries []chattool.MemoryIndexEntry) memoryConsolidation {
	known := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		known[entry.Name] = struct{}{}
	}
	claimed := make(map[string]struct{})
	claim := func(raw string) (string, bool) {
		name := strings.ToLower(strings.TrimSpace(raw))
		if _, exists := known[name]; !exists {
			return "", false
		}
		if _, taken := claimed[name]; taken {
			return "", false
		}
		claimed[name] = struct{}{}
		return name, true
	}

	var plan memoryConsolidation
	for _, merge := range proposed.Merge {
		keep, ok := claim(merge.Keep)
		if !ok {
			continue
		}
		var drop []string
		for _, raw := range merge.Drop {
			if name, ok := claim(raw); ok {
				drop = append(drop, name)
			}
		}
		if len(drop) == 0 {
			delete(claimed, keep)
			continue
		}
		plan.Merge = append(plan.Merge, memoryConsolidationMerge{Keep: keep, Drop: drop})
	}
	for _, raw := range proposed.Delete {
		if name, ok := claim(raw); ok {
			plan.Delete = append(plan.Delete, name)
		}
	}
	return plan
}

// applyMemoryConsolidation appends each dropped body to its kept memory and
// removes the dropped rows. A drop that would push the kept body over the
// size limit is left in place rather than truncated.
func applyMemoryConsolidation(ctx context.Context, store chattool.MemoryStore, plan memoryConsolidation) (deleted, merged int, err error) {
	for _, merge := range plan.Merge {
		keep, err := store.Get(ctx, merge.Keep)
		if errors.Is(err, chattool.ErrMemoryNotFound) {
			continue
		}
		if err != nil {
			return deleted, merged, err
		}
		body := keep.Body
		var drops []string
		for _, name := range merge.Drop {
			drop, err := store.Get(ctx, name)
			if errors.Is(err, chattool.ErrMemoryNotFound) {
				continue
			}
			if err != nil {
				return deleted, merged, err
			}
			combined := body + memoryConsolidationMergeSeparator + drop.Body
			if len(combined) > chattool.MaxMemoryBodyBytes {
				continue
			}
			body = combined
			drops = append(drops, name)
		}
		if len(drops) == 0 {
			continue
		}
		if _, err := store.Upsert(ctx, chattool.MemoryInput{Name: keep.Name, Description: keep.Description, Body: body}); err != nil {
			return deleted, merged, err
		}
		for _, name := range drops {
			if err := store.Delete(ctx, name); err != nil && !errors.Is(err, chattool.ErrMemoryNotFound) {
				return deleted, merged, err
			}
			merged++
		}
	}
	for _, name := range plan.Delete {
		if err := store.Delete(ctx, name); err != nil && !errors.Is(err, chattool.ErrMemoryNotFound) {
			return deleted, merged, err
		}
		deleted++
	}
	return deleted, merged, nil
}
