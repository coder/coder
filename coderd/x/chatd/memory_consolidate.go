package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
	memoryConsolidationMinMemories   = 20
	memoryConsolidationDebounce      = 24 * time.Hour
	memoryConsolidationRunningStale  = 10 * time.Minute
	memoryConsolidationFailureRetry  = 15 * time.Minute
	memoryConsolidationMaxMutations  = 8
	memoryConsolidationModelTimeout  = 90 * time.Second
	memoryConsolidationWorkTimeout   = 3 * time.Minute
	memoryConsolidationInputBytes    = 96 * 1024
	memoryConsolidationProtectWindow = 24 * time.Hour
	memoryConsolidationKeepRecords   = 20
	// Every merge or update returns a complete body of up to
	// chattool.MaxMemoryBodyBytes, so the budget must hold several full
	// bodies plus the JSON envelope. It matches the advisor's ceiling,
	// which every supported provider accepts.
	memoryConsolidationMaxOutputTokens = 16384
)

const memoryConsolidationPrompt = "You consolidate durable chat memories. Review the memories below and return only high-confidence changes. " +
	"Merge near-duplicates into the existing name with the most history. Delete only facts that are clearly superseded or one-off. " +
	"Never invent facts. Prefer fewer edits. Leave any memory updated in the last 24 hours alone. " +
	"A merge must combine at least two existing memories; its target may be new only when it combines at least two memories. " +
	"For each merge or update, provide a concise description and complete body. Return no mutations when no change is clearly warranted."

type memoryConsolidation struct {
	Mutations []memoryConsolidationMutation `json:"mutations"`
}

type memoryConsolidationMutation struct {
	Op          string   `json:"op"`
	Name        string   `json:"name"`
	Into        string   `json:"into"`
	From        []string `json:"from"`
	Description string   `json:"description"`
	Body        string   `json:"body"`
	Reason      string   `json:"reason"`
}

type memoryConsolidationScope struct {
	organizationID uuid.UUID
	projectID      uuid.UUID
}

// memoryConsolidationScopeForChat identifies the project a chat's memory
// belongs to. Callers only reach this after resolveMemoryScope confirmed the
// chat is in a project.
func memoryConsolidationScopeForChat(chat database.Chat) memoryConsolidationScope {
	return memoryConsolidationScope{organizationID: chat.OrganizationID, projectID: chat.ProjectID.UUID}
}

func (s memoryConsolidationScope) lockID() int64 {
	return database.GenLockID("chat-memory-consolidation:" + s.projectID.String())
}

