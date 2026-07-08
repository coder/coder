package chatd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/coder/coder/v2/coderd/util/ptr"
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
	"Write the title in the same language as the user's message. " +
	"Sentence case. No quotes, emoji, markdown, or trailing punctuation.\n\n" +
	"Examples:\n" +
	"Message: how do I set up SSO with Okta?\n" +
	"Title: Set up SSO with Okta\n\n" +
	"Message: getting `pq: duplicate key value violates unique constraint` when running migrations in coderd\n" +
	"Title: Fix pq duplicate key violation in coderd migrations\n\n" +
	"Message: review PR #123 in acme/webapp and flag risky changes\n" +
	"Title: Review risky changes in acme/webapp PR #123\n\n" +
	"Message: corrige el error de compilación en main.go\n" +
	"Title: Corregir error de compilación en main.go\n\n" +
	"Message: help\n" +
	"Title: Help request"

// quickgenTemperature keeps title and status-label output stable
// across repeated runs over the same input. Fantasy providers drop
// this with a call warning for models that reject it (OpenAI
// reasoning models, Anthropic thinking models).
const quickgenTemperature = 0.0

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
	{fantasybedrock.Name, "global.anthropic.claude-haiku-4-5-20251001-v1:0"},
	{fantasyopenrouter.Name, "anthropic/claude-3.5-haiku"},
	{fantasyvercel.Name, "anthropic/claude-haiku-4.5"},
}

type shortTextCandidate struct {
	provider        string
	model           string
	route           aiGatewayModelRoute
	lm              fantasy.LanguageModel
	providerOptions fantasy.ProviderOptions
}

