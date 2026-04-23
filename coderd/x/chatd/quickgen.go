package chatd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"charm.land/fantasy"
	"charm.land/fantasy/object"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyazure "charm.land/fantasy/providers/azure"
	fantasybedrock "charm.land/fantasy/providers/bedrock"
	fantasygoogle "charm.land/fantasy/providers/google"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenrouter "charm.land/fantasy/providers/openrouter"
	fantasyvercel "charm.land/fantasy/providers/vercel"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
	"github.com/coder/coder/v2/codersdk"
)

const titleGenerationPrompt = "Write a short title for the user's message. " +
	"Populate the title field with the result. " +
	"Return only the title text in 2-8 words. " +
	"Do not answer the user or describe the title-writing task. " +
	"Preserve specific identifiers such as PR numbers, repo names, file paths, function names, and error messages. " +
	"If the message is short or vague, stay close to the user's wording instead of inventing context. " +
	"Sentence case. No quotes, emoji, markdown, or trailing punctuation."

const (
	// maxConversationContextRunes caps the conversation sample in manual
	// title prompts to avoid exceeding model context windows.
	maxConversationContextRunes = 6000
	// maxLatestUserMessageRunes caps the latest user message excerpt.
	maxLatestUserMessageRunes = 1000
	// recentTurnWindow is the number of most recent turns included
	// alongside the first user turn in manual title context.
	recentTurnWindow = 3
)

// preferredTitleModels are lightweight models used for title
// generation, one per provider type. Each entry uses the
// cheapest/fastest small model for that provider as identified
// by the charmbracelet/catwalk model catalog. Providers that
// aren't configured (no API key) are silently skipped.
var preferredTitleModels = []struct {
	provider string
	model    string
}{
	{fantasyanthropic.Name, "claude-haiku-4-5"},
	{fantasyopenai.Name, "gpt-4o-mini"},
	{fantasygoogle.Name, "gemini-2.5-flash"},
	{fantasyazure.Name, "gpt-4o-mini"},
	{fantasybedrock.Name, "anthropic.claude-haiku-4-5-20251001-v1:0"},
	{fantasyopenrouter.Name, "anthropic/claude-3.5-haiku"},
	{fantasyvercel.Name, "anthropic/claude-haiku-4.5"},
}

type shortTextCandidate struct {
	provider string
	model    string
	lm       fantasy.LanguageModel
}

func selectPreferredConfiguredShortTextModelConfig(
	configs []database.ChatModelConfig,
) (database.ChatModelConfig, bool) {
	for _, preferred := range preferredTitleModels {
		for _, config := range configs {
			if chatprovider.NormalizeProvider(config.Provider) != preferred.provider {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(config.Model), preferred.model) {
				continue
			}
			return config, true
		}
	}
	return database.ChatModelConfig{}, false
}

func normalizeShortTextOutput(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	text = strings.Trim(text, "\"'`")
	return strings.Join(strings.Fields(text), " ")
}

type generatedTitle struct {
	Title string `json:"title" description:"Short descriptive chat title"`
}

