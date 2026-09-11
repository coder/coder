package chatd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
)

const (
	projectMemoryExtractionWorkTimeout        = 120 * time.Second
	projectMemoryExtractionModelTimeout       = 60 * time.Second
	projectMemoryExtractionTranscriptMaxBytes = 24 * 1024
	projectMemoryExtractionMaxOutputTokens    = 2048
)

// The extractor is deliberately upsert-only. Dogfooding showed a model
// treating an assistant's "I don't know" as a contradiction and deleting a
// correct memory; deletion stays with the main agent's tool and the UI.
const projectMemoryExtractionPrompt = "You review a completed coding-chat turn and record project memory the main agent did not save itself. " +
	chattool.ProjectMemoryGuidance + " " +
	"Record only facts the user stated or explicitly confirmed in this turn. " +
	"Never record that something is unknown, unspecified, undecided, or pending, and never record questions or the assistant's own guesses. " +
	"Skip facts that already appear in the memory index unless the user changed them, and reuse the existing name when updating. " +
	"Most turns contain nothing new: return an empty list in that case."

type projectMemoryExtraction struct {
	Upserts []projectMemoryExtractionUpsert `json:"upserts"`
}

type projectMemoryExtractionUpsert struct {
	Name        string                         `json:"name"`
	Type        database.ChatProjectMemoryType `json:"type"`
	Description string                         `json:"description"`
	Body        string                         `json:"body"`
}

func (p *Server) maybeExtractProjectMemoriesAsync(ctx context.Context, logger slog.Logger, chat database.Chat) {
	if chat.ParentChatID.Valid || !chat.ProjectID.Valid || !p.experiments.Enabled(codersdk.ExperimentChatProjects) {
		return
	}
	extractCtx, cancel := p.inflightContext(ctx)
	if err := p.goInflight(func() {
		defer cancel()
		p.extractProjectMemories(extractCtx, logger, chat)
	}); err != nil {
		cancel()
		logger.Debug(ctx, "skipped project memory extraction", slog.F("chat_id", chat.ID), slog.Error(err))
	}
}

func (p *Server) extractProjectMemories(ctx context.Context, logger slog.Logger, chat database.Chat) {
	ctx, cancel := context.WithTimeout(ctx, projectMemoryExtractionWorkTimeout)
	defer cancel()
	//nolint:gocritic // Background project-memory extraction acts as the chat daemon.
	ctx = dbauthz.AsChatd(ctx)

	chat, err := p.db.GetChatByID(ctx, chat.ID)
	if err != nil || !chat.ProjectID.Valid {
		if err != nil {
			logger.Debug(ctx, "failed to re-read chat for project memory extraction", slog.Error(err))
		}
		return
	}
	cursor, err := p.db.GetChatProjectMemoryCursor(ctx, chat.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		logger.Debug(ctx, "failed to read project memory cursor", slog.F("chat_id", chat.ID), slog.Error(err))
		return
	}
	// A missing cursor leaves HistoryVersion at 0, which admits every message.
	if err == nil && chat.HistoryVersion <= cursor.HistoryVersion {
		return
	}

	messages, err := p.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	if err != nil {
		logger.Debug(ctx, "failed to load project memory transcript", slog.F("chat_id", chat.ID), slog.Error(err))
		return
	}
	transcript := renderProjectMemoryTranscript(messages, cursor.HistoryVersion)
	if transcript == "" {
		return
	}
	if turnUsedProjectMemoryTools(messages, cursor.HistoryVersion) {
		// The main agent curated memory itself this turn. Running the
		// extractor on top of that mostly produced split duplicates of
		// what it had just saved, so advance the cursor and stop.
		if _, err := p.db.UpsertChatProjectMemoryCursor(ctx, database.UpsertChatProjectMemoryCursorParams{ChatID: chat.ID, HistoryVersion: chat.HistoryVersion}); err != nil {
			logger.Debug(ctx, "failed to advance project memory cursor", slog.F("chat_id", chat.ID), slog.Error(err))
		}
		return
	}
	memories, err := p.db.GetChatProjectMemoriesByProjectID(ctx, chat.ProjectID.UUID)
	if err != nil {
		logger.Debug(ctx, "failed to load project memories for extraction", slog.F("chat_id", chat.ID), slog.Error(err))
		return
	}
	entries := make([]chattool.ProjectMemoryIndexEntry, len(memories))
	for i, memory := range memories {
		entries[i] = chattool.ProjectMemoryIndexEntry{Name: memory.ChatProjectMemory.Name, Type: memory.ChatProjectMemory.Type, Description: memory.ChatProjectMemory.Description}
	}

	apiKeyID, err := p.ensureSyntheticAPIKeyID(ctx, chat.OwnerID)
	if err != nil {
		logger.Debug(ctx, "failed to ensure synthetic API key for project memory extraction", slog.Error(err))
		return
	}
	resolved, err := p.resolveModelCall(ctx, modelCallSpec{purpose: "project_memory_extraction", chat: chat, buildOptions: modelBuildOptions{ActiveAPIKeyID: apiKeyID}})
	if err != nil {
		logger.Debug(ctx, "failed to resolve model for project memory extraction", slog.Error(err))
		return
	}
	call := resolved.newObjectCall("project_memory_extraction", "Record new project memories stated by the user in this turn.", projectMemoryExtractionMaxOutputTokens)
	call.Prompt = quickgenPrompt(projectMemoryExtractionPrompt, fmt.Sprintf("Current memory index:\n%s\n\nNew user messages:\n%s", chattool.FormatProjectMemoryIndex(entries), transcript))
	modelCtx, cancelModel := context.WithTimeout(ctx, projectMemoryExtractionModelTimeout)
	defer cancelModel()
	result, err := generateQuickgenObject[projectMemoryExtraction](modelCtx, resolved.model.LanguageModel(), call)
	if err != nil {
		logger.Debug(ctx, "failed to generate project memory extraction", slog.F("chat_id", chat.ID), slog.Error(err))
		return
	}
	for _, upsert := range result.Object.Upserts {
		if err := applyProjectMemoryUpsert(ctx, p.db, chat, upsert); err != nil {
			logger.Debug(ctx, "ignored invalid project memory upsert", slog.F("chat_id", chat.ID), slog.F("name", upsert.Name), slog.Error(err))
		}
	}
	if _, err := p.db.UpsertChatProjectMemoryCursor(ctx, database.UpsertChatProjectMemoryCursorParams{ChatID: chat.ID, HistoryVersion: chat.HistoryVersion}); err != nil {
		logger.Debug(ctx, "failed to advance project memory cursor", slog.F("chat_id", chat.ID), slog.Error(err))
	}
}

