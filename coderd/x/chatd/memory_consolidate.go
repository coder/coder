package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
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
	memoryConsolidationMinMemories     = 20
	memoryConsolidationDebounce        = 24 * time.Hour
	memoryConsolidationRunningStale    = 10 * time.Minute
	memoryConsolidationMaxMutations    = 8
	memoryConsolidationModelTimeout    = 90 * time.Second
	memoryConsolidationWorkTimeout     = 3 * time.Minute
	memoryConsolidationBodyBytes       = 2048
	memoryConsolidationInputBytes      = 96 * 1024
	memoryConsolidationProtectWindow   = 24 * time.Hour
	memoryConsolidationKeepRecords     = 20
	memoryConsolidationMaxOutputTokens = 2048
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
	projectID      uuid.NullUUID
	userID         uuid.NullUUID
}

func memoryConsolidationScopeForChat(chat database.Chat) memoryConsolidationScope {
	if chat.ProjectID.Valid {
		return memoryConsolidationScope{organizationID: chat.OrganizationID, projectID: chat.ProjectID}
	}
	return memoryConsolidationScope{organizationID: chat.OrganizationID, userID: uuid.NullUUID{UUID: chat.OwnerID, Valid: true}}
}

func (s memoryConsolidationScope) lockID() int64 {
	id := s.userID.UUID
	if s.projectID.Valid {
		id = s.projectID.UUID
	}
	return database.GenLockID("chat-memory-consolidation:" + id.String())
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
	if memoryConsolidationDebounced(ctx, p.db, scope, before, now, logger) {
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
		if memoryConsolidationDebounced(ctx, tx, scope, before, now, logger) {
			return nil
		}
		record, err = tx.InsertChatMemoryConsolidation(ctx, database.InsertChatMemoryConsolidationParams{
			OrganizationID: scope.organizationID,
			ProjectID:      scope.projectID,
			UserID:         scope.userID,
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

	finish := func(status database.ChatMemoryConsolidationStatus, after int64, mutations []codersdk.ChatMemoryMutation, finishErr error) {
		if err := p.finishMemoryConsolidation(ctx, scope, record.ID, status, after, mutations, finishErr); err != nil {
			logger.Warn(ctx, "failed to finish memory consolidation", slog.F("chat_id", chat.ID), slog.Error(err))
		}
	}

	memories, err := store.ListFull(ctx)
	if err != nil {
		finish(database.ChatMemoryConsolidationStatusFailed, before, nil, xerrors.Errorf("load memories: %w", err))
		return
	}
	call := resolved.newObjectCall("memory_consolidation", "Consolidate durable chat memories.", memoryConsolidationMaxOutputTokens)
	call.Prompt = quickgenPrompt(memoryConsolidationPrompt, formatMemoryConsolidationInput(memories))
	modelCtx, cancelModel := context.WithTimeout(ctx, memoryConsolidationModelTimeout)
	result, err := generateQuickgenObject[memoryConsolidation](modelCtx, resolved.model.LanguageModel(), call)
	cancelModel()
	if err != nil {
		finish(database.ChatMemoryConsolidationStatusFailed, before, nil, xerrors.Errorf("generate consolidation: %w", err))
		return
	}

	mutations := validateMemoryConsolidationMutations(result.Object.Mutations, memories, time.Now())
	if len(mutations) == 0 {
		finish(database.ChatMemoryConsolidationStatusSkipped, before, nil, nil)
		return
	}
	if err := store.InTx(func(txStore chattool.MemoryStore) error {
		return applyMemoryConsolidationMutations(ctx, txStore, mutations)
	}); err != nil {
		finish(database.ChatMemoryConsolidationStatusFailed, before, nil, xerrors.Errorf("apply mutations: %w", err))
		return
	}
	after, err := store.Count(ctx)
	if err != nil {
		finish(database.ChatMemoryConsolidationStatusFailed, before, nil, xerrors.Errorf("count consolidated memories: %w", err))
		return
	}
	finish(database.ChatMemoryConsolidationStatusSucceeded, after, memoryConsolidationJournalMutations(mutations), nil)
}

// memoryConsolidationDebounced reports whether the scope was consolidated
// recently. It takes the store explicitly so the transactional check runs
// against the transaction that also inserts the running record.
func memoryConsolidationDebounced(ctx context.Context, db database.Store, scope memoryConsolidationScope, count int64, now time.Time, logger slog.Logger) bool {
	if count >= chattool.MaxMemories {
		return false
	}
	var (
		record database.ChatMemoryConsolidation
		err    error
	)
	if scope.projectID.Valid {
		record, err = db.GetLatestChatMemoryConsolidationByProject(ctx, scope.projectID.UUID)
	} else {
		record, err = db.GetLatestChatMemoryConsolidationByUser(ctx, database.GetLatestChatMemoryConsolidationByUserParams{UserID: scope.userID.UUID, OrganizationID: scope.organizationID})
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		logger.Debug(ctx, "failed to load latest memory consolidation", slog.Error(err))
		return true
	}
	window := memoryConsolidationDebounce
	if record.Status == database.ChatMemoryConsolidationStatusRunning {
		window = memoryConsolidationRunningStale
	}
	return now.Sub(record.StartedAt) < window
}

func (p *Server) finishMemoryConsolidation(ctx context.Context, scope memoryConsolidationScope, id uuid.UUID, status database.ChatMemoryConsolidationStatus, after int64, mutations []codersdk.ChatMemoryMutation, finishErr error) error {
	encoded, err := json.Marshal(mutations)
	if err != nil {
		return xerrors.Errorf("marshal mutations: %w", err)
	}
	errText := ""
	if finishErr != nil {
		errText = finishErr.Error()
	}
	if _, err := p.db.FinishChatMemoryConsolidation(ctx, database.FinishChatMemoryConsolidationParams{ID: id, Status: status, MemoriesAfter: memoryConsolidationCount(after), Mutations: encoded, Error: errText}); err != nil {
		return err
	}
	if scope.projectID.Valid {
		return p.db.PruneChatMemoryConsolidationsByProject(ctx, database.PruneChatMemoryConsolidationsByProjectParams{ProjectID: scope.projectID.UUID, KeepCount: memoryConsolidationKeepRecords})
	}
	return p.db.PruneChatMemoryConsolidationsByUser(ctx, database.PruneChatMemoryConsolidationsByUserParams{UserID: scope.userID.UUID, OrganizationID: scope.organizationID, KeepCount: memoryConsolidationKeepRecords})
}

func formatMemoryConsolidationInput(memories []chattool.Memory) string {
	var b strings.Builder
	_, _ = b.WriteString("Memories, newest first:\n")
	for _, memory := range memories {
		body := memory.Body
		if len(body) > memoryConsolidationBodyBytes {
			body = body[:memoryConsolidationBodyBytes]
		}
		entry := fmt.Sprintf("\n<memory name=%q updated_at=%q>\nDescription: %s\nBody:\n%s\n</memory>\n", memory.Name, memory.UpdatedAt.Format(time.RFC3339), memory.Description, body)
		if b.Len()+len(entry) > memoryConsolidationInputBytes {
			break
		}
		_, _ = b.WriteString(entry)
	}
	return b.String()
}

func validateMemoryConsolidationMutations(proposed []memoryConsolidationMutation, memories []chattool.Memory, now time.Time) []memoryConsolidationMutation {
	byName := make(map[string]chattool.Memory, len(memories))
	for _, memory := range memories {
		byName[memory.Name] = memory
	}
	valid := make([]memoryConsolidationMutation, 0, min(len(proposed), memoryConsolidationMaxMutations))
	for _, mutation := range proposed[:min(len(proposed), memoryConsolidationMaxMutations)] {
		mutation.Op = strings.ToLower(strings.TrimSpace(mutation.Op))
		var touched []string
		switch mutation.Op {
		case "merge":
			mutation.Into = strings.ToLower(strings.TrimSpace(mutation.Into))
			if err := chattool.ValidateMemoryName(mutation.Into); err != nil || len(mutation.From) < 2 {
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
			if len(mutation.From) < 2 {
				continue
			}
			description, body, ok := normalizeConsolidatedMemory(mutation.Description, mutation.Body)
			if !ok {
				continue
			}
			mutation.Description, mutation.Body = description, body
			touched = append(touched, mutation.From...)
			if _, exists := byName[mutation.Into]; exists {
				touched = append(touched, mutation.Into)
			}
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
		fresh := false
		for _, name := range touched {
			if now.Sub(byName[name].UpdatedAt) < memoryConsolidationProtectWindow {
				fresh = true
				break
			}
		}
		if !fresh {
			valid = append(valid, mutation)
			switch mutation.Op {
			case "merge":
				for _, from := range mutation.From {
					delete(byName, from)
				}
				byName[mutation.Into] = chattool.Memory{Name: mutation.Into, Description: mutation.Description, Body: mutation.Body}
			case "update":
				memory := byName[mutation.Name]
				memory.Description = mutation.Description
				memory.Body = mutation.Body
				byName[mutation.Name] = memory
			case "delete":
				delete(byName, mutation.Name)
			}
		}
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

func applyMemoryConsolidationMutations(ctx context.Context, store chattool.MemoryStore, mutations []memoryConsolidationMutation) error {
	for _, mutation := range mutations {
		switch mutation.Op {
		case "merge":
			if _, err := store.Upsert(ctx, chattool.MemoryInput{Name: mutation.Into, Description: mutation.Description, Body: mutation.Body}); err != nil {
				return err
			}
			for _, from := range mutation.From {
				if err := store.Delete(ctx, from); err != nil {
					return err
				}
			}
		case "update":
			if _, err := store.Upsert(ctx, chattool.MemoryInput{Name: mutation.Name, Description: mutation.Description, Body: mutation.Body}); err != nil {
				return err
			}
		case "delete":
			if err := store.Delete(ctx, mutation.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

func memoryConsolidationJournalMutations(mutations []memoryConsolidationMutation) []codersdk.ChatMemoryMutation {
	journal := make([]codersdk.ChatMemoryMutation, len(mutations))
	for i, mutation := range mutations {
		journal[i] = codersdk.ChatMemoryMutation{Op: mutation.Op, Name: mutation.Name, Into: mutation.Into, From: mutation.From, Reason: mutation.Reason}
	}
	return journal
}
