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

	t.Run("ValidAnthropicPDFPasses", func(t *testing.T) {
		t.Parallel()

		err := ValidatePrompt(
			fantasyanthropic.Name,
			200_000,
			promptWithPDF(pdfPart("report.pdf", validPDFWithPages(1))),
		)
		require.NoError(t, err)
	})

	t.Run("UncappedProviderSkipsPDFPreflight", func(t *testing.T) {
		t.Parallel()

		err := ValidatePrompt(
			fantasyopenai.Name,
			0,
			promptWithPDF(pdfPart("not-a-pdf.pdf", []byte("not a pdf"))),
		)
		require.NoError(t, err)
	})

	t.Run("InvalidPDFBytesFail", func(t *testing.T) {
		t.Parallel()

		err := ValidatePrompt(
			" ANTHROPIC ",
			200_000,
			promptWithPDF(pdfPart("not-a-pdf.pdf", []byte("not a pdf"))),
		)
		validationErr := requireValidationError(t, err)
		require.Equal(t, ValidationReasonInvalidPDF, validationErr.Reason)
		require.Equal(t, fantasyanthropic.Name, validationErr.Provider)
		require.Contains(t, validationErr.UserMessage(), "not-a-pdf.pdf")
		require.Contains(t, validationErr.Error(), "data_bytes=9")
	})

	t.Run("EncryptedPDFFails", func(t *testing.T) {
		t.Parallel()

		data := append(validPDFWithPages(1), []byte("\ntrailer << /Encrypt 5 0 R >>")...)
		err := ValidatePrompt(
			fantasyanthropic.Name,
			200_000,
			promptWithPDF(pdfPart("locked.pdf", data)),
		)
		validationErr := requireValidationError(t, err)
		require.Equal(t, ValidationReasonEncrypted, validationErr.Reason)
		require.Contains(t, validationErr.UserMessage(), "locked.pdf")
	})

	t.Run("BedrockUsesAnthropicPDFPreflight", func(t *testing.T) {
		t.Parallel()

		err := ValidatePrompt(
			fantasybedrock.Name,
			200_000,
			promptWithPDF(pdfPart("not-a-pdf.pdf", []byte("not a pdf"))),
		)
		validationErr := requireValidationError(t, err)
		require.Equal(t, ValidationReasonInvalidPDF, validationErr.Reason)
		require.Equal(t, fantasybedrock.Name, validationErr.Provider)
	})

	t.Run("ContextLimitControlsPageCap", func(t *testing.T) {
		t.Parallel()

		data := validPDFWithPages(101)
		err := ValidatePrompt(
			fantasyanthropic.Name,
			200_000,
			promptWithPDF(pdfPart("long.pdf", data)),
		)
		validationErr := requireValidationError(t, err)
		require.Equal(t, ValidationReasonPageCap, validationErr.Reason)
		require.Equal(t, 100, validationErr.PageCap)

		err = ValidatePrompt(
			fantasyanthropic.Name,
			200_001,
			promptWithPDF(pdfPart("long.pdf", data)),
		)
		require.NoError(t, err)
	})
}

func TestValidatePDFPromptWithCaps(t *testing.T) {
	t.Parallel()

	t.Run("AggregatePageCountFailsAcrossPDFParts", func(t *testing.T) {
		t.Parallel()

		caps := chatprovider.AnthropicPDFCaps{
			RequestPayloadBytes: 1 << 20,
			PageCap:             2,
		}
		err := validatePromptWithCaps(
			promptWithPDF(
				pdfPart("first.pdf", validPDFWithPages(2)),
				pdfPart("second.pdf", validPDFWithPages(1)),
			),
			caps,
			fantasyanthropic.Name,
			"Anthropic",
		)
		validationErr := requireValidationError(t, err)
		require.Equal(t, ValidationReasonPageCap, validationErr.Reason)
		require.Equal(t, "second.pdf", validationErr.FileName)
		require.Equal(t, 3, validationErr.TotalPages)
		require.Contains(t, validationErr.UserMessage(), "about 3 pages")
	})

	t.Run("RepeatedPDFPartsCountPerOccurrence", func(t *testing.T) {
		t.Parallel()

		caps := chatprovider.AnthropicPDFCaps{
			RequestPayloadBytes: 1 << 20,
			PageCap:             1,
		}
		part := pdfPart("repeat.pdf", validPDFWithPages(1))
		err := validatePromptWithCaps(
			promptWithPDF(part, part),
			caps,
			fantasyanthropic.Name,
			"Anthropic",
		)
		validationErr := requireValidationError(t, err)
		require.Equal(t, ValidationReasonPageCap, validationErr.Reason)
		require.Equal(t, 2, validationErr.TotalPages)
	})

	t.Run("SinglePDFOverPageCapNamesFile", func(t *testing.T) {
		t.Parallel()

		caps := chatprovider.AnthropicPDFCaps{
			RequestPayloadBytes: 1 << 20,
			PageCap:             1,
		}
		err := validatePromptWithCaps(
			promptWithPDF(pdfPart("large.pdf", validPDFWithPages(2))),
			caps,
			fantasyanthropic.Name,
			"Anthropic",
		)
		validationErr := requireValidationError(t, err)
		require.Equal(t, ValidationReasonPageCap, validationErr.Reason)
		require.Contains(t, validationErr.UserMessage(), "large.pdf")
		require.Contains(t, validationErr.UserMessage(), "about 2 pages")
	})

	t.Run("AggregateEncodedPayloadFails", func(t *testing.T) {
		t.Parallel()

		data := validPDFWithPages(1)
		caps := chatprovider.AnthropicPDFCaps{
			RequestPayloadBytes: base64.StdEncoding.EncodedLen(len(data)) - 1,
			PageCap:             100,
		}
		err := validatePromptWithCaps(
			promptWithPDF(pdfPart("payload.pdf", data)),
			caps,
			fantasyanthropic.Name,
			"Anthropic",
		)
		validationErr := requireValidationError(t, err)
		require.Equal(t, ValidationReasonPayloadCap, validationErr.Reason)
		require.Equal(t, base64.StdEncoding.EncodedLen(len(data)), validationErr.EncodedPDFBytes)
		require.Contains(t, validationErr.UserMessage(), "request limit")
	})

	t.Run("UnknownPageCountDoesNotFail", func(t *testing.T) {
		t.Parallel()

		caps := chatprovider.AnthropicPDFCaps{
			RequestPayloadBytes: 1 << 20,
			PageCap:             1,
		}
		err := validatePromptWithCaps(
			promptWithPDF(pdfPart("unknown.pdf", []byte("%PDF-1.7\nxref\n0 0"))),
			caps,
			fantasyanthropic.Name,
			"Anthropic",
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
	return []fantasy.Message{
		{
			Role:    fantasy.MessageRoleUser,
			Content: content,
		},
	}
}

func pdfPart(name string, data []byte) fantasy.FilePart {
	return fantasy.FilePart{
		Filename:  name,
		Data:      data,
		MediaType: "application/pdf",
	}
}

func validPDFWithPages(pages int) []byte {
	var b strings.Builder
	_, _ = b.WriteString("%PDF-1.7\n")
	for i := range pages {
		_, _ = fmt.Fprintf(&b, "%d 0 obj << /Type /Page >> endobj\n", i+1)
	}
	return []byte(b.String())
}
