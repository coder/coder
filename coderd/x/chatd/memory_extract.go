package chatd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

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
	memoryExtractionWorkTimeout        = 120 * time.Second
	memoryExtractionModelTimeout       = 60 * time.Second
	memoryExtractionTranscriptMaxBytes = 24 * 1024
	memoryExtractionMaxOutputTokens    = 2048
	// The claim outlives the work timeout so a live extractor is never
	// treated as stale, while a crashed one is reclaimable soon after.
	memoryExtractionClaimTTL  = memoryExtractionWorkTimeout + time.Minute
	memoryExtractionMaxDrains = 5
)

// memoryScopeStatus reports why a chat has, or lacks, a memory scope.
type memoryScopeStatus int

const (
	// memoryScopeUnavailable covers subagents and transient lookup failures.
	memoryScopeUnavailable memoryScopeStatus = iota
	// memoryScopeDisabled means the user turned personal memory off. Turns
	// completed in this state must never be extracted later.
	memoryScopeDisabled
	memoryScopeAvailable
)

// The extractor is deliberately create-only. Dogfooding showed a model
// treating an assistant's "I don't know" as a contradiction and overwriting a
// correct memory; updates stay with the main agent's tool and the UI.
const memoryExtractionPrompt = "You review a completed coding-chat turn and record durable memory the main agent did not save itself. " +
	"%s " +
	chattool.MemoryGuidance + " " +
	"Record only facts the user stated or explicitly confirmed in this turn. " +
	"Never record that something is unknown, unspecified, undecided, or pending, and never record questions or the assistant's own guesses. " +
	"Skip anything already covered by a memory in the index; existing memories are updated by the main agent, not by you. " +
	"Most turns contain nothing new: return an empty list in that case."

type memoryExtraction struct {
	Upserts []memoryExtractionUpsert `json:"upserts"`
}

type memoryExtractionUpsert struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
}

// resolveMemoryScope returns the durable-memory store available to a chat.
func (p *Server) resolveMemoryScope(ctx context.Context, chat database.Chat) (chattool.MemoryStore, chattool.MemoryScope, memoryScopeStatus) {
	if chat.ParentChatID.Valid {
		return nil, chattool.MemoryScope{}, memoryScopeUnavailable
	}
	if chat.ProjectID.Valid {
		scope := chattool.MemoryScope{Kind: chattool.MemoryScopeProject}
		project, err := p.db.GetChatProjectByID(ctx, chat.ProjectID.UUID)
		if err != nil {
			p.logger.Debug(ctx, "failed to load chat project for memory scope", slog.F("chat_id", chat.ID), slog.Error(err))
		} else {
			scope.Label = project.Name
		}
		return chattool.NewProjectMemoryStore(p.db, chat.ProjectID.UUID, chat.OrganizationID, chat.ID, chat.OwnerID), scope, memoryScopeAvailable
	}
	enabled, err := p.configCache.GetUserChatPersonalMemoryEnabled(ctx, chat.OwnerID)
	if err != nil {
		p.logger.Debug(ctx, "failed to load personal memory setting", slog.F("chat_id", chat.ID), slog.Error(err))
		return nil, chattool.MemoryScope{}, memoryScopeUnavailable
	}
	if !enabled {
		return nil, chattool.MemoryScope{}, memoryScopeDisabled
	}
	return chattool.NewPersonalMemoryStore(p.db, chat.OwnerID, chat.OrganizationID, chat.ID), chattool.MemoryScope{Kind: chattool.MemoryScopePersonal}, memoryScopeAvailable
}

func (p *Server) maybeExtractMemoriesAsync(ctx context.Context, logger slog.Logger, chat database.Chat) {
	if chat.ParentChatID.Valid {
		return
	}
	extractCtx, cancel := p.inflightContext(ctx)
	if err := p.goInflight(func() {
		defer cancel()
		p.extractMemories(extractCtx, logger, chat)
	}); err != nil {
		cancel()
		logger.Debug(ctx, "skipped memory extraction", slog.F("chat_id", chat.ID), slog.Error(err))
	}
}

