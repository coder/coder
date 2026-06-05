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

// ValidationReason identifies the PDF preflight failure category.
type ValidationReason string

const (
	ValidationReasonInvalidPDF ValidationReason = "invalid_pdf"
	ValidationReasonEncrypted  ValidationReason = "encrypted_pdf"
	ValidationReasonPageCap    ValidationReason = "page_cap"
	ValidationReasonPayloadCap ValidationReason = "payload_cap"
)

// ValidationError describes a PDF preflight failure.
type ValidationError struct {
	Reason              ValidationReason
	Message             string
	Provider            string
	ProviderDisplayName string
	FileName            string
	DataBytes           int
	FilePages           int
	TotalPages          int
	PageCap             int
	FileEncodedBytes    int
	EncodedPDFBytes     int
	RequestPayloadBytes int
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "pdf preflight failed"
	}
	parts := []string{
		fmt.Sprintf("reason=%s", e.Reason),
		fmt.Sprintf("provider=%q", e.Provider),
	}
	if e.FileName != "" {
		parts = append(parts, fmt.Sprintf("file=%q", e.FileName))
	}
	if e.DataBytes > 0 {
		parts = append(parts, fmt.Sprintf("data_bytes=%d", e.DataBytes))
	}
	if e.FilePages > 0 {
		parts = append(parts, fmt.Sprintf("file_pages=%d", e.FilePages))
	}
	if e.TotalPages > 0 {
		parts = append(parts, fmt.Sprintf("total_pages=%d", e.TotalPages))
	}
	if e.PageCap > 0 {
		parts = append(parts, fmt.Sprintf("page_cap=%d", e.PageCap))
	}
	if e.FileEncodedBytes > 0 {
		parts = append(parts, fmt.Sprintf("file_encoded_bytes=%d", e.FileEncodedBytes))
	}
	if e.EncodedPDFBytes > 0 {
		parts = append(parts, fmt.Sprintf("encoded_pdf_bytes=%d", e.EncodedPDFBytes))
	}
	if e.RequestPayloadBytes > 0 {
		parts = append(parts, fmt.Sprintf("request_payload_bytes=%d", e.RequestPayloadBytes))
	}
	return "pdf preflight failed: " + strings.Join(parts, " ")
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
	return validatePromptWithCaps(
		prompt,
		caps,
		normalizedProvider,
		chatprovider.ProviderDisplayName(normalizedProvider),
	)
}

func validatePromptWithCaps(
	prompt []fantasy.Message,
	caps chatprovider.AnthropicPDFCaps,
	provider string,
	providerDisplayName string,
) error {
	var totalPages int
	var encodedPDFBytes int
	for _, msg := range prompt {
		for _, part := range msg.Content {
			file, ok := fantasy.AsMessagePart[fantasy.FilePart](part)
			if !ok || chatfiles.BaseMediaType(file.MediaType) != pdfMediaType {
				continue
			}

			if !chatfiles.IsPDF(file.Data) {
				return &ValidationError{
					Reason:              ValidationReasonInvalidPDF,
					Message:             fmt.Sprintf("%s is not a valid PDF. Re-upload the original document.", attachmentLabel(file.Filename)),
					Provider:            provider,
					ProviderDisplayName: providerDisplayName,
					FileName:            strings.TrimSpace(file.Filename),
					DataBytes:           len(file.Data),
					PageCap:             caps.PageCap,
					RequestPayloadBytes: caps.RequestPayloadBytes,
				}
			}

			if chatfiles.IsEncryptedPDF(file.Data) {
				return &ValidationError{
					Reason:              ValidationReasonEncrypted,
					Message:             fmt.Sprintf("%s is encrypted or password-protected. Upload an unlocked copy.", attachmentLabel(file.Filename)),
					Provider:            provider,
					ProviderDisplayName: providerDisplayName,
					FileName:            strings.TrimSpace(file.Filename),
					DataBytes:           len(file.Data),
					PageCap:             caps.PageCap,
					RequestPayloadBytes: caps.RequestPayloadBytes,
				}
			}

			if pages, ok := chatfiles.ApproxPDFPageCount(file.Data); ok {
				totalPages += pages
				if caps.PageCap > 0 && totalPages > caps.PageCap {
					return pageCapError(
						file.Filename,
						pages,
						totalPages,
						caps,
						provider,
						providerDisplayName,
					)
				}
			}

			fileEncodedBytes := base64.StdEncoding.EncodedLen(len(file.Data))
			encodedPDFBytes += fileEncodedBytes
			if caps.RequestPayloadBytes > 0 && encodedPDFBytes > caps.RequestPayloadBytes {
				return &ValidationError{
					Reason: ValidationReasonPayloadCap,
					Message: fmt.Sprintf(
						"PDF attachments are too large for %s's request limit. Remove or shrink some PDFs and retry.",
						providerDisplayName,
					),
					Provider:            provider,
					ProviderDisplayName: providerDisplayName,
					FileName:            strings.TrimSpace(file.Filename),
					DataBytes:           len(file.Data),
					FileEncodedBytes:    fileEncodedBytes,
					EncodedPDFBytes:     encodedPDFBytes,
					PageCap:             caps.PageCap,
					RequestPayloadBytes: caps.RequestPayloadBytes,
				}
			}
		}
	}
	return nil
}

func pageCapError(
	fileName string,
	filePages int,
	totalPages int,
	caps chatprovider.AnthropicPDFCaps,
	provider string,
	providerDisplayName string,
) *ValidationError {
	message := fmt.Sprintf(
		"PDF attachments include about %d pages, but %s accepts at most %d pages for this model. Split the PDF and retry.",
		totalPages,
		providerDisplayName,
		caps.PageCap,
	)
	if filePages > caps.PageCap {
		message = fmt.Sprintf(
			"%s has about %d pages, but %s accepts at most %d pages for this model. Split the PDF and retry.",
			attachmentLabel(fileName),
			filePages,
			providerDisplayName,
			caps.PageCap,
		)
	}
	return &ValidationError{
		Reason:              ValidationReasonPageCap,
		Message:             message,
		Provider:            provider,
		ProviderDisplayName: providerDisplayName,
		FileName:            strings.TrimSpace(fileName),
		FilePages:           filePages,
		TotalPages:          totalPages,
		PageCap:             caps.PageCap,
		RequestPayloadBytes: caps.RequestPayloadBytes,
	}
}

func attachmentLabel(fileName string) string {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return "PDF attachment"
	}
	return fmt.Sprintf("PDF attachment %q", fileName)
}
