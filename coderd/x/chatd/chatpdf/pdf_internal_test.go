package chatpdf

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasybedrock "charm.land/fantasy/providers/bedrock"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

func TestValidatePDFPrompt(t *testing.T) {
	t.Parallel()

	t.Run("Rejects invalid Anthropic PDF bytes", func(t *testing.T) {
		t.Parallel()

		err := ValidatePrompt(
			" ANTHROPIC ", 200_000,
			promptWithPDF(pdfPart("bad.pdf", []byte("not a pdf"))),
		)
		validationErr := requireValidationError(t, err)
		require.Contains(t, validationErr.UserMessage(), "bad.pdf")
		require.Contains(t, validationErr.UserMessage(), "not a valid PDF")
		require.Contains(t, validationErr.Error(), `provider="anthropic"`)
		require.Contains(t, validationErr.Error(), "reason=invalid_pdf")
	})

	t.Run("Rejects encrypted PDFs", func(t *testing.T) {
		t.Parallel()

		data := append(validPDFWithPages(1), []byte("\ntrailer << /Encrypt 5 0 R >>")...)
		err := ValidatePrompt(fantasyanthropic.Name, 200_000, promptWithPDF(pdfPart("locked.pdf", data)))
		validationErr := requireValidationError(t, err)
		require.Contains(t, validationErr.UserMessage(), "locked.pdf")
		require.Contains(t, validationErr.Error(), "reason=encrypted_pdf")
	})

	t.Run("Applies Bedrock caps and skips uncapped providers", func(t *testing.T) {
		t.Parallel()

		err := ValidatePrompt(fantasybedrock.Name, 200_000, promptWithPDF(pdfPart("bad.pdf", []byte("not a pdf"))))
		require.Contains(t, requireValidationError(t, err).Error(), `provider="bedrock"`)

		err = ValidatePrompt(fantasyopenai.Name, 0, promptWithPDF(pdfPart("bad.pdf", []byte("not a pdf"))))
		require.NoError(t, err)
	})

	t.Run("Uses larger page cap for larger context models", func(t *testing.T) {
		t.Parallel()

		prompt := promptWithPDF(pdfPart("long.pdf", validPDFWithPages(101)))
		err := ValidatePrompt(fantasyanthropic.Name, 200_000, prompt)
		require.Contains(t, requireValidationError(t, err).Error(), "page_cap=100")

		err = ValidatePrompt(fantasyanthropic.Name, 200_001, prompt)
		require.NoError(t, err)
	})
}

func TestValidatePDFPromptWithCaps(t *testing.T) {
	t.Parallel()

	t.Run("Counts repeated PDF parts and aggregate pages", func(t *testing.T) {
		t.Parallel()

		part := pdfPart("repeat.pdf", validPDFWithPages(1))
		err := validatePromptWithCaps(
			promptWithPDF(part, part),
			chatprovider.AnthropicPDFCaps{RequestPayloadBytes: 1 << 20, PageCap: 1},
			fantasyanthropic.Name,
		)
		validationErr := requireValidationError(t, err)
		require.Contains(t, validationErr.UserMessage(), "about 2 pages")
		require.Contains(t, validationErr.Error(), "total_pages=2")
	})

	t.Run("Rejects aggregate encoded payload over cap", func(t *testing.T) {
		t.Parallel()

		data := validPDFWithPages(1)
		err := validatePromptWithCaps(
			promptWithPDF(pdfPart("payload.pdf", data)),
			chatprovider.AnthropicPDFCaps{
				RequestPayloadBytes: base64.StdEncoding.EncodedLen(len(data)) - 1,
				PageCap:             100,
			},
			fantasyanthropic.Name,
		)
		validationErr := requireValidationError(t, err)
		require.Contains(t, validationErr.UserMessage(), "request limit")
		require.Contains(t, validationErr.Error(), "reason=payload_cap")
	})

	t.Run("Allows PDFs with unknown page count", func(t *testing.T) {
		t.Parallel()

		err := validatePromptWithCaps(
			promptWithPDF(pdfPart("unknown.pdf", []byte("%PDF-1.7\nxref\n0 0"))),
			chatprovider.AnthropicPDFCaps{RequestPayloadBytes: 1 << 20, PageCap: 1},
			fantasyanthropic.Name,
		)
		require.NoError(t, err)
	})
}

func requireValidationError(t *testing.T, err error) *ValidationError {
	t.Helper()

	require.Error(t, err)
	var validationErr *ValidationError
	require.True(t, errors.As(err, &validationErr))
	return validationErr
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