func (p *Server) extractMemories(ctx context.Context, logger slog.Logger, chat database.Chat) {
	ctx, cancel := context.WithTimeout(ctx, memoryExtractionWorkTimeout)
	defer cancel()
	//nolint:gocritic // Background memory extraction acts as the chat daemon.
	ctx = dbauthz.AsChatd(ctx)

	// Turns can finish faster than extraction runs. Exactly one extractor
	// owns a chat at a time; the owner drains any turns that completed
	// while it was working, and a rival that fails to claim simply exits.
	for range memoryExtractionMaxDrains {
		claim, err := p.db.ClaimChatMemoryExtraction(ctx, database.ClaimChatMemoryExtractionParams{
			ChatID:       chat.ID,
			ClaimedUntil: p.clock.Now().Add(memoryExtractionClaimTTL),
		})
		if errors.Is(err, sql.ErrNoRows) {
			return
		}
		if err != nil {
			logger.Debug(ctx, "failed to claim memory extraction", slog.F("chat_id", chat.ID), slog.Error(err))
			return
		}
		processed, more := p.extractMemoriesOnce(ctx, logger, chat.ID, claim.HistoryVersion)
		if err := p.db.ReleaseChatMemoryExtraction(ctx, database.ReleaseChatMemoryExtractionParams{ChatID: chat.ID, ClaimedUntil: claim.ClaimedUntil.Time}); err != nil {
			logger.Debug(ctx, "failed to release memory extraction claim", slog.F("chat_id", chat.ID), slog.Error(err))
		}
		if !processed || !more {
			return
		}
	}
}

// extractMemoriesOnce runs one extraction pass from the cursor to the chat's
// current history version. It reports whether the cursor advanced and
// whether the chat has moved on since, in which case the caller loops.
func (p *Server) extractMemoriesOnce(ctx context.Context, logger slog.Logger, chatID uuid.UUID, cursor int64) (processed, more bool) {
	chat, err := p.db.GetChatByID(ctx, chatID)
	if err != nil {
		logger.Debug(ctx, "failed to re-read chat for memory extraction", slog.Error(err))
		return false, false
	}
	if chat.HistoryVersion <= cursor {
		return false, false
	}
	advance := func() bool {
		if _, err := p.db.UpsertChatMemoryCursor(ctx, database.UpsertChatMemoryCursorParams{ChatID: chat.ID, HistoryVersion: chat.HistoryVersion}); err != nil {
			logger.Debug(ctx, "failed to advance memory cursor", slog.F("chat_id", chat.ID), slog.Error(err))
			return false
		}
		return true
	}
	hasMore := func() bool {
		current, err := p.db.GetChatByID(ctx, chat.ID)
		return err == nil && current.HistoryVersion > chat.HistoryVersion
	}

	store, scope, status := p.resolveMemoryScope(ctx, chat)
	switch status {
	case memoryScopeUnavailable:
		return false, false
	case memoryScopeDisabled:
		// Fence the turns completed while memory was off so re-enabling it
		// later never extracts them retroactively.
		return advance(), false
	case memoryScopeAvailable:
	}

	messages, err := p.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	if err != nil {
		logger.Debug(ctx, "failed to load memory transcript", slog.F("chat_id", chat.ID), slog.Error(err))
		return false, false
	}
	// Turns where the main agent curated memory itself are excluded per
	// turn, not for the whole window: running the extractor over them
	// mostly produced split duplicates, but later turns in a lagging window
	// still deserve extraction.
	transcript := renderMemoryTranscript(messages, cursor)
	if transcript == "" {
		return advance(), hasMore()
	}
	entries, err := store.List(ctx)
	if err != nil {
		logger.Debug(ctx, "failed to load memories for extraction", slog.F("chat_id", chat.ID), slog.Error(err))
		return false, false
	}

	apiKeyID, err := p.ensureSyntheticAPIKeyID(ctx, chat.OwnerID)
	if err != nil {
		logger.Debug(ctx, "failed to ensure synthetic API key for memory extraction", slog.Error(err))
		return false, false
	}
	resolved, err := p.resolveModelCall(ctx, modelCallSpec{purpose: "memory_extraction", chat: chat, buildOptions: modelBuildOptions{ActiveAPIKeyID: apiKeyID}})
	if err != nil {
		logger.Debug(ctx, "failed to resolve model for memory extraction", slog.Error(err))
		return false, false
	}
	call := resolved.newObjectCall("memory_extraction", "Record new durable memories stated by the user in this turn.", memoryExtractionMaxOutputTokens)
	call.Prompt = quickgenPrompt(fmt.Sprintf(memoryExtractionPrompt, scope.Intro()), fmt.Sprintf("Current memory index:\n%s\n\nNew user messages:\n%s", chattool.FormatMemoryIndexForTool(entries), transcript))
	modelCtx, cancelModel := context.WithTimeout(ctx, memoryExtractionModelTimeout)
	defer cancelModel()
	result, err := generateQuickgenObject[memoryExtraction](modelCtx, resolved.model.LanguageModel(), call)
	if err != nil {
		logger.Debug(ctx, "failed to generate memory extraction", slog.F("chat_id", chat.ID), slog.Error(err))
		return false, false
	}
	for _, upsert := range result.Object.Upserts {
		if err := applyMemoryUpsert(ctx, store, upsert); err != nil {
			logger.Debug(ctx, "ignored invalid memory upsert", slog.F("chat_id", chat.ID), slog.F("name", upsert.Name), slog.Error(err))
		}
	}
	return advance(), hasMore()
}

