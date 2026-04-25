package chattest

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// OpenAIHandler handles OpenAI API requests and returns a response.
type OpenAIHandler func(req *OpenAIRequest) OpenAIResponse

// OpenAIResponse represents a response to an OpenAI request.
// Either StreamingChunks or Response should be set, not both.
type OpenAIResponse struct {
	StreamingChunks <-chan OpenAIChunk
	Response        *OpenAICompletion
	Reasoning       *OpenAIReasoningItem
	WebSearch       *OpenAIWebSearchCall
	FunctionCalls   []OpenAIResponsesFunctionCall
	ResponseID      string         // If set, used as the response ID in streamed events; otherwise auto-generated.
	Error           *ErrorResponse // If set, server returns this HTTP error instead of streaming/JSON.
}

// OpenAIReasoningItem configures a streamed reasoning output item for the
// Responses API test server.
type OpenAIReasoningItem struct {
	ID               string `json:"id,omitempty"`
	Summary          string `json:"summary,omitempty"`
	EncryptedContent string `json:"encrypted_content,omitempty"`
}

// OpenAIWebSearchCall configures a streamed web_search_call output item for the
// Responses API test server.
type OpenAIWebSearchCall struct {
	ID    string `json:"id,omitempty"`
	Query string `json:"query,omitempty"`
}

// OpenAIResponsesFunctionCall configures a streamed function_call
// output item for the Responses API test server. When the response
// is stored (store=true) the test server remembers the emitted
// call_id on the stored response so a follow-up request chained via
// previous_response_id must include the matching
// function_call_output in its input.
type OpenAIResponsesFunctionCall struct {
	// ItemID is the response item identifier ("fc_..."). Generated
	// when empty.
	ItemID string
	// CallID is the function-call identifier ("call_...") that
	// must be matched by a function_call_output on the next chained
	// request. Generated when empty.
	CallID string
	// Name is the function name emitted to the client.
	Name string
	// Arguments is the JSON-encoded argument string.
	Arguments string
}

// OpenAIRequest represents an OpenAI chat completion request.
type OpenAIRequest struct {
	*http.Request
	Model              string          `json:"model"`
	Messages           []OpenAIMessage `json:"messages"`
	Stream             bool            `json:"stream,omitempty"`
	Tools              []OpenAITool    `json:"tools,omitempty"`
	Prompt             []interface{}   `json:"prompt,omitempty"` // Legacy Responses API field alias; the real wire field is "input".
	Store              *bool           `json:"store,omitempty"`
	PreviousResponseID *string         `json:"previous_response_id,omitempty"`
	// Input is the raw Responses API input array. Each element is
	// an opaque JSON object (message / function_call /
	// function_call_output / reasoning / etc.). The test server
	// inspects it to enforce the chain-mode contract: follow-up
	// requests that set previous_response_id to a response with
	// unanswered function_calls must include the matching
	// function_call_output items here.
	Input []json.RawMessage `json:"input,omitempty"`
	// TODO: encoding/json ignores inline tags. Add custom UnmarshalJSON to capture unknown keys.
	Options map[string]interface{} `json:",inline"` //nolint:revive
}

// OpenAIMessage represents a message in an OpenAI request.
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIToolFunction represents the function definition inside a tool.
type OpenAIToolFunction struct {
	Name string `json:"name"`
}

// OpenAITool represents a tool definition in an OpenAI request.
type OpenAITool struct {
	Type     string             `json:"type"`
	Name     string             `json:"name,omitempty"`
	Function OpenAIToolFunction `json:"function"`
}

// OpenAIToolCallFunction represents the function details in a tool call.
type OpenAIToolCallFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// OpenAIToolCall represents a tool call in a streaming chunk or completion.
type OpenAIToolCall struct {
	ID       string                 `json:"id,omitempty"`
	Type     string                 `json:"type,omitempty"`
	Function OpenAIToolCallFunction `json:"function,omitempty"`
	Index    int                    `json:"index,omitempty"` // For streaming deltas
}

// OpenAIChunkChoice represents a choice in a streaming chunk.
type OpenAIChunkChoice struct {
	Index        int              `json:"index"`
	Delta        string           `json:"delta,omitempty"`
	ToolCalls    []OpenAIToolCall `json:"tool_calls,omitempty"`
	FinishReason string           `json:"finish_reason,omitempty"`
}

