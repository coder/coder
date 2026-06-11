package chatsanitize

import (
	"encoding/base64"
	"fmt"
	"strings"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasybedrock "charm.land/fantasy/providers/bedrock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// anthropicRequestCapBytes is Anthropic's documented request size limit
	// for the Messages API.
	anthropicRequestCapBytes = 32 * 1024 * 1024

	// bedrockRequestCapBytes is Bedrock's InvokeModel request payload limit,
	// which is lower than Anthropic's Messages API limit and binds first for
	// Bedrock-hosted Claude requests.
	bedrockRequestCapBytes = 20 * 1024 * 1024

	pdfDefaultPageCap       = 100
	pdfLargeContextPageCap  = 600
	pdfLargeContextMinToken = 200_000

	pdfMediaType = "application/pdf"
)

// limits describes the preflight caps that apply to a request, plus the
// user-facing provider name used when reporting a violated cap.
type limits struct {
	displayName         string
	requestPayloadBytes int
	pageCap             int
}

// limitsFor returns the preflight caps for provider, or (zero, false) when no
// documented cap applies. provider must be the canonical fantasy provider
// name; like ApplyAnthropicProviderToolGuard, this fails open for
// unrecognized names. Bedrock shares Anthropic's PDF page caps because
// fantasy's bedrock provider wraps the anthropic client, but uses a lower
// request payload cap to match Bedrock's InvokeModel transport limit.
func limitsFor(provider string, contextLimit int64) (limits, bool) {
	switch provider {
	case fantasyanthropic.Name, fantasybedrock.Name:
		pageCap := pdfDefaultPageCap
		// A missing context limit is treated as a 200k-token model so preflight
		// does not allow a request Anthropic may reject for page count.
		if contextLimit > pdfLargeContextMinToken {
			pageCap = pdfLargeContextPageCap
		}
		// Display names mirror chatprovider.ProviderDisplayName, which cannot
		// be imported here without an import cycle through chatprompt.
		displayName := "Anthropic"
		payloadCap := anthropicRequestCapBytes
		if provider == fantasybedrock.Name {
			displayName = "AWS Bedrock"
			payloadCap = bedrockRequestCapBytes
		}
		return limits{
			displayName:         displayName,
			requestPayloadBytes: payloadCap,
			pageCap:             pageCap,
		}, true
	default:
		return limits{}, false
	}
}

// ValidatePromptLimits rejects prompts that Anthropic or Bedrock-hosted Claude
// requests are known to reject: PDF attachments that are invalid, encrypted,
// or over the documented page caps, and requests whose estimated body size
// exceeds the provider's request size limit. provider must be the canonical
// fantasy provider name; for uncapped or unrecognized providers it is a no-op.
// A returned error is classified as a non-retryable configuration error
// carrying a user-facing message, so callers can propagate it directly.
func ValidatePromptLimits(provider string, contextLimit int64, prompt []fantasy.Message) error {
	caps, ok := limitsFor(provider, contextLimit)
	if !ok {
		return nil
	}
	return (&validator{provider: provider, caps: caps}).validate(prompt)
}

// validator walks a provider-ready prompt and accumulates PDF page counts and
// an estimated request size across every part so per-request caps account for
// repeated references, inline attachments, and surrounding prompt content.
type validator struct {
	provider string
	caps     limits

	totalPages            int
	estimatedRequestBytes int
}

func (v *validator) validate(prompt []fantasy.Message) error {
	for _, msg := range prompt {
		for _, part := range msg.Content {
			v.estimatedRequestBytes += estimatePartBytes(part)
			file, ok := fantasy.AsMessagePart[fantasy.FilePart](part)
			if !ok || chatfiles.BaseMediaType(file.MediaType) != pdfMediaType {
				continue
			}
			if err := v.checkPDF(file); err != nil {
				return err
			}
		}
	}
	// The payload cap is enforced after the walk so every part counts
	// regardless of order. The estimate is a lower bound on the real request
	// body, so exceeding the cap here guarantees the provider would reject.
	if v.caps.requestPayloadBytes > 0 && v.estimatedRequestBytes > v.caps.requestPayloadBytes {
		return v.reject(
			fmt.Sprintf("This request is too large for %s's request size limit. Remove or shrink attachments and retry.", v.caps.displayName),
			"reason=payload_cap estimated_request_bytes=%d request_payload_bytes=%d",
			v.estimatedRequestBytes, v.caps.requestPayloadBytes,
		)
	}
	return nil
}

