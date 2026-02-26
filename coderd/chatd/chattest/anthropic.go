package chattest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// AnthropicHandler handles Anthropic API requests and returns a response.
type AnthropicHandler func(req *AnthropicRequest) AnthropicResponse

// AnthropicResponse represents a response to an Anthropic request.
// Either StreamingChunks or Response should be set, not both.
type AnthropicResponse struct {
	StreamingChunks <-chan AnthropicChunk
	Response        *AnthropicMessage
}

// AnthropicRequest represents an Anthropic messages request.
type AnthropicRequest struct {
	*http.Request                           // Embed http.Request
	Model         string                    `json:"model"`
	Messages      []AnthropicRequestMessage `json:"messages"`
	Stream        bool                      `json:"stream,omitempty"`
	MaxTokens     int                       `json:"max_tokens,omitempty"`
	// TODO: encoding/json ignores inline tags. Add custom UnmarshalJSON to capture unknown keys.
	Options map[string]interface{} `json:",inline"` //nolint:revive
}

// AnthropicRequestMessage represents a message in an Anthropic request.
// Content may be either a string or a structured content array.
type AnthropicRequestMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// AnthropicMessage represents a message in an Anthropic response.
type AnthropicMessage struct {
	ID         string         `json:"id,omitempty"`
	Type       string         `json:"type,omitempty"`
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	Model      string         `json:"model,omitempty"`
	StopReason string         `json:"stop_reason,omitempty"`
	Usage      AnthropicUsage `json:"usage,omitempty"`
}

// AnthropicUsage represents usage information in an Anthropic response.
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AnthropicChunk represents a streaming chunk from Anthropic.
type AnthropicChunk struct {
	Type         string                `json:"type"`
	Index        int                   `json:"index,omitempty"`
	Message      AnthropicChunkMessage `json:"message,omitempty"`
	ContentBlock AnthropicContentBlock `json:"content_block,omitempty"`
	Delta        AnthropicDeltaBlock   `json:"delta,omitempty"`
	StopReason   string                `json:"stop_reason,omitempty"`
	StopSequence *string               `json:"stop_sequence,omitempty"`
	Usage        AnthropicUsage        `json:"usage,omitempty"`
}

// AnthropicChunkMessage represents message metadata in a chunk.
type AnthropicChunkMessage struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Role  string `json:"role"`
	Model string `json:"model"`
}

// AnthropicContentBlock represents a content block in a chunk.
type AnthropicContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// AnthropicDeltaBlock represents a delta block in a chunk.
type AnthropicDeltaBlock struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

// anthropicServer is a test server that mocks the Anthropic API.
type anthropicServer struct {
	mu      sync.Mutex
	server  *httptest.Server
	handler AnthropicHandler
	request *AnthropicRequest
}

// NewAnthropic creates a new Anthropic test server with a handler function.
// The handler is called for each request and should return either a streaming
// response (via channel) or a non-streaming response.
// Returns the base URL of the server.
func NewAnthropic(t testing.TB, handler AnthropicHandler) string {
	t.Helper()

	s := &anthropicServer{
		handler: handler,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/messages", s.handleMessages)

	s.server = httptest.NewServer(mux)

	t.Cleanup(func() {
		s.server.Close()
	})

	return s.server.URL
}

func (s *anthropicServer) handleMessages(w http.ResponseWriter, r *http.Request) {
	var req AnthropicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Return a more detailed error for debugging
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}
	req.Request = r // Embed the original http.Request

	s.mu.Lock()
	s.request = &req
	s.mu.Unlock()

	resp := s.handler(&req)
	s.writeResponse(w, &req, resp)
}

func (s *anthropicServer) writeResponse(w http.ResponseWriter, req *AnthropicRequest, resp AnthropicResponse) {
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
		s.writeStreamingResponse(w, resp.StreamingChunks)
	default:
		s.writeNonStreamingResponse(w, resp.Response)
	}
}

func (s *anthropicServer) writeStreamingResponse(w http.ResponseWriter, chunks <-chan AnthropicChunk) {
	_ = s // receiver unused but kept for consistency
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("anthropic-version", "2023-06-01")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	for chunk := range chunks {
		chunkData := make(map[string]interface{})
		chunkData["type"] = chunk.Type

		switch chunk.Type {
		case "message_start":
			chunkData["message"] = chunk.Message
		case "content_block_start":
			chunkData["index"] = chunk.Index
			chunkData["content_block"] = chunk.ContentBlock
		case "content_block_delta":
			chunkData["index"] = chunk.Index
			chunkData["delta"] = chunk.Delta
		case "content_block_stop":
			chunkData["index"] = chunk.Index
		case "message_delta":
			chunkData["delta"] = map[string]interface{}{
				"stop_reason":   chunk.StopReason,
				"stop_sequence": chunk.StopSequence,
			}
			chunkData["usage"] = chunk.Usage
		case "message_stop":
			// No additional fields
		}

		chunkBytes, err := json.Marshal(chunkData)
		if err != nil {
			return
		}

		// Send both event and data lines to match Anthropic API format
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", chunk.Type, chunkBytes); err != nil {
			return
		}
		flusher.Flush()
	}
}

