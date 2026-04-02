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

// OpenAIRequest represents an OpenAI chat completion request.
type OpenAIRequest struct {
	*http.Request
	Model              string          `json:"model"`
	Messages           []OpenAIMessage `json:"messages"`
	Stream             bool            `json:"stream,omitempty"`
	Tools              []OpenAITool    `json:"tools,omitempty"`
	Prompt             []interface{}   `json:"prompt,omitempty"` // For responses API
	Store              *bool           `json:"store,omitempty"`
	PreviousResponseID *string         `json:"previous_response_id,omitempty"`
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
type openAIServer struct {
	mu      sync.Mutex
	t       testing.TB
	server  *httptest.Server
	handler OpenAIHandler
	request *OpenAIRequest
}

// NewOpenAI creates a new OpenAI test server with a handler function.
// The handler is called for each request and should return either a streaming
// response (via channel) or a non-streaming response.
// Returns the base URL of the server.
func NewOpenAI(t testing.TB, handler OpenAIHandler) string {
	t.Helper()

	s := &openAIServer{
		t:       t,
		handler: handler,
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

	resp := s.handler(&req)
	s.writeResponsesAPIResponse(w, &req, resp)
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
		writeResponsesAPIStreaming(s.t, w, req.Request, resp)
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

func writeResponsesAPIStreaming(t testing.TB, w http.ResponseWriter, r *http.Request, resp OpenAIResponse) {
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
