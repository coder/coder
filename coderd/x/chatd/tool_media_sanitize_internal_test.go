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

	for _, tc := range []struct {
		name      string
		transport string
		mediaType string
		size      int
		// wantNote is empty when the media part must pass through unchanged.
		wantNote string
	}{
		{name: "AnthropicAudio", transport: "anthropic", mediaType: "audio/mpeg", size: 3, wantNote: "[audio/mpeg content omitted"},
		{name: "AnthropicOversizedImage", transport: "anthropic", mediaType: "image/png", size: 5 * 1024 * 1024, wantNote: "[image omitted: 5242880 bytes exceeds the inline image limit"},
		{name: "AnthropicSmallImage", transport: "anthropic", mediaType: "image/png", size: 3},
		{name: "OpenAIOversizedImage", transport: "openai", mediaType: "image/png", size: 5 * 1024 * 1024},
		{name: "OpenAIAudio", transport: "openai", mediaType: "audio/mpeg", size: 3},
		{name: "OpenAISVG", transport: "openai", mediaType: "image/svg+xml", size: 6, wantNote: "[image/svg+xml content omitted"},
		{name: "GoogleBMP", transport: "google", mediaType: "image/bmp", size: 6, wantNote: "[image/bmp content omitted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			prompt := func() []fantasy.Message {
				return []fantasy.Message{
					{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "screenshot please"}}},
					{Role: fantasy.MessageRoleTool, Content: []fantasy.MessagePart{fantasy.ToolResultPart{
						ToolCallID: "call-1",
						Output: fantasy.ToolResultOutputContentMedia{
							Data:      base64.StdEncoding.EncodeToString(make([]byte, tc.size)),
							MediaType: tc.mediaType,
							Text:      "Ran Playwright code",
						},
					}}},
				}
			}
			in := prompt()
			model := chatprovider.NewModel(&chattest.FakeModel{ProviderName: tc.transport, ModelName: "m"}, nil)
			out := replaceUnsupportedToolMedia(context.Background(), slogtest.Make(t, nil), in, model, tc.transport)

			require.Equal(t, prompt(), in, "input must not be mutated")
			if tc.wantNote == "" {
				require.Equal(t, in, out)
				return
			}
			require.Equal(t, in[0], out[0])
			result, ok := out[1].Content[0].(fantasy.ToolResultPart)
			require.True(t, ok)
			text, ok := result.Output.(fantasy.ToolResultOutputContentText)
			require.True(t, ok, "expected text output, got %T", result.Output)
			require.Contains(t, text.Text, "Ran Playwright code\n"+tc.wantNote)
		})
	}
}

func TestBase64DecodedLen(t *testing.T) {
	t.Parallel()
	for _, n := range []int{0, 1, 2, 3, 4, 5, 6, 100} {
		data := base64.StdEncoding.EncodeToString(make([]byte, n))
		require.Equal(t, n, base64DecodedLen(data), "encoded %d bytes", n)
	}
}
