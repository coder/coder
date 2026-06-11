package chatpdf

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

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

func TestValidatePDFPrompt(t *testing.T) {
	t.Parallel()

	t.Run("Rejects invalid Anthropic PDF bytes", func(t *testing.T) {
		t.Parallel()

		err := ValidatePrompt(
			" ANTHROPIC ", 200_000,
			promptWithPDF(pdfPart("bad.pdf", []byte("not a pdf"))),
		)
		classified := requireRejected(t, err)
		require.Equal(t, fantasyanthropic.Name, classified.Provider)
		require.Contains(t, classified.Message, "bad.pdf")
		require.Contains(t, classified.Message, "not a valid PDF")
		require.Contains(t, err.Error(), "reason=invalid_pdf")
	})

	t.Run("Rejects encrypted PDFs", func(t *testing.T) {
		t.Parallel()

		data := append(validPDFWithPages(1), []byte("\ntrailer << /Encrypt 5 0 R >>")...)
		err := ValidatePrompt(fantasyanthropic.Name, 200_000, promptWithPDF(pdfPart("locked.pdf", data)))
		classified := requireRejected(t, err)
		require.Contains(t, classified.Message, "locked.pdf")
		require.Contains(t, err.Error(), "reason=encrypted_pdf")
	})

	t.Run("Applies Bedrock caps and skips uncapped providers", func(t *testing.T) {
		t.Parallel()

		err := ValidatePrompt(fantasybedrock.Name, 200_000, promptWithPDF(pdfPart("bad.pdf", []byte("not a pdf"))))
		require.Equal(t, fantasybedrock.Name, requireRejected(t, err).Provider)

		err = ValidatePrompt(fantasyopenai.Name, 0, promptWithPDF(pdfPart("bad.pdf", []byte("not a pdf"))))
		require.NoError(t, err)
	})

	t.Run("Uses larger page cap for larger context models", func(t *testing.T) {
		t.Parallel()

		prompt := promptWithPDF(pdfPart("long.pdf", validPDFWithPages(101)))
		err := ValidatePrompt(fantasyanthropic.Name, 200_000, prompt)
		require.Contains(t, err.Error(), "page_cap=100")

		err = ValidatePrompt(fantasyanthropic.Name, 200_001, prompt)
		require.NoError(t, err)
	})
}

func TestLimitsFor(t *testing.T) {
	t.Parallel()

	t.Run("Bedrock uses a lower payload cap than Anthropic", func(t *testing.T) {
		t.Parallel()

		anthropic, ok := limitsFor(fantasyanthropic.Name, 200_000)
		require.True(t, ok)
		require.Equal(t, pdfRequestCapBytes, anthropic.requestPayloadBytes)

		bedrock, ok := limitsFor(fantasybedrock.Name, 200_000)
		require.True(t, ok)
		require.Equal(t, bedrockPDFRequestCapBytes, bedrock.requestPayloadBytes)

		require.Less(t, bedrock.requestPayloadBytes, anthropic.requestPayloadBytes)
		require.Equal(t, anthropic.pageCap, bedrock.pageCap)
	})

	t.Run("Uncapped providers return false", func(t *testing.T) {
		t.Parallel()

		_, ok := limitsFor(fantasyopenai.Name, 200_000)
		require.False(t, ok)
	})
}

func TestValidatePDFPromptWithCaps(t *testing.T) {
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
		require.Contains(t, classified.Message, "request limit")
		require.Contains(t, err.Error(), "reason=payload_cap")
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
	return (&validator{
		provider:    fantasyanthropic.Name,
		displayName: chatprovider.ProviderDisplayName(fantasyanthropic.Name),
		caps:        caps,
	}).validate(prompt)
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
	return []fantasy.Message{{Role: fantasy.MessageRoleUser, Content: content}}
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
