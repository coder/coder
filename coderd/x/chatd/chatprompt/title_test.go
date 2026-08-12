package chatprompt_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
)

func TestTitleText(t *testing.T) {
	t.Parallel()

	pasteFileID := uuid.New()
	otherPasteFileID := uuid.New()
	syntheticPasteFile := func(id uuid.UUID) codersdk.ChatMessagePart {
		return codersdk.ChatMessageFile(id, "text/plain", "pasted-text-2026-01-02-03-04-05.txt")
	}

	tests := []struct {
		name      string
		parts     []codersdk.ChatMessagePart
		pasteText map[uuid.UUID]string
		want      string
	}{
		{
			name: "joins trimmed text parts",
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("  fix the flaky test  "),
				codersdk.ChatMessageReasoning("skip me"),
				codersdk.ChatMessageText(" in coderd "),
			},
			want: "fix the flaky test in coderd",
		},
		{
			name: "formats file reference with line range and content fence",
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageFileReference("main.go", 3, 7, "fmt.Println(\"hi\")\n"),
			},
			want: "[file-reference] main.go:3-7\n```main.go\nfmt.Println(\"hi\")\n```",
		},
		{
			name: "formats single line file reference without content",
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageFileReference("main.go", 3, 3, "   "),
			},
			want: "[file-reference] main.go:3",
		},
		{
			name: "joins text and file reference parts in order",
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("explain this"),
				codersdk.ChatMessageFileReference("app.ts", 1, 1, ""),
			},
			want: "explain this [file-reference] app.ts:1",
		},
		{
			name: "falls back to paste content for file only messages",
			parts: []codersdk.ChatMessagePart{
				syntheticPasteFile(pasteFileID),
			},
			pasteText: map[uuid.UUID]string{pasteFileID: "  pasted panic log\nsecond line  "},
			want:      "pasted panic log\nsecond line",
		},
		{
			name: "text wins over paste content",
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("typed context"),
				syntheticPasteFile(pasteFileID),
			},
			pasteText: map[uuid.UUID]string{pasteFileID: "pasted content"},
			want:      "typed context",
		},
		{
			name: "joins multiple pastes in part order",
			parts: []codersdk.ChatMessagePart{
				syntheticPasteFile(pasteFileID),
				syntheticPasteFile(otherPasteFileID),
			},
			pasteText: map[uuid.UUID]string{
				pasteFileID:      "first paste",
				otherPasteFileID: "second paste",
			},
			want: "first paste\n\nsecond paste",
		},
		{
			name: "ignores file parts without resolved paste content",
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageFile(uuid.New(), "image/png", "photo.png"),
			},
			pasteText: map[uuid.UUID]string{pasteFileID: "unrelated"},
			want:      "",
		},
		{
			name: "ignores whitespace only paste content",
			parts: []codersdk.ChatMessagePart{
				syntheticPasteFile(pasteFileID),
			},
			pasteText: map[uuid.UUID]string{pasteFileID: " \n\t "},
			want:      "",
		},
		{
			name: "empty parts yield empty text",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, chatprompt.TitleText(tt.parts, tt.pasteText))
		})
	}
}

func TestTitleText_TruncatesPasteContentRuneSafe(t *testing.T) {
	t.Parallel()

	pasteFileID := uuid.New()
	parts := []codersdk.ChatMessagePart{
		codersdk.ChatMessageFile(pasteFileID, "text/plain", "pasted-text-2026-01-02-03-04-05.txt"),
	}
	// Multi-byte runes ensure truncation cannot split a UTF-8 sequence.
	content := strings.Repeat("é", chatprompt.SyntheticPasteTitleBudgetForTest+10)

	got := chatprompt.TitleText(parts, map[uuid.UUID]string{pasteFileID: content})

	require.Len(t, []rune(got), chatprompt.SyntheticPasteTitleBudgetForTest)
	require.True(t, strings.HasPrefix(content, got))
}

