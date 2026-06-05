package chatpdf

import (
	"encoding/base64"
	"fmt"
	"strings"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
)

const pdfMediaType = "application/pdf"

// ValidationError describes a PDF preflight failure.
type ValidationError struct {
	Message string
	Detail  string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "pdf preflight failed"
	}
	if e.Detail != "" {
		return e.Detail
	}
	return e.Message
}

// UserMessage returns the concise message to show to the user.
func (e *ValidationError) UserMessage() string {
	if e == nil || e.Message == "" {
		return "PDF attachments are not valid for this model. Update the PDFs and retry."
	}
	return e.Message
}

// ValidatePrompt rejects PDF attachments that Anthropic or Bedrock-hosted
// Claude requests are known to reject.
func ValidatePrompt(provider string, contextLimit int64, prompt []fantasy.Message) error {
	caps, ok := chatprovider.PDFCaps(provider, contextLimit)
	if !ok {
		return nil
	}
	normalizedProvider := chatprovider.NormalizeProvider(provider)
	return validatePromptWithCaps(prompt, caps, normalizedProvider)
}

func validatePromptWithCaps(
	prompt []fantasy.Message,
	caps chatprovider.AnthropicPDFCaps,
	provider string,
) error {
	displayName := chatprovider.ProviderDisplayName(provider)
	var totalPages int
	var encodedPDFBytes int
	for _, msg := range prompt {
		for _, part := range msg.Content {
			file, ok := fantasy.AsMessagePart[fantasy.FilePart](part)
			if !ok || chatfiles.BaseMediaType(file.MediaType) != pdfMediaType {
				continue
			}

			fileName := strings.TrimSpace(file.Filename)
			if !chatfiles.IsPDF(file.Data) {
				return validationError(
					fmt.Sprintf("%s is not a valid PDF. Re-upload the original document.", attachmentLabel(fileName)),
					"reason=invalid_pdf provider=%q file=%q data_bytes=%d",
					provider, fileName, len(file.Data),
				)
			}
			if chatfiles.IsEncryptedPDF(file.Data) {
				return validationError(
					fmt.Sprintf("%s is encrypted or password-protected. Upload an unlocked copy.", attachmentLabel(fileName)),
					"reason=encrypted_pdf provider=%q file=%q data_bytes=%d",
					provider, fileName, len(file.Data),
				)
			}
			if pages, ok := chatfiles.ApproxPDFPageCount(file.Data); ok {
				totalPages += pages
				if caps.PageCap > 0 && totalPages > caps.PageCap {
					return pageCapError(fileName, pages, totalPages, caps.PageCap, provider, displayName)
				}
			}

			encodedPDFBytes += base64.StdEncoding.EncodedLen(len(file.Data))
			if caps.RequestPayloadBytes > 0 && encodedPDFBytes > caps.RequestPayloadBytes {
				return validationError(
					fmt.Sprintf("PDF attachments are too large for %s's request limit. Remove or shrink some PDFs and retry.", displayName),
					"reason=payload_cap provider=%q file=%q encoded_pdf_bytes=%d request_payload_bytes=%d",
					provider, fileName, encodedPDFBytes, caps.RequestPayloadBytes,
				)
			}
		}
	}
	return nil
}

func pageCapError(
	fileName string,
	filePages int,
	totalPages int,
	pageCap int,
	provider string,
	displayName string,
) *ValidationError {
	message := fmt.Sprintf(
		"PDF attachments include about %d pages, but %s accepts at most %d pages for this model. Split the PDF and retry.",
		totalPages, displayName, pageCap,
	)
	if filePages > pageCap {
		message = fmt.Sprintf(
			"%s has about %d pages, but %s accepts at most %d pages for this model. Split the PDF and retry.",
			attachmentLabel(fileName), filePages, displayName, pageCap,
		)
	}
	return validationError(
		message,
		"reason=page_cap provider=%q file=%q file_pages=%d total_pages=%d page_cap=%d",
		provider, fileName, filePages, totalPages, pageCap,
	)
}

func validationError(message string, detail string, args ...any) *ValidationError {
	return &ValidationError{
		Message: message,
		Detail:  "pdf preflight failed: " + fmt.Sprintf(detail, args...),
	}
}

func attachmentLabel(fileName string) string {
	if fileName == "" {
		return "PDF attachment"
	}
	return fmt.Sprintf("PDF attachment %q", fileName)
}