func (v *validator) checkPDF(file fantasy.FilePart) error {
	name := strings.TrimSpace(file.Filename)
	switch {
	case !chatfiles.IsPDF(file.Data):
		return v.reject(
			fmt.Sprintf("%s is not a valid PDF. Re-upload the original document.", attachmentLabel(name)),
			"reason=invalid_pdf file=%q data_bytes=%d", name, len(file.Data),
		)
	case chatfiles.IsEncryptedPDF(file.Data):
		return v.reject(
			fmt.Sprintf("%s is encrypted or password-protected. Upload an unlocked copy.", attachmentLabel(name)),
			"reason=encrypted_pdf file=%q data_bytes=%d", name, len(file.Data),
		)
	}

	if pages, ok := chatfiles.ApproxPDFPageCount(file.Data); ok {
		v.totalPages += pages
		if v.caps.pageCap > 0 && v.totalPages > v.caps.pageCap {
			return v.reject(
				v.pageCapMessage(name, pages),
				"reason=page_cap file=%q file_pages=%d total_pages=%d page_cap=%d",
				name, pages, v.totalPages, v.caps.pageCap,
			)
		}
	}
	return nil
}

// pageCapMessage names the single oversized file when one PDF alone exceeds the
// cap, otherwise it reports the aggregate page count across all attachments.
func (v *validator) pageCapMessage(name string, filePages int) string {
	if filePages > v.caps.pageCap {
		return fmt.Sprintf(
			"%s has about %d pages, but %s accepts at most %d pages for this model. Split the PDF and retry.",
			attachmentLabel(name), filePages, v.caps.displayName, v.caps.pageCap,
		)
	}
	return fmt.Sprintf(
		"PDF attachments include about %d pages, but %s accepts at most %d pages for this model. Split the PDF and retry.",
		v.totalPages, v.caps.displayName, v.caps.pageCap,
	)
}

func attachmentLabel(name string) string {
	if name == "" {
		return "PDF attachment"
	}
	return fmt.Sprintf("PDF attachment %q", name)
}

// estimatePartBytes estimates a part's serialized contribution to the provider
// request body. Binary data is counted at base64-encoded size. JSON framing
// overhead is not modeled, so this is a lower bound on the real request size.
func estimatePartBytes(part fantasy.MessagePart) int {
	if file, ok := fantasy.AsMessagePart[fantasy.FilePart](part); ok {
		return base64.StdEncoding.EncodedLen(len(file.Data))
	}
	if text, ok := fantasy.AsMessagePart[fantasy.TextPart](part); ok {
		return len(text.Text)
	}
	if reasoning, ok := fantasy.AsMessagePart[fantasy.ReasoningPart](part); ok {
		return len(reasoning.Text)
	}
	if call, ok := safeMessageToolCallPart(part); ok {
		return len(call.ToolName) + len(call.Input)
	}
	if result, ok := safeMessageToolResultPart(part); ok {
		return toolResultOutputBytes(result.Output)
	}
	return 0
}

func toolResultOutputBytes(output fantasy.ToolResultOutputContent) int {
	if text, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](output); ok {
		return len(text.Text)
	}
	if media, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentMedia](output); ok {
		// Media data is already base64-encoded.
		return len(media.Data) + len(media.Text)
	}
	return 0
}

// reject builds a non-retryable configuration error. userMessage is shown to
// the user; detail carries structured diagnostics for operator logs.
func (v *validator) reject(userMessage string, detailFormat string, args ...any) error {
	detail := "prompt preflight failed: " + fmt.Sprintf(detailFormat, args...)
	return chaterror.WithClassification(xerrors.New(detail), chaterror.ClassifiedError{
		Kind:      codersdk.ChatErrorKindConfig,
		Provider:  v.provider,
		Message:   userMessage,
		Detail:    detail,
		Retryable: false,
	})
}
