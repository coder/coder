package chatd

import (
	"context"
	"encoding/base64"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
)

func TestReplaceUnsupportedToolMedia(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slogtest.Make(t, nil)
	modelOn := func(transport string) chatprovider.Model {
		return chatprovider.NewModel(&chattest.FakeModel{ProviderName: transport, ModelName: "m"}, nil)
	}
	mediaPart := func(mediaType string, payload []byte) fantasy.ToolResultPart {
		return fantasy.ToolResultPart{
			ToolCallID: "call-1",
			Output: fantasy.ToolResultOutputContentMedia{
				Data:      base64.StdEncoding.EncodeToString(payload),
				MediaType: mediaType,
				Text:      "Ran Playwright code",
			},
		}
	}
	prompt := func(part fantasy.ToolResultPart) []fantasy.Message {
		return []fantasy.Message{
			{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "screenshot please"}}},
			{Role: fantasy.MessageRoleTool, Content: []fantasy.MessagePart{part}},
		}
	}
	textOutput := func(t *testing.T, messages []fantasy.Message) string {
		t.Helper()
		result, ok := messages[1].Content[0].(fantasy.ToolResultPart)
		require.True(t, ok)
		text, ok := result.Output.(fantasy.ToolResultOutputContentText)
		require.True(t, ok, "expected text output, got %T", result.Output)
		return text.Text
	}

	t.Run("AudioBecomesTextOnAnthropic", func(t *testing.T) {
		t.Parallel()
		in := prompt(mediaPart("audio/mpeg", []byte{1, 2, 3}))
		out := replaceUnsupportedToolMedia(ctx, logger, in, modelOn("anthropic"), "anthropic")
		text := textOutput(t, out)
		require.Contains(t, text, "Ran Playwright code\n")
		require.Contains(t, text, "[audio/mpeg content omitted")
		_, stillMedia := in[1].Content[0].(fantasy.ToolResultPart).Output.(fantasy.ToolResultOutputContentMedia)
		require.True(t, stillMedia, "input must not be mutated")
	})

	// Bedrock Claude is routed through the Anthropic transport; the note
	// still names the configured provider.
	t.Run("OversizedImageBecomesTextOnBedrockClaude", func(t *testing.T) {
		t.Parallel()
		in := prompt(mediaPart("image/png", make([]byte, 5*1024*1024)))
		out := replaceUnsupportedToolMedia(ctx, logger, in, modelOn("anthropic"), "bedrock")
		require.Contains(t, textOutput(t, out), "[image omitted: 5242880 bytes exceeds the AWS Bedrock inline image limit")
	})

	// Bedrock non-Anthropic models are routed through the OpenAI transport.
	t.Run("OversizedImageStaysMediaOnBedrockOpenAI", func(t *testing.T) {
		t.Parallel()
		in := prompt(mediaPart("image/png", make([]byte, 5*1024*1024)))
		out := replaceUnsupportedToolMedia(ctx, logger, in, modelOn("openai"), "bedrock")
		require.Equal(t, in, out)
	})

	t.Run("SmallImageStaysMediaOnAnthropic", func(t *testing.T) {
		t.Parallel()
		in := prompt(mediaPart("image/png", []byte{1, 2, 3}))
		out := replaceUnsupportedToolMedia(ctx, logger, in, modelOn("anthropic"), "anthropic")
		require.Equal(t, in, out)
	})

	t.Run("AudioStaysMediaOnOpenAI", func(t *testing.T) {
		t.Parallel()
		in := prompt(mediaPart("audio/mpeg", []byte{1, 2, 3}))
		out := replaceUnsupportedToolMedia(ctx, logger, in, modelOn("openai"), "openai")
		require.Equal(t, in, out)
	})
}

func TestBase64DecodedLen(t *testing.T) {
	t.Parallel()
	for _, n := range []int{0, 1, 2, 3, 4, 5, 6, 100} {
		data := base64.StdEncoding.EncodeToString(make([]byte, n))
		require.Equal(t, n, base64DecodedLen(data), "encoded %d bytes", n)
	}
}