// maybeGenerateChatTitle generates an AI title for the chat when
// appropriate (first user message, no assistant reply yet, and the
// current title is either empty or still the fallback truncation).
// It tries cheap, fast models first and falls back to the user's
// chat model. It is a best-effort operation that logs and swallows
// errors.
func (p *Server) maybeGenerateChatTitle(
	ctx context.Context,
	chat database.Chat,
	messages []database.ChatMessage,
	fallbackProvider string,
	fallbackModelName string,
	fallbackModel fantasy.LanguageModel,
	keys chatprovider.ProviderAPIKeys,
	generatedTitle *generatedChatTitle,
	logger slog.Logger,
	debugSvc *chatdebug.Service,
) {
	input, ok := titleInput(chat, messages)
	if !ok {
		return
	}
	debugEnabled := debugSvc != nil && debugSvc.IsEnabled(ctx, chat.ID, chat.OwnerID)

	titleCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Build candidate list: preferred lightweight models first,
	// then the user's chat model as last resort.
	candidates := make([]shortTextCandidate, 0, len(preferredTitleModels)+1)
	for _, c := range preferredTitleModels {
		m, err := chatprovider.ModelFromConfig(
			c.provider, c.model, keys, chatprovider.UserAgent(),
			chatprovider.CoderHeaders(chat),
			nil,
		)
		if err == nil {
			candidates = append(candidates, shortTextCandidate{
				provider: c.provider,
				model:    c.model,
				lm:       m,
			})
		}
	}
	candidates = append(candidates, shortTextCandidate{
		provider: fallbackProvider,
		model:    fallbackModelName,
		lm:       fallbackModel,
	})

	var historyTipMessageID int64
	if len(messages) > 0 {
		historyTipMessageID = messages[len(messages)-1].ID
	}

	var triggerMessageID int64
	for _, message := range messages {
		if message.Visibility == database.ChatMessageVisibilityModel {
			continue
		}
		if message.Role == database.ChatMessageRoleUser {
			triggerMessageID = message.ID
			break
		}
	}

	seedSummary := chatdebug.SeedSummary(
		chatdebug.TruncateLabel(input, chatdebug.MaxLabelLength),
	)

	var lastErr error
	for _, candidate := range candidates {
		candidateCtx := titleCtx
		candidateModel := candidate.lm
		finishDebugRun := func(error) {}
		if debugEnabled {
			candidateCtx, candidateModel, finishDebugRun = prepareQuickgenDebugCandidate(
				titleCtx,
				chat,
				keys,
				debugSvc,
				candidate,
				chatdebug.KindTitleGeneration,
				triggerMessageID,
				historyTipMessageID,
				seedSummary,
				logger,
			)
		}

		title, err := generateTitle(candidateCtx, candidateModel, input)
		finishDebugRun(err)
		if err != nil {
			lastErr = err
			logger.Debug(ctx, "title model candidate failed",
				slog.F("chat_id", chat.ID),
				slog.Error(err),
			)
			continue
		}
		if title == "" || title == chat.Title {
			return
		}

		_, err = p.db.UpdateChatByID(ctx, database.UpdateChatByIDParams{
			ID:    chat.ID,
			Title: title,
		})
		if err != nil {
			logger.Warn(ctx, "failed to update generated chat title",
				slog.F("chat_id", chat.ID),
				slog.Error(err),
			)
			return
		}
		chat.Title = title
		generatedTitle.Store(title)
		p.publishChatPubsubEvent(chat, codersdk.ChatWatchEventKindTitleChange, nil)
		return
	}

	if lastErr != nil {
		logger.Debug(ctx, "all title model candidates failed",
			slog.F("chat_id", chat.ID),
			slog.Error(lastErr),
		)
	}
}

func newQuickgenDebugModel(
	chat database.Chat,
	keys chatprovider.ProviderAPIKeys,
	debugSvc *chatdebug.Service,
	provider string,
	model string,
) (fantasy.LanguageModel, error) {
	httpClient := &http.Client{Transport: &chatdebug.RecordingTransport{}}
	debugModel, err := chatprovider.ModelFromConfig(
		provider,
		model,
		keys,
		chatprovider.UserAgent(),
		chatprovider.CoderHeaders(chat),
		httpClient,
	)
	if err != nil {
		return nil, err
	}
	if debugModel == nil {
		return nil, xerrors.Errorf(
			"create model for %s/%s returned nil",
			provider,
			model,
		)
	}

	return chatdebug.WrapModel(debugModel, debugSvc, chatdebug.RecorderOptions{
		ChatID:   chat.ID,
		OwnerID:  chat.OwnerID,
		Provider: provider,
		Model:    model,
	}), nil
}