// OpenAIChunk represents a streaming chunk from OpenAI.
type OpenAIChunk struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []OpenAIChunkChoice `json:"choices"`
}

// OpenAICompletionChoice represents a choice in a completion response.
type OpenAICompletionChoice struct {
	Index        int              `json:"index"`
	Message      OpenAIMessage    `json:"message"`
	ToolCalls    []OpenAIToolCall `json:"tool_calls,omitempty"`
	FinishReason string           `json:"finish_reason"`
}

// OpenAICompletionUsage represents usage information in a completion response.
type OpenAICompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAICompletion represents a non-streaming OpenAI completion response.
type OpenAICompletion struct {
	ID      string                   `json:"id"`
	Object  string                   `json:"object"`
	Created int64                    `json:"created"`
	Model   string                   `json:"model"`
	Choices []OpenAICompletionChoice `json:"choices"`
	Usage   OpenAICompletionUsage    `json:"usage"`
}

// openAIServer is a test server that mocks the OpenAI API.
// openAIResponsesStoredState tracks information about a single
// response persisted on the fake Responses API server. It mirrors
// the server-side behavior OpenAI uses when store=true: the server
// remembers which function_call items a response emitted so that a
// follow-up request chained via previous_response_id can be
// validated against the expected function_call_output items.
type openAIResponsesStoredState struct {
	// pendingFunctionCalls is the set of function-call IDs that
	// were emitted by the response and have not yet been answered
	// by a function_call_output with the same call_id.
	pendingFunctionCalls map[string]struct{}
}

type openAIServer struct {
	mu      sync.Mutex
	t       testing.TB
	server  *httptest.Server
	handler OpenAIHandler
	request *OpenAIRequest
	// storedResponses maps response IDs to their server-side state
	// for the subset of requests that set store=true. Access is
	// guarded by mu.
	storedResponses map[string]*openAIResponsesStoredState
}

// OpenAI creates a fake OpenAI-compatible test server with a
// sensible default handler and returns its base URL. It handles
// both the Responses API (/responses) and the Chat Completions
// API (/chat/completions).
//
// Non-streaming requests (e.g. structured-output title generation)
// receive a JSON payload satisfying the generatedTitle schema.
// Streaming requests (e.g. the main chat loop) receive a single
// text chunk. Use NewOpenAI when a test needs control over the
// response.
func OpenAI(t testing.TB) string {
	t.Helper()
	return NewOpenAI(t, func(req *OpenAIRequest) OpenAIResponse {
		if req.Stream {
			return OpenAIStreamingResponse(OpenAITextChunks("Hello from test server.")...)
		}
		return OpenAINonStreamingResponse(`{"title": "Test Chat"}`)
	})
}

// NewOpenAI creates a new OpenAI test server with a handler function.
// The handler is called for each request and should return either a streaming
// response (via channel) or a non-streaming response.
// Returns the base URL of the server.
func NewOpenAI(t testing.TB, handler OpenAIHandler) string {
	t.Helper()

	s := &openAIServer{
		t:               t,
		handler:         handler,
		storedResponses: make(map[string]*openAIResponsesStoredState),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat/completions", s.handleChatCompletions)
	mux.HandleFunc("POST /responses", s.handleResponses)

	s.server = httptest.NewServer(mux)

	t.Cleanup(func() {
		s.server.Close()
	})

	return s.server.URL
}

func (s *openAIServer) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req OpenAIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Request = r

	s.mu.Lock()
	s.request = &req
	s.mu.Unlock()

	resp := s.handler(&req)
	s.writeChatCompletionsResponse(w, &req, resp)
}

func (s *openAIServer) handleResponses(w http.ResponseWriter, r *http.Request) {
	var req OpenAIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.Request = r

	s.mu.Lock()
	s.request = &req
	s.mu.Unlock()

	// Enforce the Responses API chain-mode contract: when a
	// request sets previous_response_id to a stored response that
	// emitted function_calls, the new request's input must
	// include a function_call_output for each of those call IDs.
	// OpenAI returns HTTP 400 with
	//   "No tool output found for function call call_..."
	// when this contract is violated.
	if errResp := s.validateChainModeContract(&req); errResp != nil {
		writeErrorResponse(s.t, w, errResp)
		return
	}

	resp := s.handler(&req)
	s.writeResponsesAPIResponse(w, &req, resp)
}