// applyMemoryUpsert records a memory the extractor proposed. It only creates:
// updates to existing memories are reserved for the main agent's tool and UI.
func applyMemoryUpsert(ctx context.Context, store chattool.MemoryStore, upsert memoryExtractionUpsert) error {
	input, err := normalizeMemoryExtraction(upsert)
	if err != nil {
		return err
	}
	_, err = store.Insert(ctx, input)
	if errors.Is(err, chattool.ErrMemoryExists) {
		return nil
	}
	return err
}

func normalizeMemoryExtraction(upsert memoryExtractionUpsert) (chattool.MemoryInput, error) {
	name := strings.ToLower(strings.TrimSpace(upsert.Name))
	if err := chattool.ValidateMemoryName(name); err != nil {
		return chattool.MemoryInput{}, err
	}
	description := chattool.NormalizeMemoryText(upsert.Description)
	body := chattool.NormalizeMemoryText(upsert.Body)
	if description == "" || len([]rune(description)) > chattool.MaxMemoryDescriptionChars {
		return chattool.MemoryInput{}, xerrors.New("invalid memory description")
	}
	if body == "" || len(body) > chattool.MaxMemoryBodyBytes {
		return chattool.MemoryInput{}, xerrors.New("invalid memory body")
	}
	return chattool.MemoryInput{Name: name, Description: description, Body: body}, nil
}

// memoryTurn is one user turn inside the extraction window: the user's
// visible text and whether the assistant saved or deleted memory
// successfully in response.
type memoryTurn struct {
	userText  []string
	usedTools bool
}

// memoryTurns splits the messages written after the given history version
// into turns. Message revisions hold the snapshot version that wrote them,
// so this window matches the cursor fence exactly instead of relying on
// wall-clock timestamps. A tool call counts only when its result was not an
// error; a failed save must not suppress extraction.
func memoryTurns(messages []database.ChatMessage, afterHistoryVersion int64) []memoryTurn {
	var turns []memoryTurn
	callTurn := make(map[string]int)
	for _, message := range messages {
		if message.Revision <= afterHistoryVersion {
			continue
		}
		parts, err := chatprompt.ParseContent(message)
		if err != nil {
			continue
		}
		switch message.Role {
		case database.ChatMessageRoleUser:
			if message.Visibility != database.ChatMessageVisibilityBoth && message.Visibility != database.ChatMessageVisibilityUser {
				continue
			}
			text := strings.TrimSpace(contentBlocksToText(parts))
			if text == "" {
				continue
			}
			turns = append(turns, memoryTurn{userText: []string{fmt.Sprintf("[%s]: %s", message.Role, text)}})
		case database.ChatMessageRoleAssistant:
			if len(turns) == 0 {
				continue
			}
			for _, part := range parts {
				if part.Type != codersdk.ChatMessagePartTypeToolCall {
					continue
				}
				switch part.ToolName {
				case chattool.SaveMemoryToolName, chattool.DeleteMemoryToolName:
					callTurn[part.ToolCallID] = len(turns) - 1
				}
			}
		case database.ChatMessageRoleTool:
			for _, part := range parts {
				if part.Type != codersdk.ChatMessagePartTypeToolResult || part.IsError {
					continue
				}
				if turn, ok := callTurn[part.ToolCallID]; ok {
					turns[turn].usedTools = true
				}
			}
		}
	}
	return turns
}

// renderMemoryTranscript renders the user text of every turn after the given
// history version in which the main agent did not curate memory itself.
// Assistant text is excluded on purpose: the extractor records what the user
// said, and dogfooding showed it re-recording the assistant's restatement of
// existing memories.
func renderMemoryTranscript(messages []database.ChatMessage, afterHistoryVersion int64) string {
	var lines []string
	for _, turn := range memoryTurns(messages, afterHistoryVersion) {
		if turn.usedTools {
			continue
		}
		lines = append(lines, turn.userText...)
	}
	for len(strings.Join(lines, "\n")) > memoryExtractionTranscriptMaxBytes && len(lines) > 1 {
		lines = lines[1:]
	}
	transcript := strings.Join(lines, "\n")
	if len(transcript) <= memoryExtractionTranscriptMaxBytes {
		return transcript
	}

	const truncatedPrefix = "[truncated] "
	tailStart := len(transcript) - (memoryExtractionTranscriptMaxBytes - len(truncatedPrefix))
	for tailStart < len(transcript) && !utf8.RuneStart(transcript[tailStart]) {
		tailStart++
	}
	return truncatedPrefix + transcript[tailStart:]
}
