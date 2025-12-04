package llmmock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

// Server wraps the LLM mock server and provides an HTTP API to retrieve requests.
type Server struct {
	httpServer   *http.Server
	httpListener net.Listener
	logger       slog.Logger

	hostAddress string
	apiPort     int

	// Storage for intercepted requests
	records   []RequestRecord
	recordsMu sync.RWMutex
}

type Config struct {
	HostAddress string
	APIPort     int
	Logger      slog.Logger
}

type openAIRequest struct {
	Model    string                 `json:"model"`
	Messages []openAIMessage        `json:"messages"`
	Stream   bool                   `json:"stream,omitempty"`
	Extra    map[string]interface{} `json:"-"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int           `json:"index"`
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type anthropicRequest struct {
	Model     string                 `json:"model"`
	Messages  []anthropicMessage     `json:"messages"`
	Stream    bool                   `json:"stream,omitempty"`
	MaxTokens int                    `json:"max_tokens"`
	Extra     map[string]interface{} `json:"-"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model        string  `json:"model"`
	StopReason   string  `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (s *Server) Start(ctx context.Context, cfg Config) error {
	s.hostAddress = cfg.HostAddress
	s.apiPort = cfg.APIPort
	s.logger = cfg.Logger

	if err := s.startAPIServer(ctx); err != nil {
		return xerrors.Errorf("start API server: %w", err)
	}

	return nil
}

func (s *Server) Stop() error {
	if s.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			return xerrors.Errorf("shutdown HTTP server: %w", err)
		}
	}
	return nil
}

func (s *Server) APIAddress() string {
	return fmt.Sprintf("http://%s:%d", s.hostAddress, s.apiPort)
}

func (s *Server) RequestCount() int {
	s.recordsMu.RLock()
	defer s.recordsMu.RUnlock()
	return len(s.records)
}

func (s *Server) Purge() {
	s.recordsMu.Lock()
	defer s.recordsMu.Unlock()
	s.records = s.records[:0]
}

func (s *Server) startAPIServer(ctx context.Context) error {
	mux := http.NewServeMux()

	// LLM API endpoints
	mux.HandleFunc("POST /v1/chat/completions", s.handleOpenAI)
	mux.HandleFunc("POST /v1/messages", s.handleAnthropic)

	// Query API endpoints
	mux.HandleFunc("GET /api/requests", s.handleGetRequests)
	mux.HandleFunc("POST /api/purge", s.handlePurge)

	s.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.hostAddress, s.apiPort))
	if err != nil {
		return xerrors.Errorf("listen on %s:%d: %w", s.hostAddress, s.apiPort, err)
	}
	s.httpListener = listener

	tcpAddr, valid := listener.Addr().(*net.TCPAddr)
	if !valid {
		err := listener.Close()
		if err != nil {
			s.logger.Error(ctx, "failed to close listener", slog.Error(err))
		}
		return xerrors.Errorf("listener returned invalid address: %T", listener.Addr())
	}
	s.apiPort = tcpAddr.Port

	go func() {
		if err := s.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error(ctx, "http API server error", slog.Error(err))
		}
	}()

	return nil
}

func (s *Server) handleOpenAI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := uuid.New()
	now := time.Now()

	// Read request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error(ctx, "failed to read request body", slog.Error(err))
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse request
	var req openAIRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		s.logger.Error(ctx, "failed to parse OpenAI request", slog.Error(err))
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Extract user ID from Authorization header if present
	userID := s.extractUserID(r)

	// Store request
	requestSummary := RequestSummary{
		ID:          requestID,
		Timestamp:   now,
		Provider:    ProviderOpenAI,
		Model:       req.Model,
		UserID:      userID,
		Stream:      req.Stream,
		RequestBody: string(bodyBytes),
	}

	// Generate mock response
	var resp openAIResponse
	resp.ID = fmt.Sprintf("chatcmpl-%s", requestID.String()[:8])
	resp.Object = "chat.completion"
	resp.Created = now.Unix()
	resp.Model = req.Model
	resp.Choices = []struct {
		Index        int           `json:"index"`
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	}{
		{
			Index: 0,
			Message: openAIMessage{
				Role:    "assistant",
				Content: "This is a mock response from OpenAI.",
			},
			FinishReason: "stop",
		},
	}
	resp.Usage.PromptTokens = 10
	resp.Usage.CompletionTokens = 5
	resp.Usage.TotalTokens = 15

	responseBody, _ := json.Marshal(resp)
	responseTime := time.Now()

	// Store response
	responseSummary := ResponseSummary{
		RequestID:    requestID,
		Timestamp:    responseTime,
		Status:       http.StatusOK,
		Stream:       req.Stream,
		FinishReason: "stop",
		PromptTokens: resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
		TotalTokens:  resp.Usage.TotalTokens,
		ResponseBody: string(responseBody),
	}

	s.recordsMu.Lock()
	s.records = append(s.records, RequestRecord{
		Request:  requestSummary,
		Response: &responseSummary,
	})
	s.recordsMu.Unlock()

	// Send response
	if req.Stream {
		s.sendOpenAIStream(w, resp)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(responseBody)
	}
}

func (s *Server) handleAnthropic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := uuid.New()
	now := time.Now()

	// Read request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error(ctx, "failed to read request body", slog.Error(err))
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse request
	var req anthropicRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		s.logger.Error(ctx, "failed to parse Anthropic request", slog.Error(err))
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Extract user ID from Authorization header if present
	userID := s.extractUserID(r)

	// Store request
	requestSummary := RequestSummary{
		ID:          requestID,
		Timestamp:   now,
		Provider:    ProviderAnthropic,
		Model:       req.Model,
		UserID:      userID,
		Stream:      req.Stream,
		RequestBody: string(bodyBytes),
	}

	// Generate mock response
	var resp anthropicResponse
	resp.ID = fmt.Sprintf("msg_%s", requestID.String()[:8])
	resp.Type = "message"
	resp.Role = "assistant"
	resp.Content = []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{
		{
			Type: "text",
			Text: "This is a mock response from Anthropic.",
		},
	}
	resp.Model = req.Model
	resp.StopReason = "end_turn"
	resp.Usage.InputTokens = 10
	resp.Usage.OutputTokens = 5

	responseBody, _ := json.Marshal(resp)
	responseTime := time.Now()

	// Store response
	responseSummary := ResponseSummary{
		RequestID:    requestID,
		Timestamp:    responseTime,
		Status:       http.StatusOK,
		Stream:       req.Stream,
		FinishReason: resp.StopReason,
		PromptTokens: resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
		TotalTokens:  resp.Usage.InputTokens + resp.Usage.OutputTokens,
		ResponseBody: string(responseBody),
	}

	s.recordsMu.Lock()
	s.records = append(s.records, RequestRecord{
		Request:  requestSummary,
		Response: &responseSummary,
	})
	s.recordsMu.Unlock()

	// Send response
	if req.Stream {
		s.sendAnthropicStream(w, resp)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("anthropic-version", "2023-06-01")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(responseBody)
	}
}

func (s *Server) sendOpenAIStream(w http.ResponseWriter, resp openAIResponse) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Send initial chunk
	chunk := map[string]interface{}{
		"id":      resp.ID,
		"object":  "chat.completion.chunk",
		"created": resp.Created,
		"model":   resp.Model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"delta": map[string]interface{}{
					"role":    "assistant",
					"content": resp.Choices[0].Message.Content,
				},
				"finish_reason": nil,
			},
		},
	}
	chunkBytes, _ := json.Marshal(chunk)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", chunkBytes)

	// Send final chunk
	finalChunk := map[string]interface{}{
		"id":      resp.ID,
		"object":  "chat.completion.chunk",
		"created": resp.Created,
		"model":   resp.Model,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"delta":         map[string]interface{}{},
				"finish_reason": resp.Choices[0].FinishReason,
			},
		},
	}
	finalChunkBytes, _ := json.Marshal(finalChunk)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", finalChunkBytes)
	_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
}

func (s *Server) sendAnthropicStream(w http.ResponseWriter, resp anthropicResponse) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("anthropic-version", "2023-06-01")
	w.WriteHeader(http.StatusOK)

	// Send message_start event
	startEvent := map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":    resp.ID,
			"type":  resp.Type,
			"role":  resp.Role,
			"model": resp.Model,
		},
	}
	startBytes, _ := json.Marshal(startEvent)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", startBytes)

	// Send content_block_start event
	contentStartEvent := map[string]interface{}{
		"type":  "content_block_start",
		"index": 0,
		"content_block": map[string]interface{}{
			"type": "text",
			"text": resp.Content[0].Text,
		},
	}
	contentStartBytes, _ := json.Marshal(contentStartEvent)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", contentStartBytes)

	// Send content_block_delta event
	deltaEvent := map[string]interface{}{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]interface{}{
			"type": "text_delta",
			"text": resp.Content[0].Text,
		},
	}
	deltaBytes, _ := json.Marshal(deltaEvent)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", deltaBytes)

	// Send content_block_stop event
	contentStopEvent := map[string]interface{}{
		"type":  "content_block_stop",
		"index": 0,
	}
	contentStopBytes, _ := json.Marshal(contentStopEvent)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", contentStopBytes)

	// Send message_delta event
	deltaMsgEvent := map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   resp.StopReason,
			"stop_sequence": resp.StopSequence,
		},
		"usage": resp.Usage,
	}
	deltaMsgBytes, _ := json.Marshal(deltaMsgEvent)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", deltaMsgBytes)

	// Send message_stop event
	stopEvent := map[string]interface{}{
		"type": "message_stop",
	}
	stopBytes, _ := json.Marshal(stopEvent)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", stopBytes)
}

func (s *Server) handleGetRequests(w http.ResponseWriter, r *http.Request) {
	s.recordsMu.RLock()
	records := slices.Clone(s.records)
	s.recordsMu.RUnlock()

	// Apply filters
	userID := r.URL.Query().Get("user_id")
	providerStr := r.URL.Query().Get("provider")

	var filtered []RequestRecord
	for _, record := range records {
		if userID != "" && record.Request.UserID != userID {
			continue
		}
		if providerStr != "" && string(record.Request.Provider) != providerStr {
			continue
		}
		filtered = append(filtered, record)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(filtered); err != nil {
		s.logger.Warn(r.Context(), "failed to encode JSON response", slog.Error(err))
	}
}

func (s *Server) handlePurge(w http.ResponseWriter, _ *http.Request) {
	s.Purge()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) extractUserID(r *http.Request) string {
	// Try to extract user ID from Authorization header
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	// For now, just return a simple identifier
	// In a real scenario, this might parse a JWT or API key
	// For scale tests, we can use the token itself or extract from it
	if strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		// Use first 8 chars as a simple identifier
		if len(token) > 8 {
			return token[:8]
		}
		return token
	}

	return ""
}