func prepareQuickgenDebugCandidate(
	ctx context.Context,
	chat database.Chat,
	keys chatprovider.ProviderAPIKeys,
	debugSvc *chatdebug.Service,
	candidate shortTextCandidate,
	kind chatdebug.RunKind,
	triggerMessageID int64,
	historyTipMessageID int64,
	seedSummary map[string]any,
	logger slog.Logger,
) (context.Context, fantasy.LanguageModel, func(error)) {
	finishDebugRun := func(error) {}
	if debugSvc == nil {
		return ctx, candidate.lm, finishDebugRun
	}

	debugModel, err := newQuickgenDebugModel(
		chat,
		keys,
		debugSvc,
		candidate.provider,
		candidate.model,
	)
	if err != nil {
		logger.Warn(ctx, "failed to build short-text debug model",
			slog.F("chat_id", chat.ID),
			slog.F("run_kind", kind),
			slog.F("provider", candidate.provider),
			slog.F("model", candidate.model),
			slog.Error(err),
		)
		return ctx, candidate.lm, finishDebugRun
	}

	// Debug instrumentation must not eat into the quickgen budget
	// (30s titleCtx / summaryCtx on the caller). Detach and bound
	// the insert so a slow DB can't delay title generation or push
	// summaries, matching prepareManualTitleDebugRun,
	// prepareChatTurnDebugRun, and startCompactionDebugRun.
	createRunCtx, createRunCancel := context.WithTimeout(
		context.WithoutCancel(ctx), debugCreateRunTimeout,
	)
	run, err := debugSvc.CreateRun(createRunCtx, chatdebug.CreateRunParams{
		ChatID:              chat.ID,
		TriggerMessageID:    triggerMessageID,
		HistoryTipMessageID: historyTipMessageID,
		Kind:                kind,
		Status:              chatdebug.StatusInProgress,
		Provider:            candidate.provider,
		Model:               candidate.model,
		Summary:             seedSummary,
	})
	createRunCancel()
	if err != nil {
		logger.Warn(ctx, "failed to create short-text debug run",
			slog.F("chat_id", chat.ID),
			slog.F("run_kind", kind),
			slog.F("provider", candidate.provider),
			slog.F("model", candidate.model),
			slog.Error(err),
		)
		return ctx, candidate.lm, finishDebugRun
	}

	runCtx := chatdebug.ContextWithRun(
		ctx,
		&chatdebug.RunContext{
			RunID:               run.ID,
			ChatID:              chat.ID,
			TriggerMessageID:    triggerMessageID,
			HistoryTipMessageID: historyTipMessageID,
			Kind:                kind,
			Provider:            candidate.provider,
			Model:               candidate.model,
		},
	)
	finishDebugRun = func(runErr error) {
		if finalizeErr := debugSvc.FinalizeRun(ctx, chatdebug.FinalizeRunParams{
			RunID:       run.ID,
			ChatID:      chat.ID,
			Status:      chatdebug.ClassifyError(runErr),
			SeedSummary: seedSummary,
			Timeout:     10 * time.Second,
		}); finalizeErr != nil {
			logger.Warn(ctx, "failed to finalize short-text debug run",
				slog.F("chat_id", chat.ID),
				slog.F("run_kind", kind),
				slog.F("run_id", run.ID),
				slog.Error(finalizeErr),
			)
		}
	}
	return runCtx, debugModel, finishDebugRun
}

// generateTitle calls the model with a title-generation system prompt
// and returns the normalized result. It retries transient LLM errors
// (rate limits, overloaded, etc.) with exponential backoff.
func generateTitle(
	ctx context.Context,
	model fantasy.LanguageModel,
	input string,
) (string, error) {
	title, err := generateStructuredTitle(ctx, model, titleGenerationPrompt, input)
	if err != nil {
		return "", err
	}
	return title, nil
}

func generateStructuredTitle(
	ctx context.Context,
	model fantasy.LanguageModel,
	systemPrompt string,
	userInput string,
) (string, error) {
	title, _, err := generateStructuredTitleWithUsage(
		ctx,
		model,
		systemPrompt,
		userInput,
	)
	if err != nil {
		return "", err
	}
	return title, nil
}

func generateStructuredTitleWithUsage(
	ctx context.Context,
	model fantasy.LanguageModel,
	systemPrompt string,
	userInput string,
) (string, fantasy.Usage, error) {
	userInput = strings.TrimSpace(userInput)
	if userInput == "" {
		return "", fantasy.Usage{}, xerrors.New("title input was empty")
	}

	prompt := fantasy.Prompt{
		{
			Role: fantasy.MessageRoleSystem,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: systemPrompt},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: userInput},
			},
		},
	}

	var maxOutputTokens int64 = 256
	var result *fantasy.ObjectResult[generatedTitle]
	err := chatretry.Retry(ctx, func(retryCtx context.Context) error {
		var genErr error
		result, genErr = object.Generate[generatedTitle](retryCtx, model, fantasy.ObjectCall{
			Prompt:            prompt,
			SchemaName:        "propose_title",
			SchemaDescription: "Propose a short chat title.",
			MaxOutputTokens:   &maxOutputTokens,
		})
		return genErr
	}, nil)
	if err != nil {
		var usage fantasy.Usage
		var noObjErr *fantasy.NoObjectGeneratedError
		if errors.As(err, &noObjErr) {
			usage = noObjErr.Usage
		}
		return "", usage, xerrors.Errorf("generate structured title: %w", err)
	}

	title := normalizeTitleOutput(result.Object.Title)
	if err := validateGeneratedTitle(title); err != nil {
		return "", result.Usage, err
	}
	return title, result.Usage, nil
}

