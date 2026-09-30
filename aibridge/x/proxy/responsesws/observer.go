package responsesws

import (
	"context"
	"strings"
	"sync"

	"github.com/openai/openai-go/v3/responses"
	"github.com/tidwall/gjson"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// Observer receives the frames bound to each interception. Calls come from
// the Send and reader goroutines concurrently, so implementations must be
// safe for concurrent use. They must not modify or retain frame, and must
// fail open: errors are logged, never returned.
type Observer interface {
	// ClientEvent receives a forwarded response.create or a response.steer
	// that targets a response owned by the interception.
	ClientEvent(ctx context.Context, interceptionID string, frame []byte)
	// ServerEvent receives an upstream event bound to the interception.
	ServerEvent(ctx context.Context, interceptionID string, frame []byte)
	// InterceptionEnded is called once before the interception end is recorded.
	InterceptionEnded(ctx context.Context, interceptionID string)
}

// NewRecordingObserver returns the minimal default observer. It records the
// user prompt of each response.create and response.steer, and token usage
// from each terminal response.
func NewRecordingObserver(rec recorder.Recorder, logger slog.Logger) Observer {
	return &recordingObserver{rec: rec, logger: logger, prompts: make(map[string]string)}
}

type recordingObserver struct {
	rec    recorder.Recorder
	logger slog.Logger

	mu sync.Mutex
	// prompts holds create prompts until response.created supplies the
	// response ID used as the prompt message ID.
	prompts map[string]string
}

func (o *recordingObserver) ClientEvent(ctx context.Context, interceptionID string, frame []byte) {
	prompt, ok := lastUserPrompt(gjson.GetBytes(frame, "input"))
	if !ok {
		return
	}
	switch gjson.GetBytes(frame, "type").String() {
	case eventCreate:
		o.mu.Lock()
		o.prompts[interceptionID] = prompt
		o.mu.Unlock()
	case eventSteer:
		o.recordPrompt(ctx, interceptionID, gjson.GetBytes(frame, "previous_response_id").String(), prompt)
	}
}

func (o *recordingObserver) ServerEvent(ctx context.Context, interceptionID string, frame []byte) {
	switch gjson.GetBytes(frame, "type").String() {
	case eventCreated:
		o.mu.Lock()
		prompt, ok := o.prompts[interceptionID]
		delete(o.prompts, interceptionID)
		o.mu.Unlock()
		if ok {
			o.recordPrompt(ctx, interceptionID, gjson.GetBytes(frame, "response.id").String(), prompt)
		}
	case eventCompleted, eventIncomplete, eventFailed:
		raw := gjson.GetBytes(frame, "response")
		if !raw.Get("usage").IsObject() {
			return
		}
		var response responses.Response
		if err := response.UnmarshalJSON([]byte(raw.Raw)); err != nil {
			o.logger.Warn(ctx, "failed to parse terminal response", slog.Error(err), slog.F("interception_id", interceptionID))
			return
		}
		o.recordTokenUsage(ctx, interceptionID, &response)
	}
}

func (o *recordingObserver) InterceptionEnded(_ context.Context, interceptionID string) {
	o.mu.Lock()
	delete(o.prompts, interceptionID)
	o.mu.Unlock()
}

func (o *recordingObserver) recordPrompt(ctx context.Context, interceptionID, msgID, prompt string) {
	if err := o.rec.RecordPromptUsage(ctx, &recorder.PromptUsageRecord{
		InterceptionID: interceptionID,
		MsgID:          msgID,
		Prompt:         prompt,
	}); err != nil {
		o.logger.Warn(ctx, "failed to record prompt usage", slog.Error(err), slog.F("interception_id", interceptionID))
	}
}

// recordTokenUsage mirrors the HTTP Responses interceptor's accounting in
// aibridge/intercept/responses.
func (o *recordingObserver) recordTokenUsage(ctx context.Context, interceptionID string, response *responses.Response) {
	usage := response.Usage
	// InputTokens include cache read and write tokens.
	inputNonCacheTokens := max(0, usage.InputTokens-
		usage.InputTokensDetails.CachedTokens-
		usage.InputTokensDetails.CacheWriteTokens)
	var metadata recorder.Metadata
	if serviceTier := string(response.ServiceTier); serviceTier != "" {
		metadata = recorder.Metadata{recorder.MetadataKeyServiceTier: serviceTier}
	}
	if err := o.rec.RecordTokenUsage(ctx, &recorder.TokenUsageRecord{
		InterceptionID:        interceptionID,
		MsgID:                 response.ID,
		Input:                 inputNonCacheTokens,
		Output:                usage.OutputTokens,
		CacheReadInputTokens:  usage.InputTokensDetails.CachedTokens,
		CacheWriteInputTokens: usage.InputTokensDetails.CacheWriteTokens,
		Metadata:              metadata,
		ExtraTokenTypes: map[string]int64{
			"output_reasoning": usage.OutputTokensDetails.ReasoningTokens,
			"total_tokens":     usage.TotalTokens,
		},
	}); err != nil {
		o.logger.Warn(ctx, "failed to record token usage", slog.Error(err), slog.F("interception_id", interceptionID))
	}
}

// lastUserPrompt mirrors the HTTP Responses interceptor's prompt extraction:
// a string input is the prompt; for an item array only a trailing user
// message counts, joining its input_text parts with newlines.
func lastUserPrompt(input gjson.Result) (string, bool) {
	if input.Type == gjson.String {
		return input.Str, true
	}
	items := input.Array()
	if !input.IsArray() || len(items) == 0 {
		return "", false
	}
	last := items[len(items)-1]
	if last.Get("role").Str != "user" {
		return "", false
	}
	content := last.Get("content")
	if content.Type == gjson.String {
		return content.Str, true
	}
	var parts []string
	for _, c := range content.Array() {
		if c.Get("type").Str == "input_text" && c.Get("text").Type == gjson.String {
			parts = append(parts, c.Get("text").Str)
		}
	}
	return strings.Join(parts, "\n"), len(parts) > 0
}
