package chatd

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func TestReplaceUnsupportedMedia(t *testing.T) {
	t.Parallel()

	anthropic := chatprovider.NewModel(&chattest.FakeModel{ProviderName: "anthropic", ModelName: "m"}, nil)
	openai := chatprovider.NewModel(&chattest.FakeModel{ProviderName: "openai", ModelName: "m"}, nil)
	google := chatprovider.NewModel(&chattest.FakeModel{ProviderName: "openai-compat", ModelName: "m"}, nil).
		WithConfiguredProvider("google")

	imageCap := codersdk.AnthropicInlineImageCapBytes
	capNote := fmt.Sprintf("[image omitted: %d bytes exceeds the inline image limit of %d bytes]", imageCap, imageCap)
	userFile := func(name, mediaType string, size int) fantasy.MessagePart {
		return fantasy.FilePart{Filename: name, MediaType: mediaType, Data: make([]byte, size)}
	}
	toolMedia := func(text, mediaType string, size int) fantasy.MessagePart {
		return fantasy.ToolResultPart{ToolCallID: "call-1", Output: fantasy.ToolResultOutputContentMedia{
			Data:      base64.StdEncoding.EncodeToString(make([]byte, size)),
			MediaType: mediaType,
			Text:      text,
		}}
	}
	toolText := func(text string) fantasy.MessagePart {
		return fantasy.ToolResultPart{ToolCallID: "call-1", Output: fantasy.ToolResultOutputContentText{Text: text}}
	}

	for _, tc := range []struct {
		name  string
		model chatprovider.Model
		in    fantasy.MessagePart
		// want is nil when the part must pass through unchanged.
		want fantasy.MessagePart
	}{
		{name: "UserPDFAtImageCap", model: anthropic, in: userFile("report.pdf", "application/pdf", imageCap)},
		{name: "UserPNGAtImageCap", model: anthropic, in: userFile("shot.png", "image/png", imageCap), want: fantasy.TextPart{Text: capNote}},
		{name: "UserPNGWithoutImageCap", model: openai, in: userFile("shot.png", "image/png", imageCap)},
		{
			name: "UserGIFOnGoogle", model: google, in: userFile("anim.gif", "image/gif", 3),
			want: fantasy.TextPart{Text: `[Attachment "anim.gif" (image/gif) omitted: not supported by this model]`},
		},
		{
			name: "UnnamedUserGIFOnGoogle", model: google, in: userFile("", "image/gif", 3),
			want: fantasy.TextPart{Text: "[Attachment (image/gif) omitted: not supported by this model]"},
		},
		{name: "ToolPDFAtImageCap", model: anthropic, in: toolMedia("Ran tool", "application/pdf", imageCap)},
		{name: "ToolPNGAtImageCap", model: anthropic, in: toolMedia("Ran tool", "image/png", imageCap), want: toolText(capNote + "\n\nRan tool")},
		{name: "ToolPNGUnderImageCap", model: anthropic, in: toolMedia("Ran tool", "image/png", imageCap-1)},
		{
			name: "ToolAudioOnAnthropic", model: anthropic, in: toolMedia("Ran tool", "audio/mpeg", 3),
			want: toolText("The tool returned audio/mpeg content, which cannot be displayed.\n\nRan tool"),
		},
		{
			name: "ToolGIFWithoutTextOnGoogle", model: google, in: toolMedia("", "image/gif", 3),
			want: toolText("The tool returned image/gif content, which cannot be displayed."),
		},
		{
			name: "ToolMediaWithoutTypeOnOpenAI", model: openai, in: toolMedia("", "", 3),
			want: toolText("The tool returned file content, which cannot be displayed."),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			role := fantasy.MessageRoleUser
			if _, ok := tc.in.(fantasy.ToolResultPart); ok {
				role = fantasy.MessageRoleTool
			}
			in := []fantasy.Message{{Role: role, Content: []fantasy.MessagePart{tc.in}}}
			out := replaceUnsupportedMedia(context.Background(), slogtest.Make(t, nil), in, tc.model)

			require.Equal(t, tc.in, in[0].Content[0], "input must not be mutated")
			want := tc.want
			if want == nil {
				want = tc.in
			}
			require.Equal(t, []fantasy.Message{{Role: role, Content: []fantasy.MessagePart{want}}}, out)
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
