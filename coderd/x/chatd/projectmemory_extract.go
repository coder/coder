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
	projectMemoryExtractionWorkTimeout        = 120 * time.Second
	projectMemoryExtractionModelTimeout       = 60 * time.Second
	projectMemoryExtractionTranscriptMaxBytes = 24 * 1024
	projectMemoryExtractionMaxOutputTokens    = 2048
	// The claim outlives the work timeout so a live extractor is never
	// treated as stale, while a crashed one is reclaimable soon after.
	projectMemoryExtractionClaimTTL  = projectMemoryExtractionWorkTimeout + time.Minute
	projectMemoryExtractionMaxDrains = 5
)

// The extractor is deliberately create-only. Dogfooding showed a model
// treating an assistant's "I don't know" as a contradiction and overwriting a
// correct memory; updates stay with the main agent's tool and the UI.
const projectMemoryExtractionPrompt = "You review a completed coding-chat turn and record project memory the main agent did not save itself. " +
	chattool.ProjectMemoryGuidance + " " +
	"Record only facts the user stated or explicitly confirmed in this turn. " +
	"Never record that something is unknown, unspecified, undecided, or pending, and never record questions or the assistant's own guesses. " +
	"Skip anything already covered by a memory in the index; existing memories are updated by the main agent, not by you. " +
	"Most turns contain nothing new: return an empty list in that case."

type projectMemoryExtraction struct {
	Upserts []projectMemoryExtractionUpsert `json:"upserts"`
}

type projectMemoryExtractionUpsert struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
}

// errInvalidProjectMemoryUpsert marks a proposal the model got wrong, which is
// dropped, as opposed to a storage failure, which must not advance the cursor.
var errInvalidProjectMemoryUpsert = xerrors.New("invalid project memory upsert")

