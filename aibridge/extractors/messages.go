package extractors

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/intercept/messages"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// messagesExtractor extracts recording facts from Anthropic Messages API
// traffic.
type messagesExtractor struct {
	// prompt is the last user prompt in the request, nil when absent. It is
	// recorded once response message ID is known (PromptUsageRecord.MsgID).
	prompt *string
	// streaming reports whether the request asked for a streamed response.
	// Used as fallback, response metadata has priority.
	streaming bool
}

// NewMessages returns an [Extractor] for one Anthropic Messages API request.
func NewMessages() Extractor {
	return &messagesExtractor{}
}

func (e *messagesExtractor) ExtractRequest(reqBody []byte, start *recorder.InterceptionRecord) error {
	payload, err := messages.NewRequestPayload(reqBody)
	if err != nil {
		return xerrors.Errorf("parse messages request: %w", err)
	}
	start.Model = payload.Model()
	start.CorrelatingToolCallID = payload.CorrelatingToolCallID()
	e.streaming = payload.Stream()

	prompt, found, err := payload.LastUserPrompt()
	if err != nil {
		return xerrors.Errorf("read last user prompt: %w", err)
	}
	if found {
		e.prompt = &prompt
	}
	return nil
}