// validateChainModeContract checks whether a Responses API request
// that chains via previous_response_id provides function_call_output
// items for every unanswered function_call in the referenced
// response. When a tool output is missing it returns an
// ErrorResponse that mirrors the shape OpenAI serves in production.
func (s *openAIServer) validateChainModeContract(req *OpenAIRequest) *ErrorResponse {
	if req.PreviousResponseID == nil || *req.PreviousResponseID == "" {
		return nil
	}

	s.mu.Lock()
	stored, ok := s.storedResponses[*req.PreviousResponseID]
	s.mu.Unlock()
	if !ok || stored == nil || len(stored.pendingFunctionCalls) == 0 {
		return nil
	}

	providedOutputs := functionCallOutputIDs(req.Input)
	for callID := range stored.pendingFunctionCalls {
		if _, provided := providedOutputs[callID]; !provided {
			return &ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Type:       "invalid_request_error",
				Message:    fmt.Sprintf("No tool output found for function call %s.", callID),
			}
		}
	}
	return nil
}

// registerPendingFunctionCall records a function_call that was
// emitted as part of the given response. Subsequent requests chained
// to this response must provide a matching function_call_output or
// the server will reject the chain per
// openAIServer.validateChainModeContract. markFunctionCallAnswered
// is called when a follow-up request satisfies the call.
func (s *openAIServer) registerPendingFunctionCall(responseID, callID string) {
	if responseID == "" || callID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.storedResponses[responseID]
	if !ok {
		state = &openAIResponsesStoredState{
			pendingFunctionCalls: make(map[string]struct{}),
		}
		s.storedResponses[responseID] = state
	}
	state.pendingFunctionCalls[callID] = struct{}{}
}

// markFunctionCallAnswered clears a function_call from the pending
// set of a stored response when a follow-up request supplies the
// matching function_call_output. OpenAI resolves this transitively
// along the chain, so once an output is posted the call never
// reappears as pending on subsequent turns.
func (s *openAIServer) markFunctionCallAnswered(responseID, callID string) {
	if responseID == "" || callID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.storedResponses[responseID]
	if !ok || state == nil {
		return
	}
	delete(state.pendingFunctionCalls, callID)
}