func (p *Server) maybeExtractProjectMemoriesAsync(ctx context.Context, logger slog.Logger, chat database.Chat) {
	if chat.ParentChatID.Valid || !chat.ProjectID.Valid || !p.experiments.Enabled(codersdk.ExperimentChatProjects) {
		return
	}
	// FinishTurn promotes a queued message and hands back a chat that is
	// already running that turn. Extracting now would process the promoted
	// user message before its assistant could save memory itself; the
	// promoted turn's own completion covers both.
	if chat.Status == database.ChatStatusRunning {
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

	// Turns can finish faster than extraction runs. Exactly one extractor
	// owns a chat at a time; the owner drains any turns that completed
	// while it was working, and a rival that fails to claim simply exits.
	for range projectMemoryExtractionMaxDrains {
		claim, err := p.db.ClaimChatProjectMemoryExtraction(ctx, database.ClaimChatProjectMemoryExtractionParams{
			ChatID:       chat.ID,
			ClaimedUntil: p.clock.Now().Add(projectMemoryExtractionClaimTTL),
		})
		if errors.Is(err, sql.ErrNoRows) {
			return
		}
		if err != nil {
			logger.Debug(ctx, "failed to claim project memory extraction", slog.F("chat_id", chat.ID), slog.Error(err))
			return
		}
		processedTo, ok := p.extractProjectMemoriesOnce(ctx, logger, chat.ID, claim.HistoryVersion)
		if err := p.db.ReleaseChatProjectMemoryExtraction(ctx, database.ReleaseChatProjectMemoryExtractionParams{ChatID: chat.ID, ClaimedUntil: claim.ClaimedUntil.Time}); err != nil {
			logger.Debug(ctx, "failed to release project memory extraction claim", slog.F("chat_id", chat.ID), slog.Error(err))
		}
		if !ok {
			return
		}
		// A turn that completed while the claim was held spawned an
		// extractor that could not claim and exited, so only this check
		// after the release can pick that work up.
		current, err := p.db.GetChatByID(ctx, chat.ID)
		if err != nil || current.HistoryVersion <= processedTo {
			return
		}
	}
}

// extractProjectMemoriesOnce runs one extraction pass from the cursor to the
// chat's history version captured at the start of the pass. It returns that
// version and whether the cursor advanced to it; the caller decides whether
// the chat has moved on since.
func (p *Server) extractProjectMemoriesOnce(ctx context.Context, logger slog.Logger, chatID uuid.UUID, cursor int64) (processedTo int64, ok bool) {
	chat, err := p.db.GetChatByID(ctx, chatID)
	if err != nil || !chat.ProjectID.Valid {
		if err != nil {
			logger.Debug(ctx, "failed to re-read chat for project memory extraction", slog.Error(err))
		}
		return 0, false
	}
	if chat.HistoryVersion <= cursor {
		return 0, false
	}
	advance := func() (int64, bool) {
		if _, err := p.db.UpsertChatProjectMemoryCursor(ctx, database.UpsertChatProjectMemoryCursorParams{ChatID: chat.ID, HistoryVersion: chat.HistoryVersion}); err != nil {
			logger.Debug(ctx, "failed to advance project memory cursor", slog.F("chat_id", chat.ID), slog.Error(err))
			return 0, false
		}
		return chat.HistoryVersion, true
	}

	messages, err := p.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	if err != nil {
		logger.Debug(ctx, "failed to load project memory transcript", slog.F("chat_id", chat.ID), slog.Error(err))
		return 0, false
	}
	// Turns where the main agent curated memory itself are excluded
	// per turn, not for the whole window: running the extractor over them
	// mostly produced split duplicates, but later turns in a lagging window
	// still deserve extraction. The window is closed at the captured
	// history version so a turn that lands mid-pass is not sent twice.
	transcript := renderProjectMemoryTranscript(messages, cursor, chat.HistoryVersion)
	if transcript == "" {
		return advance()
	}
	memories, err := p.db.GetChatProjectMemoriesByProjectID(ctx, chat.ProjectID.UUID)
	if err != nil {
		logger.Debug(ctx, "failed to load project memories for extraction", slog.F("chat_id", chat.ID), slog.Error(err))
		return 0, false
	}
	entries := make([]chattool.ProjectMemoryIndexEntry, len(memories))
	for i, memory := range memories {
		entries[i] = chattool.ProjectMemoryIndexEntry{Name: memory.ChatProjectMemory.Name, Description: memory.ChatProjectMemory.Description}
	}

	apiKeyID, err := p.ensureSyntheticAPIKeyID(ctx, chat.OwnerID)
	if err != nil {
		logger.Debug(ctx, "failed to ensure synthetic API key for project memory extraction", slog.Error(err))
		return 0, false
	}
	resolved, err := p.resolveModelCall(ctx, modelCallSpec{purpose: "project_memory_extraction", chat: chat, buildOptions: modelBuildOptions{ActiveAPIKeyID: apiKeyID}})
	if err != nil {
		logger.Debug(ctx, "failed to resolve model for project memory extraction", slog.Error(err))
		return 0, false
	}
	call := resolved.newObjectCall("project_memory_extraction", "Record new project memories stated by the user in this turn.", projectMemoryExtractionMaxOutputTokens)
	call.Prompt = quickgenPrompt(projectMemoryExtractionPrompt, fmt.Sprintf("Current memory index:\n%s\n\nNew user messages:\n%s", chattool.FormatProjectMemoryIndexForTool(entries), transcript))
	modelCtx, cancelModel := context.WithTimeout(ctx, projectMemoryExtractionModelTimeout)
	defer cancelModel()
	result, err := generateQuickgenObject[projectMemoryExtraction](modelCtx, resolved.model.LanguageModel(), call)
	if err != nil {
		logger.Debug(ctx, "failed to generate project memory extraction", slog.F("chat_id", chat.ID), slog.Error(err))
		return 0, false
	}
	// A rejected proposal is dropped, but a storage failure leaves the
	// cursor in place so the window is retried; inserts that did succeed
	// are skipped on retry by the create-only uniqueness check.
	stored := true
	for _, upsert := range result.Object.Upserts {
		err := applyProjectMemoryUpsert(ctx, p.db, chat, upsert)
		switch {
		case err == nil:
		case errors.Is(err, errInvalidProjectMemoryUpsert), errors.Is(err, chattool.ErrProjectMemoryLimit):
			logger.Debug(ctx, "ignored invalid project memory upsert", slog.F("chat_id", chat.ID), slog.F("name", upsert.Name), slog.Error(err))
		default:
			stored = false
			logger.Debug(ctx, "failed to store extracted project memory", slog.F("chat_id", chat.ID), slog.F("name", upsert.Name), slog.Error(err))
		}
	}
	if !stored {
		return 0, false
	}
	return advance()
}

// applyProjectMemoryUpsert records a memory the extractor proposed. It only
// creates: dogfooding showed the extractor rewriting a correct memory with
// hallucinated content after a question-only turn, so updates to existing
// memories are reserved for the main agent's tool and the UI.
func applyProjectMemoryUpsert(ctx context.Context, store database.Store, chat database.Chat, upsert projectMemoryExtractionUpsert) error {
	normalized, err := normalizeProjectMemoryExtraction(upsert)
	if err != nil {
		return xerrors.Errorf("%w: %v", errInvalidProjectMemoryUpsert, err)
	}
	_, err = chattool.InsertProjectMemory(ctx, store, database.InsertChatProjectMemoryParams{
		ID: uuid.NullUUID{}, ProjectID: chat.ProjectID.UUID, OrganizationID: chat.OrganizationID,
		Name: normalized.Name, Description: normalized.Description, Body: normalized.Body,
		SourceChatID: uuid.NullUUID{UUID: chat.ID, Valid: true}, CreatedBy: chat.OwnerID,
	})
	if database.IsUniqueViolation(err) {
		return nil
	}
	return err
}

type normalizedProjectMemoryExtraction struct {
	Name        string
	Description string
	Body        string
}

func normalizeProjectMemoryExtraction(upsert projectMemoryExtractionUpsert) (normalizedProjectMemoryExtraction, error) {
	name := strings.ToLower(strings.TrimSpace(upsert.Name))
	if err := chattool.ValidateProjectMemoryName(name); err != nil {
		return normalizedProjectMemoryExtraction{}, err
	}
	description := chattool.NormalizeProjectMemoryText(upsert.Description)
	body := chattool.NormalizeProjectMemoryText(upsert.Body)
	if description == "" || len([]rune(description)) > chattool.MaxProjectMemoryDescriptionChars {
		return normalizedProjectMemoryExtraction{}, xerrors.New("invalid memory description")
	}
	if body == "" || len(body) > chattool.MaxProjectMemoryBodyBytes {
		return normalizedProjectMemoryExtraction{}, xerrors.New("invalid memory body")
	}
	return normalizedProjectMemoryExtraction{Name: name, Description: description, Body: body}, nil
}

// projectMemoryTurn is one user turn inside the extraction window: the
// user's visible text and whether the assistant saved or deleted project
// memory successfully in response.
type projectMemoryTurn struct {
	userText  []string
	usedTools bool
}

// projectMemoryTurns splits the messages written after afterHistoryVersion
// and up to upToHistoryVersion into turns. Message revisions hold the
// snapshot version that wrote them, so this window matches the cursor fence
// exactly instead of relying on wall-clock timestamps. A tool call counts only
// when its result was not an error; a failed save must not suppress
// extraction.
func projectMemoryTurns(messages []database.ChatMessage, afterHistoryVersion, upToHistoryVersion int64) []projectMemoryTurn {
	var turns []projectMemoryTurn
	callTurn := make(map[string]int)
	for _, message := range messages {
		if message.Revision <= afterHistoryVersion || message.Revision > upToHistoryVersion {
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
			turns = append(turns, projectMemoryTurn{userText: []string{fmt.Sprintf("[%s]: %s", message.Role, text)}})
		case database.ChatMessageRoleAssistant:
			if len(turns) == 0 {
				continue
			}
			for _, part := range parts {
				if part.Type != codersdk.ChatMessagePartTypeToolCall {
					continue
				}
				switch part.ToolName {
				case chattool.SaveProjectMemoryToolName, chattool.DeleteProjectMemoryToolName:
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

// renderProjectMemoryTranscript renders the user text of every turn in the
// history window in which the main agent did not curate memory itself.
// Assistant text is excluded on purpose: the extractor records what the user
// said, and dogfooding showed it re-recording the assistant's restatement of
// existing memories.
func renderProjectMemoryTranscript(messages []database.ChatMessage, afterHistoryVersion, upToHistoryVersion int64) string {
	var lines []string
	for _, turn := range projectMemoryTurns(messages, afterHistoryVersion, upToHistoryVersion) {
		if turn.usedTools {
			continue
		}
		lines = append(lines, turn.userText...)
	}
	for len(strings.Join(lines, "\n")) > projectMemoryExtractionTranscriptMaxBytes && len(lines) > 1 {
		lines = lines[1:]
	}
	transcript := strings.Join(lines, "\n")
	if len(transcript) <= projectMemoryExtractionTranscriptMaxBytes {
		return transcript
	}

	const truncatedPrefix = "[truncated] "
	tailStart := len(transcript) - (projectMemoryExtractionTranscriptMaxBytes - len(truncatedPrefix))
	for tailStart < len(transcript) && !utf8.RuneStart(transcript[tailStart]) {
		tailStart++
	}
	return truncatedPrefix + transcript[tailStart:]
}
