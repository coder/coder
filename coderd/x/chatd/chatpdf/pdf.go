package chatpdf

import (
	"encoding/base64"
	"fmt"
	"strings"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasybedrock "charm.land/fantasy/providers/bedrock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatfiles"
	"github.com/coder/coder/v2/codersdk"
)

const pdfMediaType = "application/pdf"

const (
	// pdfRequestCapBytes is Anthropic's documented request payload limit for
	// PDF input requests on the Messages API.
	pdfRequestCapBytes = 32 * 1024 * 1024

	// bedrockPDFRequestCapBytes is Bedrock's InvokeModel request payload limit,
	// which is lower than Anthropic's Messages API limit and binds first for
	// Bedrock-hosted Claude requests.
	bedrockPDFRequestCapBytes = 20 * 1024 * 1024

	pdfDefaultPageCap       = 100
	pdfLargeContextPageCap  = 600
	pdfLargeContextMinToken = 200_000
)

// limits describes the PDF preflight caps that apply to a request.
type limits struct {
	requestPayloadBytes int
	pageCap             int
}

// limitsFor returns the PDF preflight caps for provider, or (zero, false) when
// no documented cap applies. Bedrock shares Anthropic's page caps because
// fantasy's bedrock provider wraps the anthropic client, but uses a lower
// request payload cap to match Bedrock's InvokeModel transport limit.
func limitsFor(provider string, contextLimit int64) (limits, bool) {
	normalized := chatprovider.NormalizeProvider(provider)
	switch normalized {
	case fantasyanthropic.Name, fantasybedrock.Name:
		pageCap := pdfDefaultPageCap
		// A missing context limit is treated as a 200k-token model so preflight
		// does not allow a request Anthropic may reject for page count.
		if contextLimit > pdfLargeContextMinToken {
			pageCap = pdfLargeContextPageCap
		}
		payloadCap := pdfRequestCapBytes
		if normalized == fantasybedrock.Name {
			payloadCap = bedrockPDFRequestCapBytes
		}
		return limits{requestPayloadBytes: payloadCap, pageCap: pageCap}, true
	default:
		return limits{}, false
	}
}

// ValidatePrompt rejects PDF attachments that Anthropic or Bedrock-hosted
// Claude requests are known to reject. For uncapped providers it is a no-op.
// A returned error is classified as a non-retryable configuration error
// carrying a user-facing message, so callers can propagate it directly.
func ValidatePrompt(provider string, contextLimit int64, prompt []fantasy.Message) error {
	caps, ok := limitsFor(provider, contextLimit)
	if !ok {
		return nil
	}
	return (&validator{
		provider:    chatprovider.NormalizeProvider(provider),
		displayName: chatprovider.ProviderDisplayName(provider),
		caps:        caps,
	}).validate(prompt)
}

// validator walks a provider-ready prompt and accumulates PDF page counts and
// encoded payload size across every PDF part so per-request caps account for
// repeated references and inline attachments.
type validator struct {
	provider    string
	displayName string
	caps        limits

	totalPages      int
	encodedPDFBytes int
}

func (v *validator) validate(prompt []fantasy.Message) error {
	for _, msg := range prompt {
		for _, part := range msg.Content {
			file, ok := fantasy.AsMessagePart[fantasy.FilePart](part)
			if !ok || chatfiles.BaseMediaType(file.MediaType) != pdfMediaType {
				continue
			}
			if err := v.checkPDF(file); err != nil {
				return err
			}
		}
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

	v.encodedPDFBytes += base64.StdEncoding.EncodedLen(len(file.Data))
	if v.caps.requestPayloadBytes > 0 && v.encodedPDFBytes > v.caps.requestPayloadBytes {
		return v.reject(
			fmt.Sprintf("PDF attachments are too large for %s's request limit. Remove or shrink some PDFs and retry.", v.displayName),
			"reason=payload_cap file=%q encoded_pdf_bytes=%d request_payload_bytes=%d",
			name, v.encodedPDFBytes, v.caps.requestPayloadBytes,
		)
	}
	return nil
}

// pageCapMessage names the single oversized file when one PDF alone exceeds the
// cap, otherwise it reports the aggregate page count across all attachments.
func (v *validator) pageCapMessage(name string, filePages int) string {
	if filePages > v.caps.pageCap {
		return fmt.Sprintf(
			"%s has about %d pages, but %s accepts at most %d pages for this model. Split the PDF and retry.",
			attachmentLabel(name), filePages, v.displayName, v.caps.pageCap,
		)
	}
	return fmt.Sprintf(
		"PDF attachments include about %d pages, but %s accepts at most %d pages for this model. Split the PDF and retry.",
		v.totalPages, v.displayName, v.caps.pageCap,
	)
}

// reject builds a non-retryable configuration error. userMessage is shown to
// the user; detail carries structured diagnostics for operator logs.
func (v *validator) reject(userMessage string, detailFormat string, args ...any) error {
	detail := "pdf preflight failed: " + fmt.Sprintf(detailFormat, args...)
	return chaterror.WithClassification(xerrors.New(detail), chaterror.ClassifiedError{
		Kind:      codersdk.ChatErrorKindConfig,
		Provider:  v.provider,
		Message:   userMessage,
		Detail:    detail,
		Retryable: false,
	})
}

func attachmentLabel(name string) string {
	if name == "" {
		return "PDF attachment"
	}
	return fmt.Sprintf("PDF attachment %q", name)
}
