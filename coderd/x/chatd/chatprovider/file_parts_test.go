package chatprovider_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func TestRouteMedia(t *testing.T) {
	t.Parallel()

	const anthropicCap = codersdk.AnthropicInlineImageCapBytes
	forceResponses := true
	forceCompletions := false

	for _, tc := range []struct {
		name      string
		transport string
		// provider is the configured provider type. Empty means a direct
		// client, so the route falls back to the transport.
		provider string
		modelID  string
		override *bool
		accepts  []string
		rejects  []string
		imageCap int
	}{
		{
			name: "Anthropic", transport: "anthropic", provider: "anthropic",
			accepts:  []string{"image/jpeg", "image/png", "image/gif", "image/webp", "application/pdf", "text/markdown", "text/plain; charset=utf-8"},
			rejects:  []string{"image/heic", "image/bmp", "image/svg+xml", "audio/mpeg", "application/json"},
			imageCap: anthropicCap,
		},
		{
			name: "BedrockClaude", transport: "anthropic", provider: "bedrock",
			accepts:  []string{"image/webp", "application/pdf", "text/csv"},
			rejects:  []string{"image/heic", "audio/mpeg", "application/json"},
			imageCap: anthropicCap,
		},
		{
			name: "BedrockMantleResponses", transport: "openai", provider: "bedrock", modelID: "openai.gpt-oss-120b", override: &forceResponses,
			accepts: []string{"image/gif", "application/pdf"},
			rejects: []string{"image/heic", "text/plain", "audio/mpeg"},
		},
		{
			name: "BedrockMantleChatCompletions", transport: "openai", provider: "bedrock", modelID: "openai.gpt-oss-120b", override: &forceCompletions,
			accepts: []string{"image/png", "application/pdf", "audio/mpeg"},
			rejects: []string{"image/heic", "text/plain", "application/json"},
		},
		{
			name: "OpenAIResponses", transport: "openai", provider: "openai", modelID: "gpt-4o",
			accepts: []string{"image/jpeg", "image/png", "image/gif", "image/webp", "application/pdf"},
			rejects: []string{"image/heic", "image/svg+xml", "text/plain", "audio/mpeg", "application/json"},
		},
		{
			name: "OpenAIChatCompletions", transport: "openai", provider: "openai", modelID: "babbage-002",
			accepts: []string{"image/png", "application/pdf", "audio/wav"},
			rejects: []string{"image/heic", "text/plain", "application/json"},
		},
		{
			name: "OpenAIForcedResponses", transport: "openai", provider: "openai", modelID: "babbage-002", override: &forceResponses,
			accepts: []string{"image/png"},
			rejects: []string{"audio/mpeg"},
		},
		{
			name: "OpenAIForcedChatCompletions", transport: "openai", provider: "openai", modelID: "gpt-4o", override: &forceCompletions,
			accepts: []string{"audio/mpeg"},
			rejects: []string{"text/plain"},
		},
		{
			name: "Azure", transport: "openai-compat", provider: "azure",
			accepts: []string{"image/jpeg", "image/gif", "audio/mpeg"},
			rejects: []string{"image/heic", "image/bmp", "image/svg+xml", "application/pdf", "text/plain", "application/json"},
		},
		{
			name: "Google", transport: "openai-compat", provider: "google",
			accepts: []string{"image/jpeg", "image/png", "image/webp", "image/heic", "image/heif", "audio/mpeg"},
			rejects: []string{"image/gif", "image/svg+xml", "application/pdf", "text/plain", "application/json"},
		},
		{
			name: "Vercel", transport: "openai-compat", provider: "vercel",
			accepts: []string{"image/bmp", "image/heic", "application/pdf", "audio/mpeg"},
			rejects: []string{"image/svg+xml", "text/plain", "application/json"},
		},
		{
			name: "OpenRouter", transport: "openai-compat", provider: "openrouter",
			accepts: []string{"image/bmp", "image/gif", "application/pdf"},
			rejects: []string{"image/svg+xml", "text/plain", "application/json"},
		},
		{
			name: "OpenAICompat", transport: "openai-compat", provider: "openai-compat",
			accepts: []string{"image/bmp", "application/pdf", "audio/mpeg"},
			rejects: []string{"image/svg+xml", "text/plain", "application/json"},
		},
		{
			name: "DirectAnthropicClient", transport: "anthropic",
			accepts:  []string{"image/png", "application/pdf"},
			rejects:  []string{"audio/mpeg"},
			imageCap: anthropicCap,
		},
		{
			name: "MixedCaseNamesAndParameters", transport: "Anthropic", provider: " BEDROCK ",
			accepts:  []string{"IMAGE/PNG; charset=binary", "Application/PDF"},
			imageCap: anthropicCap,
		},
		{
			// chatd never builds the fantasy Google client.
			name: "UnbuiltTransport", transport: "google", provider: "google",
			rejects: []string{"image/png", "application/pdf", "text/plain"},
		},
		{
			name: "UnknownConfiguredProvider", transport: "openai-compat", provider: "made-up",
			rejects: []string{"image/png", "application/pdf", "text/plain"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var openAIConfig *codersdk.ChatModelOpenAIConfig
			if tc.override != nil {
				openAIConfig = &codersdk.ChatModelOpenAIConfig{UseResponsesAPI: tc.override}
			}
			model := chatprovider.NewModel(
				&chattest.FakeModel{ProviderName: tc.transport, ModelName: tc.modelID},
				openAIConfig,
			).WithConfiguredProvider(tc.provider)

			for _, mediaType := range tc.accepts {
				require.True(t, model.AcceptsFilePartMediaType(mediaType), "accepts %q", mediaType)
			}
			for _, mediaType := range tc.rejects {
				require.False(t, model.AcceptsFilePartMediaType(mediaType), "rejects %q", mediaType)
			}
			imageCap, hasImageCap := model.InlineImageCapBytes()
			require.Equal(t, tc.imageCap, imageCap)
			require.Equal(t, tc.imageCap > 0, hasImageCap)
		})
	}
}
