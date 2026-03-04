package chatd

import (
	"context"
	"strings"
	"time"

	"charm.land/fantasy"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/chatd/chatretry"
	"github.com/coder/coder/v2/coderd/database"
	coderdpubsub "github.com/coder/coder/v2/coderd/pubsub"
)

const titleGenerationPrompt = "Generate a concise title (2-8 words) for the user's message. " +
	"Use verb-noun format describing the primary intent (e.g. \"Fix sidebar layout\", " +
	"\"Add user authentication\", \"Refactor database queries\"). " +
	"Return plain text only — no quotes, no emoji, no markdown, no code fences, " +
	"no special characters, no trailing punctuation. Sentence case."

// TitleModelFunc returns candidate language models for title
// generation. Models are returned in preference order — cheap, fast
// models first, with the user's chat model as a last resort.
type TitleModelFunc func() []fantasy.LanguageModel

// preferredTitleModels are lightweight models used for title
// generation, one per provider type. Each entry uses the
// cheapest/fastest small model for that provider as identified
// by the charmbracelet/catwalk model catalog. Providers that
// aren't configured (no API key) are silently skipped.
var preferredTitleModels = []struct {
	provider string
	model    string
}{
	{"anthropic", "claude-haiku-4-5"},
	{"openai", "gpt-4o-mini"},
	{"google", "gemini-2.5-flash"},
	{"azure", "gpt-4o-mini"},
	{"bedrock", "anthropic.claude-haiku-4-5-20251001-v1:0"},
	{"openrouter", "anthropic/claude-3.5-haiku"},
	{"vercel", "anthropic/claude-haiku-4.5"},
}

// titleModelCandidates returns an ordered list of models to try for
// title generation. It resolves provider keys, attempts to create
// each preferred lightweight model, and appends fallback as the
// last resort.
func (p *Server) titleModelCandidates(
	ctx context.Context,
	fallback fantasy.LanguageModel,
) []fantasy.LanguageModel {
	providers, err := p.db.GetEnabledChatProviders(ctx)
	if err != nil {
		return []fantasy.LanguageModel{fallback}
	}
	dbProviders := make(
		[]chatprovider.ConfiguredProvider, 0, len(providers),
	)
	for _, provider := range providers {
		dbProviders = append(dbProviders, chatprovider.ConfiguredProvider{
			Provider: provider.Provider,
			APIKey:   provider.APIKey,
			BaseURL:  provider.BaseUrl,
		})
	}
	keys := chatprovider.MergeProviderAPIKeys(
		p.providerAPIKeys, dbProviders,
	)

	candidates := make([]fantasy.LanguageModel, 0, len(preferredTitleModels)+1)
	for _, c := range preferredTitleModels {
		m, err := chatprovider.ModelFromConfig(
			c.provider, c.model, keys,
		)
		if err == nil {
			candidates = append(candidates, m)
		}
	}
	// Always fall back to the user's chat model.
	candidates = append(candidates, fallback)
	return candidates
}

// maybeGenerateChatTitle generates an AI title for the chat when
// appropriate (first user message, no assistant reply yet, and the
// current title is either empty or still the fallback truncation).
// It is a best-effort operation that logs and swallows errors.
func (p *Server) maybeGenerateChatTitle(
	ctx context.Context,
	chat database.Chat,
	messages []database.ChatMessage,
	titleModels TitleModelFunc,
	logger slog.Logger,
) {
	input, ok := titleInput(chat, messages)
	if !ok {
		return
	}

	titleCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	candidates := titleModels()
	var lastErr error
	for _, model := range candidates {
		title, err := generateTitle(titleCtx, model, input)
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
		p.publishChatPubsubEvent(chat, coderdpubsub.ChatEventKindTitleChange)
		return
	}

	if lastErr != nil {
		logger.Debug(ctx, "all title model candidates failed",
			slog.F("chat_id", chat.ID),
			slog.Error(lastErr),
		)
	}
}

// generateTitle calls the model with a title-generation system prompt
// and returns the normalized result. It retries transient LLM errors
// (rate limits, overloaded, etc.) with exponential backoff.
func generateTitle(
	ctx context.Context,
	model fantasy.LanguageModel,
	input string,
) (string, error) {
	prompt := []fantasy.Message{
		{
			Role: fantasy.MessageRoleSystem,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: titleGenerationPrompt},
			},
		},
		{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: input},
			},
		},
	}

	var response *fantasy.Response
	err := chatretry.Retry(ctx, func(retryCtx context.Context) error {
		var genErr error
		response, genErr = model.Generate(retryCtx, fantasy.Call{
			Prompt:          prompt,
			MaxOutputTokens: int64Ptr(256),
		})
		return genErr
	}, nil)
	if err != nil {
		return "", xerrors.Errorf("generate title text: %w", err)
	}

	title := normalizeTitleOutput(contentBlocksToText(response.Content))
	if title == "" {
		return "", xerrors.New("generated title was empty")
	}
	return title, nil
}

func int64Ptr(v int64) *int64 { return &v }

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
		case string(fantasy.MessageRoleAssistant), string(fantasy.MessageRoleTool):
			return "", false
		case string(fantasy.MessageRoleUser):
			userCount++
			if firstUserText == "" {
				parsed, err := chatprompt.ParseContent(
					string(fantasy.MessageRoleUser), message.Content,
				)
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
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}

	title = strings.Trim(title, "\"'`")
	title = strings.Join(strings.Fields(title), " ")
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
		title += "…"
	}

	return truncateRunes(title, maxRunes)
}

// contentBlocksToText concatenates the text parts of content blocks
// into a single space-separated string.
func contentBlocksToText(content []fantasy.Content) string {
	parts := make([]string, 0, len(content))
	for _, block := range content {
		textBlock, ok := fantasy.AsContentType[fantasy.TextContent](block)
		if !ok {
			continue
		}
		text := strings.TrimSpace(textBlock.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " ")
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
