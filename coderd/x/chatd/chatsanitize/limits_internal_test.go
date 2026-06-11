package chatsanitize

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasybedrock "charm.land/fantasy/providers/bedrock"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/codersdk"
)

func TestValidatePromptLimits(t *testing.T) {
	t.Parallel()

	t.Run("Rejects invalid Anthropic PDF bytes", func(t *testing.T) {
		t.Parallel()

		err := ValidatePromptLimits(
			fantasyanthropic.Name, 200_000,
			promptWithPDF(pdfPart("bad.pdf", []byte("not a pdf"))),
		)
		classified := requireRejected(t, err)
		require.Equal(t, fantasyanthropic.Name, classified.Provider)
		require.Contains(t, classified.Message, "bad.pdf")
		require.Contains(t, classified.Message, "not a valid PDF")
		require.Contains(t, err.Error(), "reason=invalid_pdf")
	})

	t.Run("Gates on canonical provider names", func(t *testing.T) {
		t.Parallel()

		badPDF := promptWithPDF(pdfPart("bad.pdf", []byte("not a pdf")))

		err := ValidatePromptLimits(fantasybedrock.Name, 200_000, badPDF)
		require.Equal(t, fantasybedrock.Name, requireRejected(t, err).Provider)

		// Uncapped and non-canonical provider names fail open, like
		// ApplyAnthropicProviderToolGuard.
		require.NoError(t, ValidatePromptLimits(fantasyopenai.Name, 0, badPDF))
		require.NoError(t, ValidatePromptLimits(" ANTHROPIC ", 200_000, badPDF))
	})

	t.Run("Uses larger page cap for larger context models", func(t *testing.T) {
		t.Parallel()

		prompt := promptWithPDF(pdfPart("long.pdf", validPDFWithPages(101)))
		err := ValidatePromptLimits(fantasyanthropic.Name, 200_000, prompt)
		require.Error(t, err)
		require.Contains(t, err.Error(), "page_cap=100")

		err = ValidatePromptLimits(fantasyanthropic.Name, 200_001, prompt)
		require.NoError(t, err)
	})
}

func TestPDFHelpers(t *testing.T) {
	t.Parallel()

	require.True(t, isPDF([]byte("%PDF-1.7\n")))
	require.False(t, isPDF([]byte("%PDF")))

	count, ok := approxPDFPageCount([]byte("%PDF-1.7\n<< /Type /Page >>\n<< /Type\t/Page >>"))
	require.True(t, ok)
	require.Equal(t, 2, count)

	// /Pages container objects are not page markers.
	_, ok = approxPDFPageCount([]byte("%PDF-1.7\n<< /Type /Pages /Count 2 >>"))
	require.False(t, ok)
}

