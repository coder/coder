package llmmock

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.14.0"
	"go.opentelemetry.io/otel/semconv/v1.14.0/httpconv"
	"go.opentelemetry.io/otel/semconv/v1.14.0/netconv"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/pproflabel"
	"github.com/coder/coder/v2/coderd/tracing"
)

const (
	openAIDefaultResponseText          = "This is a mock response from OpenAI."
	openAIResponsesDefaultResponseText = "This is a mock response from OpenAI Responses."
	anthropicDefaultResponseText       = "This is a mock response from Anthropic."
	mockInputTokens                    = 10
	mockOutputTokens                   = 5
	// streamFixedWindowSize makes large payload streams predictable and not whitespace-dependent.
	streamFixedWindowSize = 10
)

func responseText(responsePayloadSize int, fallback string) string {
	if responsePayloadSize > 0 {
		return strings.Repeat("x", responsePayloadSize)
	}
	return fallback
}

// Server wraps the LLM mock server and provides an HTTP API to retrieve requests.
type Server struct {
	httpServer        *http.Server
	httpListener      net.Listener
	cancelHTTPContext context.CancelFunc
	logger            slog.Logger

	address             string
	artificialLatency   time.Duration
	responsePayloadSize int
	minStreamDuration   time.Duration
	maxStreamDuration   time.Duration
	toolCallConfig      toolCallConfig

	tracerProvider trace.TracerProvider
	closeTracing   func(context.Context) error
}

type Config struct {
	Address             string
	Logger              slog.Logger
	ArtificialLatency   time.Duration
	ResponsePayloadSize int
	MinStreamDuration   time.Duration
	MaxStreamDuration   time.Duration
	MinToolCallsPerTurn int
	MaxToolCallsPerTurn int
	ToolCallCommand     string

	PprofEnable  bool
	PprofAddress string

	TraceEnable bool
}

type llmRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages,omitempty"`
	Tools    []openAITool    `json:"tools,omitempty"`
	Stream   bool            `json:"stream,omitempty"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolFunction struct {
	Name string `json:"name"`
}

type openAIToolCall struct {
	ID       string                 `json:"id,omitempty"`
	Type     string                 `json:"type,omitempty"`
	Function openAIToolCallFunction `json:"function,omitempty"`
	Index    int                    `json:"index"`
}

type openAIToolCallFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type openAIResponseChoice struct {
	Index        int           `json:"index"`
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openAIResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []openAIResponseChoice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type responsesResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Output  []struct {
		ID      string `json:"id,omitempty"`
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"output"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
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
	if cfg.ToolCallCommand == "" {
		cfg.ToolCallCommand = defaultToolCallCommand
	}
	if err := cfg.Validate(); err != nil {
		return xerrors.Errorf("validate config: %w", err)
	}

	var seedBytes [8]byte
	if _, err := crand.Read(seedBytes[:]); err != nil {
		return xerrors.Errorf("generate tool-call seed: %w", err)
	}
	seed := binary.LittleEndian.Uint64(seedBytes[:])

	s.address = cfg.Address
	s.logger = cfg.Logger
	s.artificialLatency = cfg.ArtificialLatency
	s.responsePayloadSize = cfg.ResponsePayloadSize
	s.minStreamDuration = cfg.MinStreamDuration
	s.maxStreamDuration = cfg.MaxStreamDuration
	s.toolCallConfig = toolCallConfig{
		MinToolCallsPerTurn: cfg.MinToolCallsPerTurn,
		MaxToolCallsPerTurn: cfg.MaxToolCallsPerTurn,
		ToolCallCommand:     cfg.ToolCallCommand,
		Seed:                seed,
	}
	s.logger.Info(ctx, "mock seed", slog.F("seed", seed))

	if cfg.TraceEnable {
		otel.SetTextMapPropagator(
			propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{},
				propagation.Baggage{},
			),
		)

		tracerProvider, closeTracing, err := tracing.TracerProvider(ctx, "llm-mock", tracing.TracerOpts{
			Default: cfg.TraceEnable,
		})
		if err != nil {
			s.logger.Warn(ctx, "failed to initialize tracing", slog.Error(err))
		} else {
			s.tracerProvider = tracerProvider
			s.closeTracing = closeTracing
		}
	}

	if err := s.startAPIServer(ctx); err != nil {
		return xerrors.Errorf("start API server: %w", err)
	}

	return nil
}

func (s *Server) Stop() error {
	if s.cancelHTTPContext != nil {
		s.cancelHTTPContext()
	}
	if s.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			return xerrors.Errorf("shutdown HTTP server: %w", err)
		}
	}
	if s.closeTracing != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.closeTracing(shutdownCtx); err != nil {
			s.logger.Warn(shutdownCtx, "failed to close tracing", slog.Error(err))
		}
	}
	return nil
}

func (s *Server) APIAddress() string {
	return fmt.Sprintf("http://%s", s.httpListener.Addr().String())
}

func (s *Server) randomStreamDuration() time.Duration {
	if s.minStreamDuration == 0 && s.maxStreamDuration == 0 {
		return 0
	}

	delta := s.maxStreamDuration - s.minStreamDuration
	//nolint:gosec // This is a scaletest mock, not security-sensitive.
	return s.minStreamDuration + time.Duration(rand.Int64N(int64(delta+1)))
}

func (s *Server) streamPacedChunks(ctx context.Context, totalDuration time.Duration, content string, emit func(isFirst bool, chunk string) bool) bool {
	var chunks []string
	if s.responsePayloadSize > 0 {
		chunks = streamContentFixedWindows(content)
	} else {
		fields := strings.Fields(content)
		if len(fields) == 0 {
			chunks = streamContentFixedWindows(content)
		} else {
			chunks = make([]string, 0, len(fields))
			for i, field := range fields {
				if i > 0 {
					field = " " + field
				}
				chunks = append(chunks, field)
			}
		}
	}

	if len(chunks) <= 1 {
		return emit(true, chunks[0])
	}

	delay := totalDuration / time.Duration(len(chunks)-1)
	for i, chunk := range chunks {
		if !emit(i == 0, chunk) {
			return false
		}
		if i == len(chunks)-1 || delay <= 0 {
			continue
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return false
		case <-timer.C:
		}
	}
	return true
}

func streamContentFixedWindows(content string) []string {
	if content == "" {
		return []string{""}
	}

	chunks := make([]string, 0, (len(content)+streamFixedWindowSize-1)/streamFixedWindowSize)
	for start := 0; start < len(content); start += streamFixedWindowSize {
		end := min(start+streamFixedWindowSize, len(content))
		chunks = append(chunks, content[start:end])
	}

	return chunks
}

func (s *Server) startAPIServer(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/chat/completions", s.handleOpenAI)
	mux.HandleFunc("POST /v1/responses", s.handleResponses)
	mux.HandleFunc("POST /v1/messages", s.handleAnthropic)

	var handler http.Handler = mux
	if s.tracerProvider != nil {
		handler = s.tracingMiddleware(handler)
	}

	baseCtx, cancelHTTPContext := context.WithCancel(ctx)
	s.cancelHTTPContext = cancelHTTPContext
	s.httpServer = &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return baseCtx
		},
	}

	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return xerrors.Errorf("listen on %s: %w", s.address, err)
	}
	s.httpListener = listener

	pproflabel.Go(ctx, pproflabel.Service("llm-mock"), func(ctx context.Context) {
		if err := s.httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error(ctx, "http API server error", slog.Error(err))
		}
	})

	return nil
}

func (s *Server) handleOpenAI(w http.ResponseWriter, r *http.Request) {
	pproflabel.Do(r.Context(), pproflabel.Service("llm-mock"), func(ctx context.Context) {
		s.handleOpenAIWithLabels(w, r.WithContext(ctx))
	})
}

func (s *Server) handleOpenAIWithLabels(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug(r.Context(), "handling OpenAI request")
	defer s.logger.Debug(r.Context(), "handled OpenAI request")

	ctx := r.Context()
	requestID := uuid.New()
	now := time.Now()

	var req llmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Error(ctx, "failed to parse OpenAI request", slog.Error(err))
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if s.artificialLatency > 0 {
		time.Sleep(s.artificialLatency)
	}

	resp, err := buildOpenAIResponse(req, requestID, now, s.responsePayloadSize, r.Header.Get(coderChatIDHeader), s.toolCallConfig)
	if err != nil {
		s.logger.Error(ctx, "failed to build OpenAI response", slog.Error(err))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Stream {
		s.sendOpenAIStream(ctx, w, resp)
		return
	}

	responseBody, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error(ctx, "failed to marshal OpenAI response", slog.Error(err))
		http.Error(w, "failed to marshal response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(responseBody); err != nil {
		s.logger.Error(ctx, "failed to write OpenAI response",
			slog.F("request_id", requestID),
			slog.Error(err),
			slog.F("error_type", "write_error"),
			slog.F("likely_cause", "network_error"),
		)
	}
}

func (s *Server) handleAnthropic(w http.ResponseWriter, r *http.Request) {
	pproflabel.Do(r.Context(), pproflabel.Service("llm-mock"), func(ctx context.Context) {
		s.handleAnthropicWithLabels(w, r.WithContext(ctx))
	})
}

func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	pproflabel.Do(r.Context(), pproflabel.Service("llm-mock"), func(ctx context.Context) {
		s.handleResponsesWithLabels(w, r.WithContext(ctx))
	})
}

func (s *Server) handleResponsesWithLabels(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug(r.Context(), "handling OpenAI responses request")
	defer s.logger.Debug(r.Context(), "handled OpenAI responses request")

	ctx := r.Context()
	requestID := uuid.New()
	now := time.Now()

	var req llmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Error(ctx, "failed to parse OpenAI responses request", slog.Error(err))
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if s.artificialLatency > 0 {
		time.Sleep(s.artificialLatency)
	}

	var resp responsesResponse
	resp.ID = fmt.Sprintf("resp_%s", requestID.String()[:8])
	resp.Object = "response"
	resp.Created = now.Unix()
	resp.Model = req.Model

	assistantText := responseText(s.responsePayloadSize, openAIResponsesDefaultResponseText)

	resp.Output = []struct {
		ID      string `json:"id,omitempty"`
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}{
		{
			ID:   fmt.Sprintf("msg_%s", requestID.String()[:8]),
			Type: "message",
			Role: "assistant",
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{
					Type: "output_text",
					Text: assistantText,
				},
			},
		},
	}

	resp.Usage.InputTokens = mockInputTokens
	resp.Usage.OutputTokens = mockOutputTokens
	resp.Usage.TotalTokens = mockInputTokens + mockOutputTokens

	if req.Stream {
		s.sendResponsesStream(ctx, w, resp)
		return
	}

	responseBody, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error(ctx, "failed to marshal OpenAI responses response", slog.Error(err))
		http.Error(w, "failed to marshal response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(responseBody); err != nil {
		s.logger.Error(ctx, "failed to write OpenAI responses response",
			slog.F("request_id", requestID),
			slog.Error(err),
			slog.F("error_type", "write_error"),
			slog.F("likely_cause", "network_error"),
		)
	}
}

func (s *Server) handleAnthropicWithLabels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := uuid.New()

	var req llmRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Error(ctx, "failed to parse LLM request", slog.Error(err))
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if s.artificialLatency > 0 {
		time.Sleep(s.artificialLatency)
	}

	var resp anthropicResponse
	resp.ID = fmt.Sprintf("msg_%s", requestID.String()[:8])
	resp.Type = "message"
	resp.Role = "assistant"

	assistantText := responseText(s.responsePayloadSize, anthropicDefaultResponseText)

	resp.Content = []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{
		{
			Type: "text",
			Text: assistantText,
		},
	}
	resp.Model = req.Model
	resp.StopReason = "end_turn"
	resp.Usage.InputTokens = mockInputTokens
	resp.Usage.OutputTokens = mockOutputTokens

	if req.Stream {
		s.sendAnthropicStream(ctx, w, resp)
		return
	}

	responseBody, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error(ctx, "failed to marshal Anthropic response", slog.Error(err))
		http.Error(w, "failed to marshal response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("anthropic-version", "2023-06-01")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(responseBody); err != nil {
		s.logger.Error(ctx, "failed to write Anthropic response",
			slog.F("request_id", requestID),
			slog.Error(err),
			slog.F("error_type", "write_error"),
			slog.F("likely_cause", "network_error"),
		)
	}
}

func (s *Server) sendOpenAIStream(ctx context.Context, w http.ResponseWriter, resp openAIResponse) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.logger.Error(ctx, "responseWriter does not support flushing",
			slog.F("response_id", resp.ID),
		)
		return
	}

	writeChunk := func(data string) bool {
		if _, err := fmt.Fprintf(w, "%s", data); err != nil {
			s.logger.Error(ctx, "failed to write OpenAI stream chunk",
				slog.F("response_id", resp.ID),
				slog.Error(err),
				slog.F("error_type", "write_error"),
				slog.F("likely_cause", "network_error"),
			)
			return false
		}
		flusher.Flush()
		return true
	}

	// Non-terminal chunks pass nil so JSON emits null for finish_reason.
	writeStreamChunk := func(delta map[string]interface{}, finishReason interface{}) bool {
		chunk := map[string]interface{}{
			"id":      resp.ID,
			"object":  "chat.completion.chunk",
			"created": resp.Created,
			"model":   resp.Model,
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"delta":         delta,
					"finish_reason": finishReason,
				},
			},
		}
		chunkBytes, _ := json.Marshal(chunk)
		return writeChunk(fmt.Sprintf("data: %s\n\n", chunkBytes))
	}

	choice := resp.Choices[0]
	switch {
	case len(choice.Message.ToolCalls) > 0:
		if !writeStreamChunk(map[string]interface{}{
			"role":       "assistant",
			"tool_calls": choice.Message.ToolCalls,
		}, nil) {
			return
		}
	default:
		totalDuration := s.randomStreamDuration()
		if totalDuration == 0 {
			if !writeStreamChunk(map[string]interface{}{
				"role":    "assistant",
				"content": choice.Message.Content,
			}, nil) {
				return
			}
			break
		}
		if !s.streamPacedChunks(ctx, totalDuration, choice.Message.Content, func(isFirst bool, chunk string) bool {
			delta := map[string]interface{}{
				"content": chunk,
			}
			if isFirst {
				delta["role"] = "assistant"
			}
			return writeStreamChunk(delta, nil)
		}) {
			return
		}
	}

	if !writeStreamChunk(map[string]interface{}{}, choice.FinishReason) {
		return
	}
	writeChunk("data: [DONE]\n\n")
}

func (s *Server) sendResponsesStream(ctx context.Context, w http.ResponseWriter, resp responsesResponse) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.logger.Error(ctx, "responseWriter does not support flushing",
			slog.F("response_id", resp.ID),
		)
		return
	}

	writeChunk := func(data string) bool {
		if _, err := fmt.Fprintf(w, "%s", data); err != nil {
			s.logger.Error(ctx, "failed to write OpenAI responses stream chunk",
				slog.F("response_id", resp.ID),
				slog.Error(err),
				slog.F("error_type", "write_error"),
				slog.F("likely_cause", "network_error"),
			)
			return false
		}
		flusher.Flush()
		return true
	}

	totalDuration := s.randomStreamDuration()
	if totalDuration == 0 {
		deltaChunk := map[string]interface{}{
			"id":            resp.ID,
			"object":        "response.output_text.delta",
			"created":       resp.Created,
			"model":         resp.Model,
			"output_index":  0,
			"content_index": 0,
			"delta":         resp.Output[0].Content[0].Text,
		}
		deltaBytes, _ := json.Marshal(deltaChunk)
		if !writeChunk(fmt.Sprintf("data: %s\n\n", deltaBytes)) {
			return
		}
	} else {
		if !s.streamPacedChunks(ctx, totalDuration, resp.Output[0].Content[0].Text, func(_ bool, chunk string) bool {
			deltaChunk := map[string]interface{}{
				"id":            resp.ID,
				"object":        "response.output_text.delta",
				"created":       resp.Created,
				"model":         resp.Model,
				"output_index":  0,
				"content_index": 0,
				"delta":         chunk,
			}
			deltaBytes, _ := json.Marshal(deltaChunk)
			return writeChunk(fmt.Sprintf("data: %s\n\n", deltaBytes))
		}) {
			return
		}
	}

	finalChunk := map[string]interface{}{
		"id":      resp.ID,
		"object":  "response.completed",
		"created": resp.Created,
		"model":   resp.Model,
		"response": map[string]interface{}{
			"id":      resp.ID,
			"object":  resp.Object,
			"created": resp.Created,
			"model":   resp.Model,
			"output":  resp.Output,
			"usage":   resp.Usage,
		},
	}
	finalBytes, _ := json.Marshal(finalChunk)
	if !writeChunk(fmt.Sprintf("data: %s\n\n", finalBytes)) {
		return
	}
	writeChunk("data: [DONE]\n\n")
}

func (s *Server) sendAnthropicStream(ctx context.Context, w http.ResponseWriter, resp anthropicResponse) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("anthropic-version", "2023-06-01")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.logger.Error(ctx, "responseWriter does not support flushing",
			slog.F("response_id", resp.ID),
		)
		return
	}

	writeChunk := func(eventType string, data []byte) bool {
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data); err != nil {
			s.logger.Error(ctx, "failed to write Anthropic stream chunk",
				slog.F("response_id", resp.ID),
				slog.Error(err),
				slog.F("error_type", "write_error"),
				slog.F("likely_cause", "network_error"),
			)
			return false
		}
		flusher.Flush()
		return true
	}

	totalDuration := s.randomStreamDuration()

	startEventType := "message_start"
	startEvent := map[string]interface{}{
		"type": startEventType,
		"message": map[string]interface{}{
			"id":    resp.ID,
			"type":  resp.Type,
			"role":  resp.Role,
			"model": resp.Model,
		},
	}
	startBytes, _ := json.Marshal(startEvent)
	if !writeChunk(startEventType, startBytes) {
		return
	}

	// Send content_block_start event
	contentStartEventType := "content_block_start"
	contentStartText := resp.Content[0].Text
	if totalDuration > 0 {
		contentStartText = ""
	}
	contentStartEvent := map[string]interface{}{
		"type":  contentStartEventType,
		"index": 0,
		"content_block": map[string]interface{}{
			"type": "text",
			"text": contentStartText,
		},
	}
	contentStartBytes, _ := json.Marshal(contentStartEvent)
	if !writeChunk(contentStartEventType, contentStartBytes) {
		return
	}

	// Send content_block_delta event(s)
	deltaEventType := "content_block_delta"
	if totalDuration == 0 {
		deltaEvent := map[string]interface{}{
			"type":  deltaEventType,
			"index": 0,
			"delta": map[string]interface{}{
				"type": "text_delta",
				"text": resp.Content[0].Text,
			},
		}
		deltaBytes, _ := json.Marshal(deltaEvent)
		if !writeChunk(deltaEventType, deltaBytes) {
			return
		}
	} else {
		if !s.streamPacedChunks(ctx, totalDuration, resp.Content[0].Text, func(_ bool, chunk string) bool {
			deltaEvent := map[string]interface{}{
				"type":  deltaEventType,
				"index": 0,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": chunk,
				},
			}
			deltaBytes, _ := json.Marshal(deltaEvent)
			return writeChunk(deltaEventType, deltaBytes)
		}) {
			return
		}
	}

	// Send content_block_stop event
	contentStopEventType := "content_block_stop"
	contentStopEvent := map[string]interface{}{
		"type":  contentStopEventType,
		"index": 0,
	}
	contentStopBytes, _ := json.Marshal(contentStopEvent)
	if !writeChunk(contentStopEventType, contentStopBytes) {
		return
	}

	// Send message_delta event
	deltaMsgEventType := "message_delta"
	deltaMsgEvent := map[string]interface{}{
		"type": deltaMsgEventType,
		"delta": map[string]interface{}{
			"stop_reason":   resp.StopReason,
			"stop_sequence": resp.StopSequence,
		},
		"usage": resp.Usage,
	}
	deltaMsgBytes, _ := json.Marshal(deltaMsgEvent)
	if !writeChunk(deltaMsgEventType, deltaMsgBytes) {
		return
	}

	// Send message_stop event
	stopEventType := "message_stop"
	stopEvent := map[string]interface{}{
		"type": stopEventType,
	}
	stopBytes, _ := json.Marshal(stopEvent)
	writeChunk(stopEventType, stopBytes)
}

func (s *Server) tracingMiddleware(next http.Handler) http.Handler {
	tracer := s.tracerProvider.Tracer("llm-mock")

	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		// Wrap response writer with StatusWriter for tracing
		sw := &tracing.StatusWriter{ResponseWriter: rw}

		// Extract trace context from headers
		propagator := otel.GetTextMapPropagator()
		hc := propagation.HeaderCarrier(r.Header)
		ctx := propagator.Extract(r.Context(), hc)

		// Start span with initial name (will be updated after handler)
		ctx, span := tracer.Start(ctx, fmt.Sprintf("%s %s", r.Method, r.RequestURI))
		defer span.End()
		r = r.WithContext(ctx)

		// Inject trace context into response headers
		if span.SpanContext().HasTraceID() && span.SpanContext().HasSpanID() {
			rw.Header().Set("X-Trace-ID", span.SpanContext().TraceID().String())
			rw.Header().Set("X-Span-ID", span.SpanContext().SpanID().String())

			hc := propagation.HeaderCarrier(rw.Header())
			propagator.Inject(ctx, hc)
		}

		// Execute the handler
		next.ServeHTTP(sw, r)

		// Update span with final route and response information
		route := r.URL.Path
		span.SetName(fmt.Sprintf("%s %s", r.Method, route))
		span.SetAttributes(netconv.Transport("tcp"))
		span.SetAttributes(httpconv.ServerRequest("llm-mock", r)...)
		span.SetAttributes(semconv.HTTPRouteKey.String(route))

		status := sw.Status
		if status == 0 {
			status = http.StatusOK
		}
		span.SetAttributes(semconv.HTTPStatusCodeKey.Int(status))
		span.SetStatus(httpconv.ServerStatus(status))
	})
}