// functionCallOutputIDs returns the set of call_id values carried by
// function_call_output entries in the request input. The input is
// heterogeneous so each item is decoded individually.
func functionCallOutputIDs(input []json.RawMessage) map[string]struct{} {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(input))
	for _, raw := range input {
		var item struct {
			Type   string `json:"type"`
			CallID string `json:"call_id"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		if item.Type != "function_call_output" || item.CallID == "" {
			continue
		}
		out[item.CallID] = struct{}{}
	}
	return out
}

// storeEnabledForRequest reports whether the request opted into the
// Responses API's store=true mode that causes OpenAI to persist the
// emitted response server-side.
func storeEnabledForRequest(req *OpenAIRequest) bool {
	return req != nil && req.Store != nil && *req.Store
}

func (s *openAIServer) writeChatCompletionsResponse(w http.ResponseWriter, req *OpenAIRequest, resp OpenAIResponse) {
	if resp.Error != nil {
		writeErrorResponse(s.t, w, resp.Error)
		return
	}

	hasStreaming := resp.StreamingChunks != nil
	hasNonStreaming := resp.Response != nil

	switch {
	case hasStreaming && hasNonStreaming:
		http.Error(w, "handler returned both streaming and non-streaming responses", http.StatusInternalServerError)
		return
	case !hasStreaming && !hasNonStreaming:
		http.Error(w, "handler returned empty response", http.StatusInternalServerError)
		return
	case req.Stream && !hasStreaming:
		http.Error(w, "handler returned non-streaming response for streaming request", http.StatusInternalServerError)
		return
	case !req.Stream && !hasNonStreaming:
		http.Error(w, "handler returned streaming response for non-streaming request", http.StatusInternalServerError)
		return
	case hasStreaming:
		writeChatCompletionsStreaming(w, req.Request, resp.StreamingChunks)
	default:
		s.writeChatCompletionsNonStreaming(w, resp.Response)
	}
}

func (s *openAIServer) writeResponsesAPIResponse(w http.ResponseWriter, req *OpenAIRequest, resp OpenAIResponse) {
	if resp.Error != nil {
		writeErrorResponse(s.t, w, resp.Error)
		return
	}

	hasStreaming := resp.StreamingChunks != nil
	hasNonStreaming := resp.Response != nil

	switch {
	case hasStreaming && hasNonStreaming:
		http.Error(w, "handler returned both streaming and non-streaming responses", http.StatusInternalServerError)
		return
	case !hasStreaming && !hasNonStreaming:
		http.Error(w, "handler returned empty response", http.StatusInternalServerError)
		return
	case req.Stream && !hasStreaming:
		http.Error(w, "handler returned non-streaming response for streaming request", http.StatusInternalServerError)
		return
	case !req.Stream && !hasNonStreaming:
		http.Error(w, "handler returned streaming response for non-streaming request", http.StatusInternalServerError)
		return
	case hasStreaming:
		s.writeResponsesAPIStreaming(w, req.Request, req, resp)
	default:
		s.writeResponsesAPINonStreaming(w, resp.Response)
	}
}

func writeChatCompletionsStreaming(w http.ResponseWriter, r *http.Request, chunks <-chan OpenAIChunk) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	for {
		var chunk OpenAIChunk
		var ok bool
		select {
		case <-r.Context().Done():
			log.Printf("writeChatCompletionsStreaming: request context canceled, stopping stream")
			return
		case chunk, ok = <-chunks:
			if !ok {
				_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}
		}

		choicesData := make([]map[string]interface{}, len(chunk.Choices))
		for i, choice := range chunk.Choices {
			choiceData := map[string]interface{}{
				"index": choice.Index,
			}
			if choice.Delta != "" {
				choiceData["delta"] = map[string]interface{}{
					"content": choice.Delta,
				}
			}
			if len(choice.ToolCalls) > 0 {
				// Tool calls come in the delta
				if choiceData["delta"] == nil {
					choiceData["delta"] = make(map[string]interface{})
				}
				delta, ok := choiceData["delta"].(map[string]interface{})
				if !ok {
					delta = make(map[string]interface{})
					choiceData["delta"] = delta
				}
				delta["tool_calls"] = choice.ToolCalls
			}
			if choice.FinishReason != "" {
				choiceData["finish_reason"] = choice.FinishReason
			}
			choicesData[i] = choiceData
		}

		chunkData := map[string]interface{}{
			"id":      chunk.ID,
			"object":  chunk.Object,
			"created": chunk.Created,
			"model":   chunk.Model,
			"choices": choicesData,
		}

		chunkBytes, err := json.Marshal(chunkData)
		if err != nil {
			return
		}

		if _, err := fmt.Fprintf(w, "data: %s\n\n", chunkBytes); err != nil {
			return
		}
		flusher.Flush()
	}
}

func writeNamedSSEEvent(w http.ResponseWriter, eventType string, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", eventType); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

func (s *openAIServer) writeResponsesAPIStreaming(w http.ResponseWriter, r *http.Request, req *OpenAIRequest, resp OpenAIResponse) {
	t := s.t
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	responseID := resp.ResponseID
	if responseID == "" {
		responseID = fmt.Sprintf("resp_%s", uuid.New().String()[:8])
	}
	responseModel := "gpt-4"
	sequenceNumber := int64(0)
	textOffset := 0
	itemIDs := make(map[int]string)
	itemTexts := make(map[int]string)

	writeEvent := func(eventType string, payload map[string]interface{}) bool {
		payload["type"] = eventType
		payload["sequence_number"] = sequenceNumber
		sequenceNumber++
		if err := writeNamedSSEEvent(w, eventType, payload); err != nil {
			t.Logf("writeResponsesAPIStreaming: failed to write %s: %v", eventType, err)
			return false
		}
		flusher.Flush()
		return true
	}

	if !writeEvent("response.created", map[string]interface{}{
		"response": map[string]interface{}{
			"id":     responseID,
			"object": "response",
			"model":  responseModel,
			"status": "in_progress",
			"output": []interface{}{},
		},
	}) {
		return
	}

	if resp.Reasoning != nil {
		outputIndex := textOffset
		reasoningID := resp.Reasoning.ID
		if reasoningID == "" {
			reasoningID = fmt.Sprintf("rs_%s", uuid.New().String()[:8])
		}
		summary := resp.Reasoning.Summary
		encryptedContent := resp.Reasoning.EncryptedContent
		if encryptedContent == "" {
			encryptedContent = "encrypted_data_here"
		}

		if !writeEvent("response.output_item.added", map[string]interface{}{
			"output_index": outputIndex,
			"item": map[string]interface{}{
				"type":              "reasoning",
				"id":                reasoningID,
				"summary":           []interface{}{},
				"encrypted_content": "",
			},
		}) {
			return
		}

		if summary != "" {
			if !writeEvent("response.reasoning_summary_part.added", map[string]interface{}{
				"item_id":       reasoningID,
				"output_index":  outputIndex,
				"summary_index": 0,
				"part": map[string]interface{}{
					"type": "summary_text",
					"text": "",
				},
			}) {
				return
			}
			if !writeEvent("response.reasoning_summary_text.added", map[string]interface{}{
				"item_id":       reasoningID,
				"output_index":  outputIndex,
				"summary_index": 0,
			}) {
				return
			}
			if !writeEvent("response.reasoning_summary_text.delta", map[string]interface{}{
				"item_id":       reasoningID,
				"output_index":  outputIndex,
				"summary_index": 0,
				"delta":         summary,
			}) {
				return
			}
			if !writeEvent("response.reasoning_summary_text.done", map[string]interface{}{
				"item_id":       reasoningID,
				"output_index":  outputIndex,
				"summary_index": 0,
				"text":          summary,
			}) {
				return
			}
			if !writeEvent("response.reasoning_summary_part.done", map[string]interface{}{
				"item_id":       reasoningID,
				"output_index":  outputIndex,
				"summary_index": 0,
				"part": map[string]interface{}{
					"type": "summary_text",
					"text": summary,
				},
			}) {
				return
			}
		}

		summaryItems := []interface{}{}
		if summary != "" {
			summaryItems = append(summaryItems, map[string]interface{}{
				"type": "summary_text",
				"text": summary,
			})
		}
		if !writeEvent("response.output_item.done", map[string]interface{}{
			"output_index": outputIndex,
			"item": map[string]interface{}{
				"type":              "reasoning",
				"id":                reasoningID,
				"summary":           summaryItems,
				"encrypted_content": encryptedContent,
			},
		}) {
			return
		}
		textOffset++
	}

	if resp.WebSearch != nil {
		outputIndex := textOffset
		itemID := resp.WebSearch.ID
		if itemID == "" {
			itemID = fmt.Sprintf("ws_%s", uuid.New().String()[:8])
		}
		query := resp.WebSearch.Query
		if query == "" {
			query = "latest AI news"
		}

		if !writeEvent("response.output_item.added", map[string]interface{}{
			"output_index": outputIndex,
			"item": map[string]interface{}{
				"type":   "web_search_call",
				"id":     itemID,
				"status": "in_progress",
			},
		}) {
			return
		}
		if !writeEvent("response.output_item.done", map[string]interface{}{
			"output_index": outputIndex,
			"item": map[string]interface{}{
				"type":   "web_search_call",
				"id":     itemID,
				"status": "completed",
				"action": map[string]interface{}{
					"type":  "search",
					"query": query,
				},
			},
		}) {
			return
		}
		textOffset++
	}

	// Emit any configured function_call items. When the
	// referencing request set store=true, register them on the
	// server's stored-responses map so a follow-up request with
	// previous_response_id must include a matching
	// function_call_output for each call.
	for _, fc := range resp.FunctionCalls {
		outputIndex := textOffset
		itemID := fc.ItemID
		if itemID == "" {
			itemID = fmt.Sprintf("fc_%s", uuid.New().String()[:8])
		}
		callID := fc.CallID
		if callID == "" {
			callID = fmt.Sprintf("call_%s", uuid.New().String()[:12])
		}
		if !writeEvent("response.output_item.added", map[string]interface{}{
			"output_index": outputIndex,
			"item": map[string]interface{}{
				"type":      "function_call",
				"id":        itemID,
				"status":    "in_progress",
				"call_id":   callID,
				"name":      fc.Name,
				"arguments": "",
			},
		}) {
			return
		}
		if fc.Arguments != "" {
			if !writeEvent("response.function_call_arguments.delta", map[string]interface{}{
				"item_id":      itemID,
				"output_index": outputIndex,
				"delta":        fc.Arguments,
			}) {
				return
			}
		}
		if !writeEvent("response.function_call_arguments.done", map[string]interface{}{
			"item_id":      itemID,
			"output_index": outputIndex,
			"arguments":    fc.Arguments,
		}) {
			return
		}
		if !writeEvent("response.output_item.done", map[string]interface{}{
			"output_index": outputIndex,
			"item": map[string]interface{}{
				"type":      "function_call",
				"id":        itemID,
				"status":    "completed",
				"call_id":   callID,
				"name":      fc.Name,
				"arguments": fc.Arguments,
			},
		}) {
			return
		}
		if s != nil && storeEnabledForRequest(req) {
			s.registerPendingFunctionCall(responseID, callID)
		}
		textOffset++
	}

	// When the follow-up request satisfies function_calls from a
	// prior response, mark them answered so their stored state
	// stops complaining on further chains.
	if s != nil && req != nil && req.PreviousResponseID != nil && *req.PreviousResponseID != "" {
		for callID := range functionCallOutputIDs(req.Input) {
			s.markFunctionCallAnswered(*req.PreviousResponseID, callID)
		}
	}

	for {
		var chunk OpenAIChunk
		var ok bool
		select {
		case <-r.Context().Done():
			log.Printf("writeResponsesAPIStreaming: request context canceled, stopping stream")
			return
		case chunk, ok = <-resp.StreamingChunks:
			if !ok {
				indices := make([]int, 0, len(itemIDs))
				for outputIndex := range itemIDs {
					indices = append(indices, outputIndex)
				}
				sort.Ints(indices)
				for _, outputIndex := range indices {
					itemID := itemIDs[outputIndex]
					text := itemTexts[outputIndex]
					if !writeEvent("response.output_text.done", map[string]interface{}{
						"item_id":       itemID,
						"output_index":  outputIndex,
						"content_index": 0,
						"text":          text,
						"logprobs":      []interface{}{},
					}) {
						return
					}
					if !writeEvent("response.content_part.done", map[string]interface{}{
						"item_id":       itemID,
						"output_index":  outputIndex,
						"content_index": 0,
						"part": map[string]interface{}{
							"type": "output_text",
							"text": text,
						},
					}) {
						return
					}
					if !writeEvent("response.output_item.done", map[string]interface{}{
						"output_index": outputIndex,
						"item": map[string]interface{}{
							"type":   "message",
							"id":     itemID,
							"role":   "assistant",
							"status": "completed",
							"content": []interface{}{
								map[string]interface{}{
									"type": "output_text",
									"text": text,
								},
							},
						},
					}) {
						return
					}
				}
				if !writeEvent("response.completed", map[string]interface{}{
					"response": map[string]interface{}{
						"id":     responseID,
						"object": "response",
						"model":  responseModel,
						"status": "completed",
						"output": []interface{}{},
						"usage":  map[string]interface{}{},
					},
				}) {
					return
				}
				return
			}
		}

		if chunk.Model != "" {
			responseModel = chunk.Model
		}

		for outputIndex, choice := range chunk.Choices {
			if choice.Index != 0 {
				outputIndex = choice.Index
			}
			outputIndex += textOffset
			itemID, found := itemIDs[outputIndex]
			if !found {
				itemID = fmt.Sprintf("msg_%s", uuid.New().String()[:8])
				itemIDs[outputIndex] = itemID
				if !writeEvent("response.output_item.added", map[string]interface{}{
					"output_index": outputIndex,
					"item": map[string]interface{}{
						"type":    "message",
						"id":      itemID,
						"role":    "assistant",
						"status":  "in_progress",
						"content": []interface{}{},
					},
				}) {
					return
				}
				if !writeEvent("response.content_part.added", map[string]interface{}{
					"item_id":       itemID,
					"output_index":  outputIndex,
					"content_index": 0,
					"part": map[string]interface{}{
						"type": "output_text",
						"text": "",
					},
				}) {
					return
				}
			}

			itemTexts[outputIndex] += choice.Delta
			if !writeEvent("response.output_text.delta", map[string]interface{}{
				"item_id":       itemID,
				"output_index":  outputIndex,
				"content_index": 0,
				"delta":         choice.Delta,
			}) {
				return
			}
		}
	}
}

func (s *openAIServer) writeChatCompletionsNonStreaming(w http.ResponseWriter, resp *OpenAICompletion) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.t.Errorf("writeChatCompletionsNonStreaming: failed to encode response: %v", err)
	}
}

func (s *openAIServer) writeResponsesAPINonStreaming(w http.ResponseWriter, resp *OpenAICompletion) {
	// Convert all choices to output format
	outputs := make([]map[string]interface{}, len(resp.Choices))
	for i, choice := range resp.Choices {
		outputs[i] = map[string]interface{}{
			"id":   uuid.New().String(),
			"type": "message",
			"role": "assistant",
			"content": []map[string]interface{}{
				{
					"type": "output_text",
					"text": choice.Message.Content,
				},
			},
		}
	}

	response := map[string]interface{}{
		"id":      resp.ID,
		"object":  "response",
		"created": resp.Created,
		"model":   resp.Model,
		"output":  outputs,
		"usage": map[string]interface{}{
			"input_tokens":  resp.Usage.PromptTokens,
			"output_tokens": resp.Usage.CompletionTokens,
			"total_tokens":  resp.Usage.TotalTokens,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.t.Errorf("writeResponsesAPINonStreaming: failed to encode response: %v", err)
	}
}

// OpenAIStreamingResponse creates a streaming response from chunks.
func OpenAIStreamingResponse(chunks ...OpenAIChunk) OpenAIResponse {
	ch := make(chan OpenAIChunk, len(chunks))
	go func() {
		for _, chunk := range chunks {
			ch <- chunk
		}
		close(ch)
	}()
	return OpenAIResponse{StreamingChunks: ch}
}

// OpenAINonStreamingResponse creates a non-streaming response with the given text.
func OpenAINonStreamingResponse(text string) OpenAIResponse {
	return OpenAIResponse{
		Response: &OpenAICompletion{
			ID:      fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:8]),
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4",
			Choices: []OpenAICompletionChoice{
				{
					Index: 0,
					Message: OpenAIMessage{
						Role:    "assistant",
						Content: text,
					},
					FinishReason: "stop",
				},
			},
			Usage: OpenAICompletionUsage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		},
	}
}

// OpenAITextChunks creates streaming chunks with text deltas.
// Each delta string becomes a separate chunk with a single choice.
// Returns a slice of chunks, one per delta, with each choice having its index (0, 1, 2, ...).
func OpenAITextChunks(deltas ...string) []OpenAIChunk {
	if len(deltas) == 0 {
		return nil
	}

	chunkID := fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:8])
	now := time.Now().Unix()
	chunks := make([]OpenAIChunk, len(deltas))

	for i, delta := range deltas {
		chunks[i] = OpenAIChunk{
			ID:      chunkID,
			Object:  "chat.completion.chunk",
			Created: now,
			Model:   "gpt-4",
			Choices: []OpenAIChunkChoice{
				{
					Index: i,
					Delta: delta,
				},
			},
		}
	}

	return chunks
}

// OpenAIToolCallChunk creates a streaming chunk with a tool call.
// Takes the tool name and arguments JSON string, creates a tool call for choice index 0.
func OpenAIToolCallChunk(toolName, arguments string) OpenAIChunk {
	return OpenAIChunk{
		ID:      fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:8]),
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   "gpt-4",
		Choices: []OpenAIChunkChoice{
			{
				Index: 0,
				ToolCalls: []OpenAIToolCall{
					{
						Index: 0,
						ID:    fmt.Sprintf("call_%s", uuid.New().String()[:8]),
						Type:  "function",
						Function: OpenAIToolCallFunction{
							Name:      toolName,
							Arguments: arguments,
						},
					},
				},
			},
		},
	}
}