func selectPreferredConfiguredShortTextModelConfig(
	configs []database.GetEnabledChatModelConfigsRow,
) (database.ChatModelConfig, bool) {
	for _, preferred := range preferredTitleModels {
		for _, config := range configs {
			if chatprovider.NormalizeProvider(config.Provider) != preferred.provider {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(config.ChatModelConfig.Model), preferred.model) {
				continue
			}
			return config.ChatModelConfig, true
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

type generatedTurnStatusLabel struct {
	Label string `json:"label" description:"Compact 2-5 word current chat status label"`
}

// GenerateChatTitleAsync fires a best-effort, automatic title-generation
// pass for a freshly created chat. It is intended to be called from the
// chat-creation endpoint right after the chat and its initial user
// message are persisted.
//
// The work runs in a tracked goroutine with a context detached from the
// request but bound to the server: it neither blocks the HTTP response
// nor is canceled when the request completes, and Close cancels it
// instead of blocking on the title timeout while a provider is
// unreachable. It resolves the chat's model and provider keys, then
// delegates to maybeGenerateChatTitle, which only acts on the first user
// turn (see titleInput) and is otherwise a no-op. Errors are logged and
// swallowed.
func (p *Server) GenerateChatTitleAsync(ctx context.Context, chat database.Chat) {
	logger := p.logger.With(
		slog.F("chat_id", chat.ID),
		slog.F("owner_id", chat.OwnerID),
	)
	// Snapshot the messages synchronously so the first-turn eligibility
	// check (titleInput) is evaluated against creation-time state. Loading
	// inside the goroutine would race the chat worker's first assistant
	// reply and could skip title generation.
	messages, err := p.db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	if err != nil {
		logger.Debug(ctx, "failed to load messages for automatic title generation",
			slog.Error(err),
		)
		return
	}
	if _, ok := titleInput(chat, messages); !ok {
		return
	}
	// Detach from request; bind to server so Close cancels it.
	titleCtx, stopTitleCtx := p.inflightContext(ctx)
	if err := p.goInflight(func() {
		defer stopTitleCtx()
		modelOpts := modelBuildOptionsFromMessages(messages)
		turnCtx := withActiveTurnAPIKeyID(titleCtx, modelOpts)
		model, modelConfig, route, _, _, _, err := p.resolveChatModel(turnCtx, chat, modelOpts)
		if err != nil {
			logger.Debug(turnCtx, "failed to resolve model for automatic title generation",
				slog.Error(err),
			)
			return
		}
		p.maybeGenerateChatTitle(
			turnCtx,
			chat,
			messages,
			string(route.Provider.Type),
			modelConfig,
			model,
			route,
			modelOpts,
			&generatedChatTitle{},
			logger,
			p.existingDebugService(),
		)
	}); err != nil {
		stopTitleCtx()
		logger.Error(context.WithoutCancel(ctx), "failed to schedule automatic chat title generation",
			slog.F("chat_id", chat.ID),
			slog.F("owner_id", chat.OwnerID),
			slog.Error(err),
		)
	}
}

// maybeGenerateChatTitle generates an AI title for the chat when
// appropriate (first user message, no assistant reply yet, and the
// current title is either empty or still the fallback truncation).
// It uses the configured title generation model override when set.
// Otherwise, it tries cheap, fast models first and falls back to the
// user's chat model. It is a best-effort operation that logs and
// swallows errors.
func (p *Server) maybeGenerateChatTitle(
	ctx context.Context,
	chat database.Chat,
	messages []database.ChatMessage,
	fallbackProvider string,
	fallbackConfig database.ChatModelConfig,
	fallbackModel fantasy.LanguageModel,
	fallbackRoute aiGatewayModelRoute,
	modelOpts modelBuildOptions,
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

	overrideConfig, overrideModel, overrideRoute, overrideSet, overrideErr := p.resolveTitleGenerationModelOverride(
		titleCtx,
		chat,
		modelOpts,
	)
	if overrideErr != nil {
		if overrideSet {
			logger.Warn(ctx, "title generation model override unavailable, skipping title generation",
				slog.F("chat_id", chat.ID),
				slog.F("override_context", titleGenerationOverrideContext),
				slog.Error(overrideErr),
			)
			return
		}
		logger.Debug(ctx, "failed to resolve title generation model override",
			slog.F("chat_id", chat.ID),
			slog.F("override_context", titleGenerationOverrideContext),
			slog.Error(overrideErr),
		)
	}

	var candidate shortTextCandidate
	if overrideSet {
		candidate = shortTextCandidate{
			provider:        string(overrideRoute.Provider.Type),
			model:           overrideConfig.Model,
			route:           overrideRoute,
			lm:              overrideModel,
			providerOptions: p.titleGenerationProviderOptions(ctx, overrideModel, overrideConfig),
		}
	} else {
		candidate = shortTextCandidate{
			provider:        fallbackProvider,
			model:           fallbackConfig.Model,
			route:           fallbackRoute,
			lm:              fallbackModel,
			providerOptions: p.titleGenerationProviderOptions(ctx, fallbackModel, fallbackConfig),
		}
	}

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

	candidateCtx := titleCtx
	candidateModel := candidate.lm
	finishDebugRun := func(error) {}
	if debugEnabled {
		candidateCtx, candidateModel, finishDebugRun = p.prepareQuickgenDebugCandidate(
			titleCtx,
			chat,
			debugSvc,
			candidate,
			modelOpts,
			chatdebug.KindTitleGeneration,
			triggerMessageID,
			historyTipMessageID,
			seedSummary,
			logger,
		)
	}

	title, err := generateTitle(candidateCtx, candidateModel, candidate.providerOptions, input)
	finishDebugRun(err)
	if err != nil {
		if overrideSet {
			logger.Warn(ctx, "title model candidate failed",
				slog.F("chat_id", chat.ID),
				slog.F("override_context", titleGenerationOverrideContext),
				slog.F("provider", candidate.provider),
				slog.F("model", candidate.model),
				slog.Error(err),
			)
		} else {
			logger.Debug(ctx, "title model candidate failed",
				slog.F("chat_id", chat.ID),
				slog.Error(err),
			)
		}
		return
	}
	if title == "" || title == chat.Title {
		return
	}

	_, err = p.db.UpdateChatTitleByID(ctx, database.UpdateChatTitleByIDParams{
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
}

func (p *Server) titleGenerationProviderOptions(
	ctx context.Context,
	model fantasy.LanguageModel,
	config database.ChatModelConfig,
) fantasy.ProviderOptions {
	callConfig := codersdk.ChatModelCallConfig{}
	if len(config.Options) > 0 {
		if err := json.Unmarshal(config.Options, &callConfig); err != nil {
			p.logger.Debug(ctx, "failed to parse title generation model call config",
				slog.F("model_config_id", config.ID),
				slog.Error(err),
			)
		}
	}
	providerOptions := chatprovider.ProviderOptionsFromChatModelConfig(model, callConfig.ProviderOptions)
	return chatprovider.ApplyReasoningEffort(
		model,
		providerOptions,
		chatprovider.ResolveReasoningEffort(nil, callConfig.ReasoningEffort),
	)
}

func (p *Server) newQuickgenDebugModel(
	ctx context.Context,
	chat database.Chat,
	debugSvc *chatdebug.Service,
	provider string,
	model string,
	route aiGatewayModelRoute,
	modelOpts modelBuildOptions,
) (fantasy.LanguageModel, error) {
	debugOpts := modelOpts
	debugOpts.RecordHTTP = true
	debugModel, err := p.newModel(ctx, modelClientRequest{
		Chat:         chat,
		ModelName:    model,
		UserAgent:    chatprovider.UserAgent(),
		ExtraHeaders: chatprovider.CoderHeaders(chat),
	}, route, debugOpts)
	if err != nil {
		return nil, err
	}

	return chatdebug.WrapModel(debugModel, debugSvc, chatdebug.RecorderOptions{
		ChatID:   chat.ID,
		OwnerID:  chat.OwnerID,
		Provider: provider,
		Model:    model,
	}), nil
}

func (p *Server) prepareQuickgenDebugCandidate(
	ctx context.Context,
	chat database.Chat,
	debugSvc *chatdebug.Service,
	candidate shortTextCandidate,
	modelOpts modelBuildOptions,
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

	debugModel, err := p.newQuickgenDebugModel(
		ctx,
		chat,
		debugSvc,
		candidate.provider,
		candidate.model,
		candidate.route,
		modelOpts,
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

	runContext := chatdebugRunContext(run)
	runCtx := chatdebug.ContextWithRun(ctx, &runContext)
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
	providerOptions fantasy.ProviderOptions,
	input string,
) (string, error) {
	title, err := generateStructuredTitle(ctx, model, providerOptions, titleGenerationPrompt, input)
	if err != nil {
		return "", err
	}
	return title, nil
}

func generateStructuredTitle(
	ctx context.Context,
	model fantasy.LanguageModel,
	providerOptions fantasy.ProviderOptions,
	systemPrompt string,
	userInput string,
) (string, error) {
	title, _, err := generateStructuredTitleWithUsage(
		ctx,
		model,
		providerOptions,
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
	providerOptions fantasy.ProviderOptions,
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
			Temperature:       ptr.Ref(quickgenTemperature),
			ProviderOptions:   providerOptions,
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
	write("- Write the title in the same language as the user's messages.\n")
	write("- Sentence case. No quotes, emoji, markdown, or trailing punctuation.\n")
	return prompt.String()
}

func generateManualTitle(
	ctx context.Context,
	messages []database.ChatMessage,
	fallbackModel fantasy.LanguageModel,
	providerOptions fantasy.ProviderOptions,
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
		providerOptions,
		systemPrompt,
		userInput,
	)
	if err != nil {
		return "", usage, err
	}

	return title, usage, nil
}

const turnStatusLabelPrompt = "You write compact chat status labels for a sidebar or push notification. " +
	"Given a chat title, current chat state, and the agent's latest message, populate the label field with a 2-5 word status label. " +
	"Describe the chat's current state, not the agent. " +
	"Good examples: Finished unit tests, Submitted PR, Still working on API, Waiting for user input. " +
	"Do not start with Agent, I, We, It, The agent, or The chat. " +
	"Avoid phrases like Agent asked, Agent identified, Agent found, or Agent explained. " +
	"Prefer short action or state phrases such as Finished, Submitted, Fixed, Testing, Still working, or Waiting for. " +
	"No quotes, emoji, markdown, or trailing punctuation."

// generateTurnStatusLabel produces a short turn status label using the
// caller-supplied fallback model. Returns "" on any failure.
func (p *Server) generateTurnStatusLabel(
	ctx context.Context,
	chat database.Chat,
	status database.ChatStatus,
	assistantText string,
	fallbackProvider string,
	fallbackModelName string,
	fallbackModel fantasy.LanguageModel,
	fallbackRoute aiGatewayModelRoute,
	modelOpts modelBuildOptions,
	logger slog.Logger,
	debugSvc *chatdebug.Service,
	triggerMessageID int64,
	historyTipMessageID int64,
) string {
	debugEnabled := debugSvc != nil && debugSvc.IsEnabled(ctx, chat.ID, chat.OwnerID)

	labelCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	assistantText = truncateRunes(assistantText, maxConversationContextRunes)
	input := "Current chat state: " + turnStatusLabelStateContext(status) +
		"\nChat title: " + chat.Title +
		"\n\nAgent's latest message:\n" + assistantText

	candidate := shortTextCandidate{
		provider: fallbackProvider,
		model:    fallbackModelName,
		route:    fallbackRoute,
		lm:       fallbackModel,
	}

	statusSeedSummary := chatdebug.SeedSummary("Turn status label")

	candidateCtx := labelCtx
	candidateModel := candidate.lm
	finishDebugRun := func(error) {}
	if debugEnabled {
		candidateCtx, candidateModel, finishDebugRun = p.prepareQuickgenDebugCandidate(
			labelCtx,
			chat,
			debugSvc,
			candidate,
			modelOpts,
			chatdebug.KindQuickgen,
			triggerMessageID,
			historyTipMessageID,
			statusSeedSummary,
			logger,
		)
	}

	generatedLabel, err := generateStructuredTurnStatusLabel(
		candidateCtx,
		candidateModel,
		turnStatusLabelPrompt,
		input,
	)
	finishDebugRun(err)
	if err != nil {
		logger.Debug(ctx, "turn status label model candidate failed",
			slog.Error(err),
		)
		return ""
	}
	return generatedLabel
}

func generateStructuredTurnStatusLabel(
	ctx context.Context,
	model fantasy.LanguageModel,
	systemPrompt string,
	userInput string,
) (string, error) {
	userInput = strings.TrimSpace(userInput)
	if userInput == "" {
		return "", xerrors.New("turn status label input was empty")
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

	var maxOutputTokens int64 = 64
	var result *fantasy.ObjectResult[generatedTurnStatusLabel]
	err := chatretry.Retry(ctx, func(retryCtx context.Context) error {
		var genErr error
		result, genErr = object.Generate[generatedTurnStatusLabel](retryCtx, model, fantasy.ObjectCall{
			Prompt:            prompt,
			SchemaName:        "propose_turn_status_label",
			SchemaDescription: "Propose a compact chat status label.",
			MaxOutputTokens:   &maxOutputTokens,
			Temperature:       ptr.Ref(quickgenTemperature),
		})
		return genErr
	}, nil)
	if err != nil {
		return "", xerrors.Errorf("generate structured turn status label: %w", err)
	}

	label, ok := normalizeTurnStatusLabel(result.Object.Label)
	if !ok {
		return "", xerrors.New("generated turn status label was invalid")
	}
	return label, nil
}

func turnStatusLabelStateContext(status database.ChatStatus) string {
	switch status {
	case database.ChatStatusWaiting:
		return "The turn finished and the chat is idle."
	case database.ChatStatusPending:
		return "Another user message is queued and the chat will continue."
	case database.ChatStatusRequiresAction:
		return "The chat is waiting for user input or action."
	case database.ChatStatusError:
		return "The chat ended with an error."
	default:
		return "The chat state is unknown."
	}
}

func fallbackTurnStatusLabel(status database.ChatStatus) string {
	switch status {
	case database.ChatStatusWaiting:
		return "Finished latest turn"
	case database.ChatStatusPending:
		return "Still working on request"
	case database.ChatStatusRequiresAction:
		return "Waiting for user input"
	case database.ChatStatusError:
		return "Hit an error"
	default:
		return "Updated chat status"
	}
}

func normalizeTurnStatusLabel(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}

	text = strings.Trim(text, "\"'`")
	text = strings.TrimSpace(text)
	if text == "" || strings.ContainsAny(text, "\r\n") {
		return "", false
	}
	text = strings.TrimRight(text, ".!?")
	text = strings.Join(strings.Fields(text), " ")
	if text == "" || hasSentenceBoundary(text) {
		return "", false
	}

	words := strings.Fields(text)
	if len(words) < 2 || len(words) > 5 {
		return "", false
	}

	lower := strings.ToLower(text)
	if hasDisallowedTurnStatusLabelSubject(lower) {
		return "", false
	}

	disallowedPhrases := []string{
		"agent asked",
		"agent identified",
		"agent found",
		"agent explained",
	}
	for _, phrase := range disallowedPhrases {
		if strings.Contains(lower, phrase) {
			return "", false
		}
	}

	return text, true
}

func hasDisallowedTurnStatusLabelSubject(text string) bool {
	subject := leadingLetters(text)
	switch subject {
	case "agent", "i", "it", "the", "we":
		return true
	default:
		return false
	}
}

func leadingLetters(text string) string {
	for i, r := range text {
		if r < 'a' || r > 'z' {
			return text[:i]
		}
	}
	return text
}

func hasSentenceBoundary(text string) bool {
	for i, r := range text {
		switch r {
		case '.', '!', '?':
			if i+1 < len(text) && text[i+1] == ' ' {
				return true
			}
		}
	}
	return false
}
