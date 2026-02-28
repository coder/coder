package chattest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	Error           *ErrorResponse // If set, server returns this HTTP error instead of streaming/JSON.
}

// OpenAIRequest represents an OpenAI chat completion request.
type OpenAIRequest struct {
	*http.Request
	Model    string          `json:"model"`
	Messages []OpenAIMessage `json:"messages"`
	Stream   bool            `json:"stream,omitempty"`
	Prompt   []interface{}   `json:"prompt,omitempty"` // For responses API
	// TODO: encoding/json ignores inline tags. Add custom UnmarshalJSON to capture unknown keys.
	Options map[string]interface{} `json:",inline"` //nolint:revive
}

// OpenAIMessage represents a message in an OpenAI request.
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
		writeErrorResponse(w, resp.Error)
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
		s.writeChatCompletionsStreaming(w, resp.StreamingChunks)
	default:
		s.writeChatCompletionsNonStreaming(w, resp.Response)
	}
}

func (s *openAIServer) writeResponsesAPIResponse(w http.ResponseWriter, req *OpenAIRequest, resp OpenAIResponse) {
	if resp.Error != nil {
		writeErrorResponse(w, resp.Error)
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
		s.writeResponsesAPIStreaming(w, resp.StreamingChunks)
	default:
		s.writeResponsesAPINonStreaming(w, resp.Response)
	}
}

func (s *openAIServer) writeChatCompletionsStreaming(w http.ResponseWriter, chunks <-chan OpenAIChunk) {
	_ = s // receiver unused but kept for consistency
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	for chunk := range chunks {
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

	_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (s *openAIServer) writeResponsesAPIStreaming(w http.ResponseWriter, chunks <-chan OpenAIChunk) {
	_ = s // receiver unused but kept for consistency
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	itemIDs := make(map[int]string)

	for chunk := range chunks {
		// Responses API sends one event per choice
		for outputIndex, choice := range chunk.Choices {
			if choice.Index != 0 {
				outputIndex = choice.Index
			}
			itemID, found := itemIDs[outputIndex]
			if !found {
				itemID = fmt.Sprintf("msg_%s", uuid.New().String()[:8])
				itemIDs[outputIndex] = itemID
			}

			chunkData := map[string]interface{}{
				"type":          "response.output_text.delta",
				"item_id":       itemID,
				"output_index":  outputIndex,
				"created":       chunk.Created,
				"model":         chunk.Model,
				"content_index": 0,
				"delta":         choice.Delta,
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

	_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (s *openAIServer) writeChatCompletionsNonStreaming(w http.ResponseWriter, resp *OpenAICompletion) {
	_ = s // receiver unused but kept for consistency
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *openAIServer) writeResponsesAPINonStreaming(w http.ResponseWriter, resp *OpenAICompletion) {
	_ = s // receiver unused but kept for consistency
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
		"usage":   resp.Usage,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
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