func validateGeneratedTitle(title string) error {
	if title == "" {
		return xerrors.New("generated title was empty")
	}
	if len(strings.Fields(title)) > 8 {
		return xerrors.New("generated title exceeded 8 words")
	}
	return nil
}

// titleInput returns the first user message text and whether title
// generation should proceed. It returns false when the chat already
// has assistant/tool replies, has more than one visible user message,
// or the current title doesn't look like a candidate for replacement.
func titleInput(
	chat database.Chat,
	messages []database.ChatMessage,
) (string, bool) {
	userCount := 0
	firstUserText := ""

	for _, message := range messages {
		if message.Visibility == database.ChatMessageVisibilityModel {
			continue
		}

		switch message.Role {
		case database.ChatMessageRoleAssistant, database.ChatMessageRoleTool:
			return "", false
		case database.ChatMessageRoleUser:
			userCount++
			if firstUserText == "" {
				parsed, err := chatprompt.ParseContent(message)
				if err != nil {
					return "", false
				}
				firstUserText = strings.TrimSpace(
					contentBlocksToText(parsed),
				)
			}
		}
	}

	if userCount != 1 || firstUserText == "" {
		return "", false
	}

	currentTitle := strings.TrimSpace(chat.Title)
	if currentTitle == "" {
		return firstUserText, true
	}

	if currentTitle != fallbackChatTitle(firstUserText) {
		return "", false
	}

	return firstUserText, true
}

func normalizeTitleOutput(title string) string {
	title = normalizeShortTextOutput(title)
	if title == "" {
		return ""
	}
	return truncateRunes(title, 80)
}

func fallbackChatTitle(message string) string {
	const maxWords = 6
	const maxRunes = 80

	words := strings.Fields(message)
	if len(words) == 0 {
		return "New Chat"
	}

	truncated := false
	if len(words) > maxWords {
		words = words[:maxWords]
		truncated = true
	}

	title := strings.Join(words, " ")
	if truncated {
		return truncateRunes(title, maxRunes-1) + "…"
	}

	return truncateRunes(title, maxRunes)
}

// contentBlocksToText concatenates the text parts of SDK chat
// message parts into a single space-separated string.
func contentBlocksToText(parts []codersdk.ChatMessagePart) string {
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type != codersdk.ChatMessagePartTypeText {
			continue
		}
		text := strings.TrimSpace(part.Text)
		if text == "" {
			continue
		}
		texts = append(texts, text)
	}
	return strings.Join(texts, " ")
}

func truncateRunes(value string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	return string(runes[:maxLen])
}

// Manual title regeneration is user-initiated and can use richer
// conversation context than the automatic first-message title path
// above. These helpers keep the manual prompt-building logic private
// while reusing the shared title-generation utilities in this file.
type manualTitleTurn struct {
	role string
	text string
}