func applyProjectMemoryUpsert(ctx context.Context, store database.Store, chat database.Chat, upsert projectMemoryExtractionUpsert) error {
	normalized, err := normalizeProjectMemoryExtraction(upsert)
	if err != nil {
		return err
	}
	name, memoryType, description, body := normalized.Name, normalized.Type, normalized.Description, normalized.Body
	_, existingErr := store.GetChatProjectMemoryByName(ctx, database.GetChatProjectMemoryByNameParams{ProjectID: chat.ProjectID.UUID, Name: name})
	if existingErr != nil {
		count, countErr := store.CountChatProjectMemoriesByProjectID(ctx, chat.ProjectID.UUID)
		if countErr != nil {
			return xerrors.Errorf("count project memories: %w", countErr)
		}
		if count >= chattool.MaxProjectMemories {
			return xerrors.New("project memory limit reached")
		}
	}
	_, err = store.UpsertChatProjectMemoryByName(ctx, database.UpsertChatProjectMemoryByNameParams{
		ProjectID: chat.ProjectID.UUID, OrganizationID: chat.OrganizationID, Type: memoryType,
		Name: name, Description: description, Body: body,
		SourceChatID: uuid.NullUUID{UUID: chat.ID, Valid: true}, CreatedBy: chat.OwnerID,
	})
	return err
}

type normalizedProjectMemoryExtraction struct {
	Name        string
	Type        database.ChatProjectMemoryType
	Description string
	Body        string
}

func normalizeProjectMemoryExtraction(upsert projectMemoryExtractionUpsert) (normalizedProjectMemoryExtraction, error) {
	name := strings.ToLower(strings.TrimSpace(upsert.Name))
	if err := chattool.ValidateProjectMemoryName(name); err != nil {
		return normalizedProjectMemoryExtraction{}, err
	}
	if !upsert.Type.Valid() {
		return normalizedProjectMemoryExtraction{}, xerrors.New("invalid memory type")
	}
	description := chattool.NormalizeProjectMemoryText(upsert.Description)
	body := chattool.NormalizeProjectMemoryText(upsert.Body)
	if description == "" || len([]rune(description)) > chattool.MaxProjectMemoryDescriptionChars {
		return normalizedProjectMemoryExtraction{}, xerrors.New("invalid memory description")
	}
	if body == "" || len(body) > chattool.MaxProjectMemoryBodyBytes {
		return normalizedProjectMemoryExtraction{}, xerrors.New("invalid memory body")
	}
	return normalizedProjectMemoryExtraction{Name: name, Type: upsert.Type, Description: description, Body: body}, nil
}

// turnUsedProjectMemoryTools reports whether the messages written after the
// given history version contain a save or delete project memory tool call.
func turnUsedProjectMemoryTools(messages []database.ChatMessage, afterHistoryVersion int64) bool {
	for _, message := range messages {
		if message.Revision <= afterHistoryVersion || message.Role != database.ChatMessageRoleAssistant {
			continue
		}
		parts, err := chatprompt.ParseContent(message)
		if err != nil {
			continue
		}
		for _, part := range parts {
			if part.Type != codersdk.ChatMessagePartTypeToolCall {
				continue
			}
			switch part.ToolName {
			case chattool.SaveProjectMemoryToolName, chattool.DeleteProjectMemoryToolName:
				return true
			}
		}
	}
	return false
}

// renderProjectMemoryTranscript renders the visible user text written after
// the given history version. Message revisions hold the snapshot version
// that wrote them, so this window matches the cursor fence exactly instead
// of relying on wall-clock timestamps. Assistant text is excluded on
// purpose: the extractor records what the user said, and dogfooding showed
// it re-recording the assistant's restatement of existing memories.
func renderProjectMemoryTranscript(messages []database.ChatMessage, afterHistoryVersion int64) string {
	var lines []string
	for _, message := range messages {
		if message.Revision <= afterHistoryVersion {
			continue
		}
		if message.Role != database.ChatMessageRoleUser {
			continue
		}
		if message.Visibility != database.ChatMessageVisibilityBoth && message.Visibility != database.ChatMessageVisibilityUser {
			continue
		}
		parts, err := chatprompt.ParseContent(message)
		if err != nil {
			continue
		}
		text := strings.TrimSpace(contentBlocksToText(parts))
		if text == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("[%s]: %s", message.Role, text))
	}
	for len(strings.Join(lines, "\n")) > projectMemoryExtractionTranscriptMaxBytes && len(lines) > 1 {
		lines = lines[1:]
	}
	return strings.Join(lines, "\n")
}