func (s *anthropicServer) writeNonStreamingResponse(w http.ResponseWriter, resp *AnthropicMessage) {
	_ = s // receiver unused but kept for consistency
	response := map[string]interface{}{
		"id":    resp.ID,
		"type":  resp.Type,
		"role":  resp.Role,
		"model": resp.Model,
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": resp.Content,
			},
		},
		"stop_reason": resp.StopReason,
		"usage":       resp.Usage,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("anthropic-version", "2023-06-01")
	_ = json.NewEncoder(w).Encode(response)
}

// AnthropicStreamingResponse creates a streaming response from chunks.
func AnthropicStreamingResponse(chunks ...AnthropicChunk) AnthropicResponse {
	ch := make(chan AnthropicChunk, len(chunks))
	go func() {
		for _, chunk := range chunks {
			ch <- chunk
		}
		close(ch)
	}()
	return AnthropicResponse{StreamingChunks: ch}
}

// AnthropicNonStreamingResponse creates a non-streaming response with the given text.
func AnthropicNonStreamingResponse(text string) AnthropicResponse {
	return AnthropicResponse{
		Response: &AnthropicMessage{
			ID:         fmt.Sprintf("msg-%s", uuid.New().String()[:8]),
			Type:       "message",
			Role:       "assistant",
			Content:    text,
			Model:      "claude-3-opus-20240229",
			StopReason: "end_turn",
			Usage: AnthropicUsage{
				InputTokens:  10,
				OutputTokens: 5,
			},
		},
	}
}

// AnthropicTextChunks creates a complete streaming response with text deltas.
// Takes text deltas and creates all required chunks (message_start,
// content_block_start, content_block_delta for each delta,
// content_block_stop, message_delta, message_stop).
func AnthropicTextChunks(deltas ...string) []AnthropicChunk {
	if len(deltas) == 0 {
		return nil
	}

	messageID := fmt.Sprintf("msg-%s", uuid.New().String()[:8])
	model := "claude-3-opus-20240229"

	chunks := []AnthropicChunk{
		{
			Type: "message_start",
			Message: AnthropicChunkMessage{
				ID:    messageID,
				Type:  "message",
				Role:  "assistant",
				Model: model,
			},
		},
		{
			Type:  "content_block_start",
			Index: 0,
			ContentBlock: AnthropicContentBlock{
				Type: "text",
				Text: "", // According to Anthropic API spec, text should be empty in content_block_start
			},
		},
	}

	// Add a delta chunk for each delta
	for _, delta := range deltas {
		chunks = append(chunks, AnthropicChunk{
			Type:  "content_block_delta",
			Index: 0,
			Delta: AnthropicDeltaBlock{
				Type: "text_delta",
				Text: delta,
			},
		})
	}

	chunks = append(chunks,
		AnthropicChunk{
			Type:  "content_block_stop",
			Index: 0,
		},
		AnthropicChunk{
			Type:       "message_delta",
			StopReason: "end_turn",
			Usage: AnthropicUsage{
				InputTokens:  10,
				OutputTokens: 5,
			},
		},
		AnthropicChunk{
			Type: "message_stop",
		},
	)

	return chunks
}

// AnthropicToolCallChunks creates a complete streaming response for a tool call.
// Input JSON can be split across multiple deltas, matching Anthropic's
// input_json_delta streaming behavior.
func AnthropicToolCallChunks(toolName string, inputJSONDeltas ...string) []AnthropicChunk {
	if len(inputJSONDeltas) == 0 {
		return nil
	}
	if toolName == "" {
		toolName = "tool"
	}

	messageID := fmt.Sprintf("msg-%s", uuid.New().String()[:8])
	model := "claude-3-opus-20240229"
	toolCallID := fmt.Sprintf("toolu_%s", uuid.New().String()[:8])

	chunks := []AnthropicChunk{
		{
			Type: "message_start",
			Message: AnthropicChunkMessage{
				ID:    messageID,
				Type:  "message",
				Role:  "assistant",
				Model: model,
			},
		},
		{
			Type:  "content_block_start",
			Index: 0,
			ContentBlock: AnthropicContentBlock{
				Type:  "tool_use",
				ID:    toolCallID,
				Name:  toolName,
				Input: json.RawMessage("{}"),
			},
		},
	}

	for _, delta := range inputJSONDeltas {
		chunks = append(chunks, AnthropicChunk{
			Type:  "content_block_delta",
			Index: 0,
			Delta: AnthropicDeltaBlock{
				Type:        "input_json_delta",
				PartialJSON: delta,
			},
		})
	}

	chunks = append(chunks,
		AnthropicChunk{
			Type:  "content_block_stop",
			Index: 0,
		},
		AnthropicChunk{
			Type:       "message_delta",
			StopReason: "tool_use",
			Usage: AnthropicUsage{
				InputTokens:  10,
				OutputTokens: 5,
			},
		},
		AnthropicChunk{
			Type: "message_stop",
		},
	)

	return chunks
}