func extractManualTitleTurns(messages []database.ChatMessage) []manualTitleTurn {
	turns := make([]manualTitleTurn, 0, len(messages))
	for _, message := range messages {
		if message.Visibility == database.ChatMessageVisibilityModel {
			continue
		}

		role := ""
		switch message.Role {
		case database.ChatMessageRoleUser:
			role = string(database.ChatMessageRoleUser)
		case database.ChatMessageRoleAssistant:
			role = string(database.ChatMessageRoleAssistant)
		default:
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

		turns = append(turns, manualTitleTurn{
			role: role,
			text: text,
		})
	}

	return turns
}

func selectManualTitleTurnIndexes(turns []manualTitleTurn) []int {
	firstUserIndex := slices.IndexFunc(turns, func(turn manualTitleTurn) bool {
		return turn.role == string(database.ChatMessageRoleUser)
	})
	if firstUserIndex == -1 {
		return nil
	}

	windowStart := max(0, len(turns)-recentTurnWindow)
	selected := make([]int, 0, recentTurnWindow+1)
	if firstUserIndex < windowStart {
		selected = append(selected, firstUserIndex)
	}
	for i := windowStart; i < len(turns); i++ {
		selected = append(selected, i)
	}

	return selected
}

func buildManualTitleContext(
	turns []manualTitleTurn,
	selected []int,
) (conversationBlock string, latestUserMsg string) {
	userCount := 0
	for _, turn := range turns {
		if turn.role != string(database.ChatMessageRoleUser) {
			continue
		}
		userCount++
		latestUserMsg = turn.text
	}

	latestUserMsg = truncateRunes(latestUserMsg, maxLatestUserMessageRunes)
	if userCount <= 1 || len(selected) == 0 {
		return "", latestUserMsg
	}

	lines := make([]string, 0, len(selected)+1)
	for i, idx := range selected {
		if i == 1 {
			if gap := idx - selected[i-1] - 1; gap > 0 {
				lines = append(lines, fmt.Sprintf("[... %d earlier turns omitted ...]", gap))
			}
		}
		lines = append(lines, fmt.Sprintf("[%s]: %s", turns[idx].role, turns[idx].text))
	}

	conversationBlock = strings.Join(lines, "\n")
	conversationBlock = truncateRunes(conversationBlock, maxConversationContextRunes)
	return conversationBlock, latestUserMsg
}

func renderManualTitlePrompt(
	conversationBlock string,
	firstUserText string,
	latestUserMsg string,
) string {
	var prompt strings.Builder
	write := func(value string) {
		_, _ = prompt.WriteString(value)
	}

	write("Write a short title for this AI coding conversation.\n")
	write("Populate the title field with the result.\n\n")
	write("Primary user objective:\n<primary_objective>\n")
	write(firstUserText)
	write("\n</primary_objective>")

	if conversationBlock != "" {
		write("\n\nConversation sample:\n<conversation_sample>\n")
		write(conversationBlock)
		write("\n</conversation_sample>")
	}

	if strings.TrimSpace(latestUserMsg) != strings.TrimSpace(truncateRunes(firstUserText, maxLatestUserMessageRunes)) {
		write("\n\nThe user's most recent message:\n<latest_message>\n")
		write(latestUserMsg)
		write("\n</latest_message>\n")
		write("Note: Weight the overall conversation arc more heavily than just the latest message.")
	}

	write("\n\nRequirements:\n")
	write("- Return only the title text in 2-8 words.\n")
	write("- Populate the title field only.\n")
	write("- Do not answer the user or describe the title-writing task.\n")
	write("- Preserve specific identifiers (PR numbers, repo names, file paths, function names, error messages).\n")
	write("- If the conversation is short or vague, stay close to the user's wording.\n")
	write("- Sentence case. No quotes, emoji, markdown, or trailing punctuation.\n")
	return prompt.String()
}

func generateManualTitle(
	ctx context.Context,
	messages []database.ChatMessage,
	fallbackModel fantasy.LanguageModel,
) (string, fantasy.Usage, error) {
	turns := extractManualTitleTurns(messages)
	selected := selectManualTitleTurnIndexes(turns)

	firstUserIndex := slices.IndexFunc(turns, func(turn manualTitleTurn) bool {
		return turn.role == string(database.ChatMessageRoleUser)
	})
	if firstUserIndex == -1 {
		return "", fantasy.Usage{}, nil
	}
	firstUserText := truncateRunes(turns[firstUserIndex].text, maxLatestUserMessageRunes)

	conversationBlock, latestUserMsg := buildManualTitleContext(turns, selected)
	systemPrompt := renderManualTitlePrompt(
		conversationBlock,
		firstUserText,
		latestUserMsg,
	)

	titleCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	userInput := strings.TrimSpace(latestUserMsg)
	if userInput == "" {
		userInput = strings.TrimSpace(firstUserText)
	}

	title, usage, err := generateStructuredTitleWithUsage(
		titleCtx,
		fallbackModel,
		systemPrompt,
		userInput,
	)
	if err != nil {
		return "", usage, err
	}

	return title, usage, nil
}

const pushSummaryPrompt = "You are a notification assistant. Given a chat title " +
	"and the agent's last message, write a single short sentence (under 100 characters) " +
	"summarizing what the agent did. This will be shown as a push notification body. " +
	"Return plain text only — no quotes, no emoji, no markdown."

// generatePushSummary calls a cheap model to produce a short push
// notification body from the chat title and the last assistant
// message text. It follows the same candidate-selection strategy
// as title generation: try preferred lightweight models first, then
// fall back to the provided model. Returns "" on any failure.
func generatePushSummary(
	ctx context.Context,
	chat database.Chat,
	assistantText string,
	fallbackProvider string,
	fallbackModelName string,
	fallbackModel fantasy.LanguageModel,
	keys chatprovider.ProviderAPIKeys,
	logger slog.Logger,
	debugSvc *chatdebug.Service,
	triggerMessageID int64,
	historyTipMessageID int64,
) string {
	debugEnabled := debugSvc != nil && debugSvc.IsEnabled(ctx, chat.ID, chat.OwnerID)

	summaryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	assistantText = truncateRunes(assistantText, maxConversationContextRunes)
	input := "Chat title: " + chat.Title + "\n\nAgent's last message:\n" + assistantText

	candidates := make([]shortTextCandidate, 0, len(preferredTitleModels)+1)
	for _, c := range preferredTitleModels {
		m, err := chatprovider.ModelFromConfig(
			c.provider, c.model, keys, chatprovider.UserAgent(),
			chatprovider.CoderHeaders(chat),
			nil,
		)
		if err == nil {
			candidates = append(candidates, shortTextCandidate{
				provider: c.provider,
				model:    c.model,
				lm:       m,
			})
		}
	}
	candidates = append(candidates, shortTextCandidate{
		provider: fallbackProvider,
		model:    fallbackModelName,
		lm:       fallbackModel,
	})

	pushSeedSummary := chatdebug.SeedSummary("Push summary")

	for _, candidate := range candidates {
		candidateCtx := summaryCtx
		candidateModel := candidate.lm
		finishDebugRun := func(error) {}
		if debugEnabled {
			candidateCtx, candidateModel, finishDebugRun = prepareQuickgenDebugCandidate(
				summaryCtx,
				chat,
				keys,
				debugSvc,
				candidate,
				chatdebug.KindQuickgen,
				triggerMessageID,
				historyTipMessageID,
				pushSeedSummary,
				logger,
			)
		}

		summary, err := generateShortText(
			candidateCtx,
			candidateModel,
			pushSummaryPrompt,
			input,
		)
		finishDebugRun(err)
		if err != nil {
			logger.Debug(ctx, "push summary model candidate failed",
				slog.Error(err),
			)
			continue
		}
		if summary != "" {
			return summary
		}
	}
	return ""
}

// generateShortText calls a model with a system prompt and user
// input, returning a cleaned-up short text response. It reuses the
// same retry logic as title generation. Retries can therefore
// produce multiple debug steps for a single quickgen run.
func generateShortText(
	ctx context.Context,
	model fantasy.LanguageModel,
	systemPrompt string,
	userInput string,
) (string, error) {
	prompt := []fantasy.Message{
		{
			Role: fantasy.MessageRoleSystem,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: systemPrompt},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: userInput},
			},
		},
	}

	var maxOutputTokens int64 = 256

	var response *fantasy.Response
	err := chatretry.Retry(ctx, func(retryCtx context.Context) error {
		var genErr error
		response, genErr = model.Generate(retryCtx, fantasy.Call{
			Prompt:          prompt,
			MaxOutputTokens: &maxOutputTokens,
		})
		return genErr
	}, nil)
	if err != nil {
		return "", xerrors.Errorf("generate short text: %w", err)
	}

	responseParts := make([]codersdk.ChatMessagePart, 0, len(response.Content))
	for _, block := range response.Content {
		if p := chatprompt.PartFromContent(block); p.Type != "" {
			responseParts = append(responseParts, p)
		}
	}
	text := normalizeShortTextOutput(contentBlocksToText(responseParts))
	return text, nil
}