func (p *Server) maybeConsolidateMemoriesAsync(ctx context.Context, logger slog.Logger, chat database.Chat) {
	if chat.ParentChatID.Valid {
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

func (p *Server) consolidateMemories(ctx context.Context, logger slog.Logger, chat database.Chat) {
	ctx, cancel := context.WithTimeout(ctx, memoryConsolidationWorkTimeout)
	defer cancel()
	//nolint:gocritic // Consolidation is detached internal chatd work.
	ctx = dbauthz.AsChatd(ctx)

	chat, err := p.db.GetChatByID(ctx, chat.ID)
	if err != nil {
		logger.Debug(ctx, "failed to re-read chat for memory consolidation", slog.Error(err))
		return
	}
	store, _, status := p.resolveMemoryScope(ctx, chat)
	if status != memoryScopeAvailable {
		return
	}
	before, err := store.Count(ctx)
	if err != nil {
		logger.Debug(ctx, "failed to count memories for consolidation", slog.F("chat_id", chat.ID), slog.Error(err))
		return
	}
	if before < memoryConsolidationMinMemories {
		return
	}

	scope := memoryConsolidationScopeForChat(chat)
	now := time.Now()
	debounced, previousNextWindowStart := memoryConsolidationDebounced(ctx, p.db, scope, before, now, logger)
	if debounced {
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

	var record database.ChatMemoryConsolidation
	started := false
	err = p.db.InTx(func(tx database.Store) error {
		locked, err := tx.TryAcquireLock(ctx, scope.lockID())
		if err != nil {
			return xerrors.Errorf("try acquire consolidation lock: %w", err)
		}
		if !locked {
			return nil
		}
		if debounced, _ := memoryConsolidationDebounced(ctx, tx, scope, before, now, logger); debounced {
			return nil
		}
		record, err = tx.InsertChatMemoryConsolidation(ctx, database.InsertChatMemoryConsolidationParams{
			OrganizationID: scope.organizationID,
			ProjectID:      scope.projectID,
			Model:          resolved.resolvedModel,
			MemoriesBefore: memoryConsolidationCount(before),
		})
		if err != nil {
			return xerrors.Errorf("insert running consolidation: %w", err)
		}
		started = true
		return nil
	}, nil)
	if err != nil {
		logger.Warn(ctx, "failed to start memory consolidation", slog.F("chat_id", chat.ID), slog.Error(err))
		return
	}
	if !started {
		return
	}

	// A run that fails before the model answers keeps the previous cursor so
	// the same window is retried rather than skipped.
	resumeAt := previousNextWindowStart
	finish := func(status database.ChatMemoryConsolidationStatus, after int64, mutations []codersdk.ChatMemoryMutation, finishErr error) {
		if err := p.finishMemoryConsolidation(ctx, scope, record.ID, status, after, mutations, resumeAt, finishErr); err != nil {
			logger.Warn(ctx, "failed to finish memory consolidation", slog.F("chat_id", chat.ID), slog.Error(err))
		}
	}

	memories, err := store.ListFull(ctx)
	if err != nil {
		finish(database.ChatMemoryConsolidationStatusFailed, before, nil, xerrors.Errorf("load memories: %w", err))
		return
	}
	candidates := memoryConsolidationCandidates(memories, time.Now())
	if len(candidates) == 0 {
		resumeAt = 0
		finish(database.ChatMemoryConsolidationStatusSkipped, before, nil, nil)
		return
	}
	// A run that could not fit every candidate records where the next one
	// should resume, so a stuck prefix cannot starve the rest of the set.
	window, nextWindowStart := memoryConsolidationWindow(candidates, previousNextWindowStart)
	call := resolved.newObjectCall("memory_consolidation", "Consolidate durable chat memories.", memoryConsolidationMaxOutputTokens)
	call.Prompt = quickgenPrompt(memoryConsolidationPrompt, formatMemoryConsolidationInput(window))
	modelCtx, cancelModel := context.WithTimeout(ctx, memoryConsolidationModelTimeout)
	result, err := generateQuickgenObject[memoryConsolidation](modelCtx, resolved.model.LanguageModel(), call)
	cancelModel()
	if err != nil {
		finish(database.ChatMemoryConsolidationStatusFailed, before, nil, xerrors.Errorf("generate consolidation: %w", err))
		return
	}
	resumeAt = nextWindowStart

	// Only memories the model actually saw may be touched; the full
	// snapshot rejects targets it did not see and later detects
	// concurrent edits.
	mutations := validateMemoryConsolidationMutations(result.Object.Mutations, window, memories, time.Now())
	if len(mutations) == 0 {
		finish(database.ChatMemoryConsolidationStatusSkipped, before, nil, nil)
		return
	}
	var applied []memoryConsolidationMutation
	if err := store.InTx(func(txStore chattool.MemoryStore) error {
		var err error
		applied, err = applyMemoryConsolidationMutations(ctx, txStore, mutations, memories, time.Now())
		return err
	}); err != nil {
		finish(database.ChatMemoryConsolidationStatusFailed, before, nil, xerrors.Errorf("apply mutations: %w", err))
		return
	}
	// Concurrent edits can invalidate every mutation at apply time. That is
	// a skipped run, not a successful one, so a partial window at the cap
	// still continues to the rest of the set.
	if len(applied) == 0 {
		finish(database.ChatMemoryConsolidationStatusSkipped, before, nil, nil)
		return
	}
	after, err := store.Count(ctx)
	if err != nil {
		finish(database.ChatMemoryConsolidationStatusFailed, before, nil, xerrors.Errorf("count consolidated memories: %w", err))
		return
	}
	finish(database.ChatMemoryConsolidationStatusSucceeded, after, memoryConsolidationJournalMutations(applied), nil)
}

// memoryConsolidationDebounced reports whether the project was consolidated
// recently, and where the next run should start reading candidates. It takes
// the store explicitly so the transactional check runs against the
// transaction that also inserts the running record.
func memoryConsolidationDebounced(ctx context.Context, db database.Store, scope memoryConsolidationScope, count int64, now time.Time, logger slog.Logger) (debounced bool, nextWindowStart int32) {
	record, err := db.GetLatestChatMemoryConsolidationByProject(ctx, scope.projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0
	}
	if err != nil {
		logger.Debug(ctx, "failed to load latest memory consolidation", slog.Error(err))
		return true, 0
	}
	if record.Status == database.ChatMemoryConsolidationStatusRunning {
		return now.Sub(record.StartedAt) < memoryConsolidationRunningStale, record.NextWindowStart
	}
	if count >= chattool.MaxMemories {
		// At the cap a run that shrank the project may continue immediately
		// so new saves are unblocked, and a skipped run that only saw part
		// of the set moves on to the rest. A full pass that changed nothing
		// still debounces, or a full project would pay for the same model
		// call on every turn.
		if record.Status == database.ChatMemoryConsolidationStatusSucceeded && record.MemoriesAfter < record.MemoriesBefore {
			return false, record.NextWindowStart
		}
		if record.Status == database.ChatMemoryConsolidationStatusSkipped && record.NextWindowStart > 0 {
			return false, record.NextWindowStart
		}
		// A transient model or apply failure must not leave a full project
		// unable to save for the whole debounce.
		if record.Status == database.ChatMemoryConsolidationStatusFailed {
			return now.Sub(record.StartedAt) < memoryConsolidationFailureRetry, record.NextWindowStart
		}
	}
	return now.Sub(record.StartedAt) < memoryConsolidationDebounce, record.NextWindowStart
}

func (p *Server) finishMemoryConsolidation(ctx context.Context, scope memoryConsolidationScope, id uuid.UUID, status database.ChatMemoryConsolidationStatus, after int64, mutations []codersdk.ChatMemoryMutation, nextWindowStart int32, finishErr error) error {
	if mutations == nil {
		mutations = []codersdk.ChatMemoryMutation{}
	}
	encoded, err := json.Marshal(mutations)
	if err != nil {
		return xerrors.Errorf("marshal mutations: %w", err)
	}
	errText := ""
	if finishErr != nil {
		errText = finishErr.Error()
	}
	if _, err := p.db.FinishChatMemoryConsolidation(ctx, database.FinishChatMemoryConsolidationParams{ID: id, Status: status, MemoriesAfter: memoryConsolidationCount(after), Mutations: encoded, Error: errText, NextWindowStart: nextWindowStart}); err != nil {
		return err
	}
	return p.db.PruneChatMemoryConsolidationsByProject(ctx, database.PruneChatMemoryConsolidationsByProjectParams{ProjectID: scope.projectID, KeepCount: memoryConsolidationKeepRecords})
}

// memoryConsolidationCandidates returns the memories the model may change,
// oldest first. Memories inside the protect window are excluded because no
// mutation touching them would be applied, and presenting the oldest first
// means a window that cannot fit everything shows stale duplicates before
// recent memories.
func memoryConsolidationCandidates(memories []chattool.Memory, now time.Time) []chattool.Memory {
	candidates := make([]chattool.Memory, 0, len(memories))
	for _, memory := range memories {
		if now.Sub(memory.UpdatedAt) < memoryConsolidationProtectWindow {
			continue
		}
		candidates = append(candidates, memory)
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].UpdatedAt.Before(candidates[j].UpdatedAt) })
	return candidates
}

// memoryConsolidationWindow picks the candidates that fit the input cap,
// starting at the offset the previous run left off. Bodies are never cut:
// the model replaces bodies wholesale, so it must see everything it may
// rewrite. The returned start is 0 once the window reaches the end.
func memoryConsolidationWindow(candidates []chattool.Memory, start int32) (window []chattool.Memory, nextStart int32) {
	if int(start) >= len(candidates) || start < 0 {
		start = 0
	}
	size := len(memoryConsolidationInputHeader)
	for i := int(start); i < len(candidates); i++ {
		size += len(formatMemoryConsolidationEntry(candidates[i]))
		if size > memoryConsolidationInputBytes && len(window) > 0 {
			// #nosec G115 -- bounded by the 200-memory cap.
			return window, int32(i)
		}
		window = append(window, candidates[i])
	}
	return window, 0
}

const memoryConsolidationInputHeader = "Memories, oldest first:\n"

func formatMemoryConsolidationEntry(memory chattool.Memory) string {
	return fmt.Sprintf("\n<memory name=%q updated_at=%q>\nDescription: %s\nBody:\n%s\n</memory>\n", memory.Name, memory.UpdatedAt.Format(time.RFC3339), memory.Description, memory.Body)
}

func formatMemoryConsolidationInput(memories []chattool.Memory) string {
	var b strings.Builder
	_, _ = b.WriteString(memoryConsolidationInputHeader)
	for _, memory := range memories {
		_, _ = b.WriteString(formatMemoryConsolidationEntry(memory))
	}
	return b.String()
}

// validateMemoryConsolidationMutations keeps the proposals that only touch
// memories in window, the set the model was shown. snapshot is the full
// memory set; a merge into a name that exists there but not in the window
// would overwrite a body the model never saw, so it is rejected.
func validateMemoryConsolidationMutations(proposed []memoryConsolidationMutation, window, snapshot []chattool.Memory, now time.Time) []memoryConsolidationMutation {
	byName := make(map[string]chattool.Memory, len(window))
	for _, memory := range window {
		byName[memory.Name] = memory
	}
	unseen := make(map[string]struct{}, len(snapshot))
	for _, memory := range snapshot {
		if _, shown := byName[memory.Name]; !shown {
			unseen[memory.Name] = struct{}{}
		}
	}
	// Each memory may be touched by one mutation per run. Overlapping
	// proposals would otherwise fail apply-time revalidation after the
	// first one changes the row, so they are rejected up front.
	claimed := make(map[string]struct{})
	valid := make([]memoryConsolidationMutation, 0, min(len(proposed), memoryConsolidationMaxMutations))
	for _, mutation := range proposed[:min(len(proposed), memoryConsolidationMaxMutations)] {
		mutation.Op = strings.ToLower(strings.TrimSpace(mutation.Op))
		var touched []string
		switch mutation.Op {
		case "merge":
			mutation.Into = strings.ToLower(strings.TrimSpace(mutation.Into))
			if _, hidden := unseen[mutation.Into]; hidden {
				continue
			}
			_, intoExists := byName[mutation.Into]
			// A merge must combine something: two sources into a new name,
			// or at least one source into an existing memory. Anything less
			// is an overwrite the model should have proposed as an update.
			minSources := 2
			if intoExists {
				minSources = 1
			}
			if err := chattool.ValidateMemoryName(mutation.Into); err != nil || len(mutation.From) < minSources {
				continue
			}
			seen := map[string]struct{}{}
			proposedFrom := mutation.From
			mutation.From = nil
			for _, from := range proposedFrom {
				name := strings.ToLower(strings.TrimSpace(from))
				if err := chattool.ValidateMemoryName(name); err != nil {
					continue
				}
				if _, exists := byName[name]; !exists || name == mutation.Into {
					continue
				}
				if _, duplicate := seen[name]; duplicate {
					continue
				}
				seen[name] = struct{}{}
				mutation.From = append(mutation.From, name)
			}
			if len(mutation.From) < minSources {
				continue
			}
			description, body, ok := normalizeConsolidatedMemory(mutation.Description, mutation.Body)
			if !ok {
				continue
			}
			mutation.Description, mutation.Body = description, body
			touched = append(touched, mutation.From...)
			touched = append(touched, mutation.Into)
		case "update":
			mutation.Name = strings.ToLower(strings.TrimSpace(mutation.Name))
			if _, exists := byName[mutation.Name]; !exists {
				continue
			}
			description, body, ok := normalizeConsolidatedMemory(mutation.Description, mutation.Body)
			if !ok {
				continue
			}
			mutation.Description, mutation.Body = description, body
			touched = []string{mutation.Name}
		case "delete":
			mutation.Name = strings.ToLower(strings.TrimSpace(mutation.Name))
			if _, exists := byName[mutation.Name]; !exists {
				continue
			}
			touched = []string{mutation.Name}
		default:
			continue
		}
		rejected := false
		for _, name := range touched {
			if _, overlaps := claimed[name]; overlaps {
				rejected = true
				break
			}
			if memory, exists := byName[name]; exists && now.Sub(memory.UpdatedAt) < memoryConsolidationProtectWindow {
				rejected = true
				break
			}
		}
		if rejected {
			continue
		}
		for _, name := range touched {
			claimed[name] = struct{}{}
		}
		valid = append(valid, mutation)
	}
	return valid
}

func normalizeConsolidatedMemory(description, body string) (normalizedDescription, normalizedBody string, ok bool) {
	normalizedDescription = chattool.NormalizeMemoryText(description)
	normalizedBody = chattool.NormalizeMemoryText(body)
	if normalizedDescription == "" || len([]rune(normalizedDescription)) > chattool.MaxMemoryDescriptionChars || normalizedBody == "" || len(normalizedBody) > chattool.MaxMemoryBodyBytes {
		return "", "", false
	}
	return normalizedDescription, normalizedBody, true
}

func memoryConsolidationCount(count int64) int32 {
	// #nosec G115 -- MemoryStore enforces the 200-memory cap before each run.
	return int32(count)
}

func applyMemoryConsolidationMutations(ctx context.Context, store chattool.MemoryStore, mutations []memoryConsolidationMutation, snapshot []chattool.Memory, now time.Time) ([]memoryConsolidationMutation, error) {
	byName := make(map[string]chattool.Memory, len(snapshot))
	for _, memory := range snapshot {
		byName[memory.Name] = memory
	}

	// The scope lock comes before any row lock, matching save_memory's
	// order so the two cannot deadlock. It also serializes the absence
	// check for a new merge target against concurrent creation.
	if err := store.Lock(ctx); err != nil {
		return nil, xerrors.Errorf("lock memory scope: %w", err)
	}
	applied := make([]memoryConsolidationMutation, 0, len(mutations))
	for _, mutation := range mutations {
		current, err := memoryConsolidationMutationCurrent(ctx, store, mutation, byName, now)
		if err != nil {
			return nil, err
		}
		if !current {
			continue
		}

		switch mutation.Op {
		case "merge":
			// Sources go first so a merge into a new name at the cap has
			// room for the target instead of failing the whole run.
			for _, from := range mutation.From {
				if err := store.Delete(ctx, from); err != nil {
					return nil, err
				}
			}
			if _, err := store.Upsert(ctx, chattool.MemoryInput{Name: mutation.Into, Description: mutation.Description, Body: mutation.Body}); err != nil {
				return nil, err
			}
		case "update":
			if _, err := store.Upsert(ctx, chattool.MemoryInput{Name: mutation.Name, Description: mutation.Description, Body: mutation.Body}); err != nil {
				return nil, err
			}
		case "delete":
			if err := store.Delete(ctx, mutation.Name); err != nil {
				return nil, err
			}
		}
		applied = append(applied, mutation)
	}
	return applied, nil
}

func memoryConsolidationMutationCurrent(ctx context.Context, store chattool.MemoryStore, mutation memoryConsolidationMutation, snapshot map[string]chattool.Memory, now time.Time) (bool, error) {
	touched := mutation.From
	if mutation.Op != "merge" {
		touched = []string{mutation.Name}
	} else if _, exists := snapshot[mutation.Into]; exists {
		touched = append(touched, mutation.Into)
	} else {
		_, err := store.GetForUpdate(ctx, mutation.Into)
		if err == nil {
			return false, nil
		}
		if !errors.Is(err, chattool.ErrMemoryNotFound) {
			return false, err
		}
	}

	for _, name := range touched {
		expected := snapshot[name]
		memory, err := store.GetForUpdate(ctx, name)
		if errors.Is(err, chattool.ErrMemoryNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if !memory.UpdatedAt.Equal(expected.UpdatedAt) || now.Sub(memory.UpdatedAt) < memoryConsolidationProtectWindow {
			return false, nil
		}
	}
	return true, nil
}

func memoryConsolidationJournalMutations(mutations []memoryConsolidationMutation) []codersdk.ChatMemoryMutation {
	journal := make([]codersdk.ChatMemoryMutation, len(mutations))
	for i, mutation := range mutations {
		journal[i] = codersdk.ChatMemoryMutation{Op: mutation.Op, Name: mutation.Name, Into: mutation.Into, From: mutation.From, Reason: mutation.Reason}
	}
	return journal
}