func TestValidatePromptLimitsWithCaps(t *testing.T) {
	t.Parallel()

	t.Run("Counts repeated PDF parts and aggregate pages", func(t *testing.T) {
		t.Parallel()

		part := pdfPart("repeat.pdf", validPDFWithPages(1))
		err := validateWithCaps(
			promptWithPDF(part, part),
			limits{requestPayloadBytes: 1 << 20, pageCap: 1},
		)
		classified := requireRejected(t, err)
		require.Contains(t, classified.Message, "about 2 pages")
		require.Contains(t, err.Error(), "total_pages=2")
	})

	t.Run("Rejects aggregate encoded payload over cap", func(t *testing.T) {
		t.Parallel()

		data := validPDFWithPages(1)
		err := validateWithCaps(
			promptWithPDF(pdfPart("payload.pdf", data)),
			limits{
				requestPayloadBytes: base64.StdEncoding.EncodedLen(len(data)) - 1,
				pageCap:             100,
			},
		)
		classified := requireRejected(t, err)
		require.Contains(t, classified.Message, "too large")
		require.Contains(t, err.Error(), "reason=payload_cap")
	})

	t.Run("Counts every part type toward the request estimate", func(t *testing.T) {
		t.Parallel()

		// Every part type must contribute to the lower-bound estimate. Each
		// part alone fits under the injected cap; two together exceed it.
		caps := limits{requestPayloadBytes: 40, pageCap: 100}
		parts := []fantasy.MessagePart{
			fantasy.TextPart{Text: strings.Repeat("x", 30)},
			fantasy.ToolCallPart{ToolName: "grep", Input: strings.Repeat("y", 30)},
			fantasy.ToolResultPart{Output: fantasy.ToolResultOutputContentText{Text: strings.Repeat("z", 30)}},
			fantasy.ToolResultPart{Output: fantasy.ToolResultOutputContentError{Error: xerrors.New(strings.Repeat("e", 30))}},
			fantasy.FilePart{Filename: "notes.txt", Data: []byte(strings.Repeat("t", 30)), MediaType: "text/plain"},
		}
		for _, part := range parts {
			require.NoError(t, validateWithCaps(promptWithParts(part), caps))
			err := validateWithCaps(promptWithParts(part, part), caps)
			requireRejected(t, err)
			require.Contains(t, err.Error(), "reason=payload_cap")
		}
	})

	t.Run("Estimates file parts by provider serialization", func(t *testing.T) {
		t.Parallel()

		// fantasy's anthropic provider base64-encodes images and PDFs, sends
		// text files at raw size, and drops other file media types; counting
		// everything at base64 size would falsely reject text-heavy requests.
		data := []byte(strings.Repeat("t", 30))
		caps := limits{requestPayloadBytes: len(data), pageCap: 100}

		// Raw text size sits exactly at the cap; base64 counting (40 bytes)
		// would reject it.
		require.NoError(t, validateWithCaps(promptWithParts(
			fantasy.FilePart{Filename: "notes.txt", Data: data, MediaType: "text/plain"},
		), caps))

		// One extra byte of text trips the cap, so text bytes do count.
		err := validateWithCaps(promptWithParts(
			fantasy.FilePart{Filename: "notes.txt", Data: append(data, 't'), MediaType: "text/plain"},
		), caps)
		requireRejected(t, err)
		require.Contains(t, err.Error(), "reason=payload_cap")

		// Images are base64-encoded: 30 raw bytes encode to 40, over the cap.
		err = validateWithCaps(promptWithParts(
			fantasy.FilePart{Filename: "pic.png", Data: data, MediaType: "image/png"},
		), caps)
		requireRejected(t, err)

		// Unsupported file media types never reach the provider request.
		require.NoError(t, validateWithCaps(promptWithParts(
			fantasy.FilePart{Filename: "data.json", Data: []byte(strings.Repeat("j", 100)), MediaType: "application/json"},
		), caps))
	})

	t.Run("Rejects oversized requests without PDFs", func(t *testing.T) {
		t.Parallel()

		err := validateWithCaps(promptWithParts(
			fantasy.TextPart{Text: strings.Repeat("x", 11)},
		), limits{requestPayloadBytes: 10, pageCap: 100})
		requireRejected(t, err)
		require.Contains(t, err.Error(), "reason=payload_cap")
	})

	t.Run("Tolerates typed-nil parts", func(t *testing.T) {
		t.Parallel()

		// fantasy.AsMessagePart and fantasy.AsToolResultOutputType both
		// dereference typed-nil pointers, so the walker must use the
		// package's nil-safe casts for every part and output type.
		err := validateWithCaps(promptWithParts(
			(*fantasy.FilePart)(nil),
			(*fantasy.TextPart)(nil),
			(*fantasy.ReasoningPart)(nil),
			(*fantasy.ToolCallPart)(nil),
			(*fantasy.ToolResultPart)(nil),
			fantasy.ToolResultPart{Output: (*fantasy.ToolResultOutputContentText)(nil)},
		), limits{requestPayloadBytes: 1 << 20, pageCap: 100})
		require.NoError(t, err)
	})

	t.Run("Allows PDFs with unknown page count", func(t *testing.T) {
		t.Parallel()

		err := validateWithCaps(
			promptWithPDF(pdfPart("unknown.pdf", []byte("%PDF-1.7\nxref\n0 0"))),
			limits{requestPayloadBytes: 1 << 20, pageCap: 1},
		)
		require.NoError(t, err)
	})
}

// validateWithCaps exercises the walker with injected caps so aggregation
// behavior can be tested independently of provider cap derivation.
func validateWithCaps(prompt []fantasy.Message, caps limits) error {
	if caps.displayName == "" {
		caps.displayName = "Anthropic"
	}
	return (&validator{provider: fantasyanthropic.Name, caps: caps}).validate(prompt)
}

func requireRejected(t *testing.T, err error) chaterror.ClassifiedError {
	t.Helper()

	require.Error(t, err)
	classified := chaterror.Classify(err)
	require.Equal(t, codersdk.ChatErrorKindConfig, classified.Kind)
	require.False(t, classified.Retryable)
	return classified
}

func promptWithPDF(parts ...fantasy.FilePart) []fantasy.Message {
	content := make([]fantasy.MessagePart, 0, len(parts))
	for _, part := range parts {
		content = append(content, part)
	}
	return promptWithParts(content...)
}

func promptWithParts(parts ...fantasy.MessagePart) []fantasy.Message {
	return []fantasy.Message{{Role: fantasy.MessageRoleUser, Content: parts}}
}

func pdfPart(name string, data []byte) fantasy.FilePart {
	return fantasy.FilePart{Filename: name, Data: data, MediaType: "application/pdf"}
}

func validPDFWithPages(pages int) []byte {
	var b strings.Builder
	_, _ = b.WriteString("%PDF-1.7\n")
	for i := range pages {
		_, _ = fmt.Fprintf(&b, "%d 0 obj << /Type /Page >> endobj\n", i+1)
	}
	return []byte(b.String())
}
