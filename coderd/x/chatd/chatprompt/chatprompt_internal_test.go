package chatprompt

import (
	"encoding/json"
	"testing"
	"unicode/utf8"

	"charm.land/fantasy"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk"
)

func TestToolResultAntivenom(t *testing.T) {
	t.Parallel()
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})

	t.Run("PoisonedTextResultSanitized", func(t *testing.T) {
		t.Parallel()

		// Simulate raw binary bytes stored as json.RawMessage.
		// This reproduces the crash where tool output containing
		// invalid UTF-8 was passed verbatim to the LLM provider.
		poisonedBytes := json.RawMessage(string([]byte{0xFF, 0xD8, 0xFF}))
		part := codersdk.ChatMessagePart{
			Type:       codersdk.ChatMessagePartTypeToolResult,
			ToolCallID: "call-1",
			ToolName:   "test_tool",
			Result:     poisonedBytes,
			IsError:    false,
			IsMedia:    false,
		}

		result := toolResultPartToMessagePart(logger, part)

		textOutput, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](result.Output)
		require.True(t, ok, "expected text output, got %T", result.Output)
		require.True(t, utf8.ValidString(textOutput.Text), "output text must be valid UTF-8")
		require.NotEmpty(t, textOutput.Text)
	})

	t.Run("PoisonedMediaResultDegradesToText", func(t *testing.T) {
		t.Parallel()

		// Simulate raw JPEG bytes stored where base64 is expected.
		// The base64 validation guard should reject this and fall
		// through to the text path.
		corruptedData := string([]byte{0xFF, 0xD8, 0xFF, 0xE0})
		media := struct {
			Data     string `json:"data"`
			MimeType string `json:"mime_type"`
			Text     string `json:"text,omitempty"`
		}{
			Data:     corruptedData,
			MimeType: "image/jpeg",
		}
		mediaJSON, err := json.Marshal(media)
		require.NoError(t, err)

		part := codersdk.ChatMessagePart{
			Type:       codersdk.ChatMessagePartTypeToolResult,
			ToolCallID: "call-2",
			ToolName:   "computer",
			Result:     json.RawMessage(mediaJSON),
			IsError:    false,
			IsMedia:    true,
		}

		result := toolResultPartToMessagePart(logger, part)

		// Should degrade to text since the data is not valid base64.
		_, isMedia := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](result.Output)
		require.False(t, isMedia, "corrupted media should not be returned as media")

		textOutput, isText := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](result.Output)
		require.True(t, isText, "should fall through to text, got %T", result.Output)
		require.True(t, utf8.ValidString(textOutput.Text), "fallback text must be valid UTF-8")
	})

	t.Run("MediaWithInvalidUTF8TextSanitized", func(t *testing.T) {
		t.Parallel()

		// Valid base64 data with an invalid UTF-8 text annotation.
		// The media should survive but the text field must be
		// sanitized to valid UTF-8.
		validBase64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVQI12NgAAIABQAB"
		invalidText := "hello" + string([]byte{0xFF, 0xFE}) + "world"
		media := struct {
			Data     string `json:"data"`
			MimeType string `json:"mime_type"`
			Text     string `json:"text,omitempty"`
		}{
			Data:     validBase64,
			MimeType: "image/png",
			Text:     invalidText,
		}
		mediaJSON, err := json.Marshal(media)
		require.NoError(t, err)

		part := codersdk.ChatMessagePart{
			Type:       codersdk.ChatMessagePartTypeToolResult,
			ToolCallID: "call-4",
			ToolName:   "computer",
			Result:     json.RawMessage(mediaJSON),
			IsError:    false,
			IsMedia:    true,
		}

		result := toolResultPartToMessagePart(logger, part)

		mediaOutput, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](result.Output)
		require.True(t, ok, "media with valid base64 should stay as media, got %T", result.Output)
		require.Equal(t, validBase64, mediaOutput.Data)
		require.True(t, utf8.ValidString(mediaOutput.Text), "text must be sanitized to valid UTF-8")
		require.Contains(t, mediaOutput.Text, "hello")
		require.Contains(t, mediaOutput.Text, "world")
	})

	t.Run("PoisonedErrorResultSanitized", func(t *testing.T) {
		t.Parallel()
		// Simulate invalid UTF-8 in an error tool result.
		poisonedError := json.RawMessage(`{"error":"fail` + string([]byte{0xFF, 0xFE}) + `ed"}`)
		part := codersdk.ChatMessagePart{
			Type:       codersdk.ChatMessagePartTypeToolResult,
			ToolCallID: "call-5",
			ToolName:   "broken_tool",
			Result:     poisonedError,
			IsError:    true,
			IsMedia:    false,
		}

		result := toolResultPartToMessagePart(logger, part)

		errOutput, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentError](result.Output)
		require.True(t, ok, "expected error output, got %T", result.Output)
		require.True(t, utf8.ValidString(errOutput.Error.Error()),
			"error message must be valid UTF-8")
		require.Contains(t, errOutput.Error.Error(), "fail")
		require.Contains(t, errOutput.Error.Error(), "ed")
	})
}