func TestTitlePasteText(t *testing.T) {
	t.Parallel()

	t.Run("ShortDataCopiedWhole", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "hello paste", chatprompt.TitlePasteText([]byte("hello paste")))
	})

	t.Run("LongDataBounded", func(t *testing.T) {
		t.Parallel()

		data := bytes.Repeat([]byte("a"), chatprompt.TitlePasteBytePrefix+4096)
		require.Len(t, chatprompt.TitlePasteText(data), chatprompt.TitlePasteBytePrefix)
	})

	t.Run("MatchesFullContentDerivation", func(t *testing.T) {
		t.Parallel()

		pasteFileID := uuid.New()
		parts := []codersdk.ChatMessagePart{
			codersdk.ChatMessageFile(pasteFileID, "text/plain", "pasted-text-2026-01-02-03-04-05.txt"),
		}
		// Three-byte runes make the byte-prefix cut land mid-rune
		// (TitlePasteBytePrefix % 3 != 0); TitleText's rune truncation
		// must still produce the same result as the full content.
		content := strings.Repeat("€", chatprompt.TitlePasteBytePrefix/3+16)
		bounded := chatprompt.TitleText(parts, map[uuid.UUID]string{
			pasteFileID: chatprompt.TitlePasteText([]byte(content)),
		})
		full := chatprompt.TitleText(parts, map[uuid.UUID]string{
			pasteFileID: content,
		})

		require.Equal(t, full, bounded)
		require.Len(t, []rune(bounded), chatprompt.SyntheticPasteTitleBudgetForTest)
	})
}

func TestSyntheticPasteFileIDs(t *testing.T) {
	t.Parallel()

	pasteFileID := uuid.New()
	otherPasteFileID := uuid.New()

	noIDPart := codersdk.ChatMessagePart{
		Type:      codersdk.ChatMessagePartTypeFile,
		MediaType: "text/plain",
		Name:      "pasted-text-2026-01-02-03-04-05.txt",
	}

	tests := []struct {
		name  string
		parts []codersdk.ChatMessagePart
		want  []uuid.UUID
	}{
		{
			name: "collects synthetic paste file ids",
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageText("hello"),
				codersdk.ChatMessageFile(pasteFileID, "text/plain", "pasted-text-2026-01-02-03-04-05.txt"),
				codersdk.ChatMessageFile(otherPasteFileID, "text/plain; charset=utf-8", "pasted-text-2026-12-31-23-59-59.txt"),
			},
			want: []uuid.UUID{pasteFileID, otherPasteFileID},
		},
		{
			name: "skips files without the synthetic name pattern",
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageFile(uuid.New(), "text/plain", "notes.txt"),
			},
			want: nil,
		},
		{
			name: "skips files with non text media types",
			parts: []codersdk.ChatMessagePart{
				codersdk.ChatMessageFile(uuid.New(), "image/png", "pasted-text-2026-01-02-03-04-05.txt"),
			},
			want: nil,
		},
		{
			name:  "skips file parts without a file id",
			parts: []codersdk.ChatMessagePart{noIDPart},
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, chatprompt.SyntheticPasteFileIDs(tt.parts))
		})
	}
}

func TestFallbackTitle(t *testing.T) {
	t.Parallel()

	longWord := strings.Repeat("x", 30)

	tests := []struct {
		name    string
		message string
		want    string
	}{
		{
			name:    "empty message yields default title",
			message: "  \n ",
			want:    "New Chat",
		},
		{
			name:    "short message is kept verbatim",
			message: "fix the flaky test",
			want:    "fix the flaky test",
		},
		{
			name:    "collapses whitespace between words",
			message: "fix\nthe\tflaky   test",
			want:    "fix the flaky test",
		},
		{
			name:    "truncates to six words with ellipsis",
			message: "one two three four five six seven",
			want:    "one two three four five six…",
		},
		{
			name:    "caps six long words at eighty runes keeping the ellipsis",
			message: strings.Repeat(longWord+" ", 7),
			want:    strings.Repeat("x", 30) + " " + strings.Repeat("x", 30) + " " + strings.Repeat("x", 17) + "…",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := chatprompt.FallbackTitle(tt.message)
			require.Equal(t, tt.want, got)
			require.LessOrEqual(t, len([]rune(got)), 80)
		})
	}
}
