package llmmock

import (
	"context"
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
	openAIStopFinishReason             = "stop"
	openAIToolCallFinishReason         = "tool_calls"
	openAIResponsesDefaultResponseText = "This is a mock response from OpenAI Responses."
	anthropicDefaultResponseText       = "This is a mock response from Anthropic."
	mockInputTokens                    = 10
	mockOutputTokens                   = 5
	// streamFixedWindowSize keeps large payload chunks predictable.
	streamFixedWindowSize = 10
)

// Server wraps the LLM mock server and provides an HTTP API to retrieve requests.
type Server struct {
	httpServer   *http.Server
	httpListener net.Listener
	httpCancel   context.CancelFunc
	logger       slog.Logger

	address             string
	artificialLatency   time.Duration
	minStreamDuration   time.Duration
	maxStreamDuration   time.Duration
	responsePayloadSize int
	toolCallsPerTurn    int
	toolCallCommand     string

	tracerProvider trace.TracerProvider
	closeTracing   func(context.Context) error
}

type Config struct {
	Address             string
	Logger              slog.Logger
	ArtificialLatency   time.Duration
	MinStreamDuration   time.Duration
	MaxStreamDuration   time.Duration
	ResponsePayloadSize int
	ToolCallsPerTurn    int
	// ToolCallCommand is the command sent in generated execute tool calls.
	// Empty uses the default tool-call command.
	ToolCallCommand string

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
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Function openAIToolCallFunction `json:"function"`
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
	toolCallCommand := cfg.ToolCallCommand
	if toolCallCommand == "" {
		toolCallCommand = defaultToolCallCommand
	}

	s.address = cfg.Address
	s.logger = cfg.Logger
	s.artificialLatency = cfg.ArtificialLatency
	s.minStreamDuration = cfg.MinStreamDuration
	s.maxStreamDuration = cfg.MaxStreamDuration
	s.responsePayloadSize = cfg.ResponsePayloadSize
	s.toolCallsPerTurn = cfg.ToolCallsPerTurn
	s.toolCallCommand = toolCallCommand

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
	if s.httpCancel != nil {
		s.httpCancel()
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

func (s *Server) responseText(fallback string) string {
	if s.responsePayloadSize > 0 {
		return strings.Repeat("x", s.responsePayloadSize)
	}
	return fallback
}

func (s *Server) randomStreamDuration() time.Duration {
	if s.minStreamDuration <= 0 || s.maxStreamDuration <= 0 {
		return 0
	}
	if s.minStreamDuration >= s.maxStreamDuration {
		return s.minStreamDuration
	}

	delta := s.maxStreamDuration - s.minStreamDuration
	//nolint:gosec // This is a scaletest mock, not security-sensitive.
	return s.minStreamDuration + time.Duration(rand.Int64N(int64(delta+1)))
}

func (s *Server) streamContentChunks(content string) []string {
	if content == "" {
		return []string{""}
	}
	if s.responsePayloadSize > 0 {
		chunks := make([]string, 0, (len(content)+streamFixedWindowSize-1)/streamFixedWindowSize)
		for start := 0; start < len(content); start += streamFixedWindowSize {
			end := min(start+streamFixedWindowSize, len(content))
			chunks = append(chunks, content[start:end])
		}
		return chunks
	}

	chunks := strings.SplitAfter(content, " ")
	if chunks[len(chunks)-1] == "" {
		chunks = chunks[:len(chunks)-1]
	}
	return chunks
}

func streamPacedChunks(ctx context.Context, totalDuration time.Duration, chunks []string, emit func(chunk string) bool) bool {
	if len(chunks) == 0 {
		return emit("")
	}
	if len(chunks) == 1 {
		return emit(chunks[0])
	}

	delay := totalDuration / time.Duration(len(chunks)-1)
	for i, chunk := range chunks {
		if !emit(chunk) {
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

func (s *Server) startAPIServer(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/chat/completions", s.handleOpenAI)
	mux.HandleFunc("POST /v1/responses", s.handleResponses)
	mux.HandleFunc("POST /v1/messages", s.handleAnthropic)

	var handler http.Handler = mux
	if s.tracerProvider != nil {
		handler = s.tracingMiddleware(handler)
	}

	baseCtx, httpCancel := context.WithCancel(ctx)
	s.httpCancel = httpCancel
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

	choice, err := s.buildOpenAIChoice(req)
	if err != nil {
		s.logger.Error(ctx, "failed to build OpenAI response", slog.Error(err))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp := openAIResponse{
		ID:      fmt.Sprintf("chatcmpl-%s", requestID.String()[:8]),
		Object:  "chat.completion",
		Created: now.Unix(),
		Model:   req.Model,
		Choices: []openAIResponseChoice{choice},
	}
	resp.Usage.PromptTokens = mockInputTokens
	resp.Usage.CompletionTokens = mockOutputTokens
	resp.Usage.TotalTokens = mockInputTokens + mockOutputTokens

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

	assistantText := s.responseText(openAIResponsesDefaultResponseText)

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

	assistantText := s.responseText(anthropicDefaultResponseText)

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

	writeDelta := func(text string) bool {
		deltaChunk := map[string]any{
			"id":            resp.ID,
			"object":        "response.output_text.delta",
			"created":       resp.Created,
			"model":         resp.Model,
			"output_index":  0,
			"content_index": 0,
			"delta":         text,
		}
		deltaBytes, _ := json.Marshal(deltaChunk)
		return writeChunk(fmt.Sprintf("data: %s\n\n", deltaBytes))
	}

	text := resp.Output[0].Content[0].Text
	totalDuration := s.randomStreamDuration()
	if totalDuration == 0 {
		if !writeDelta(text) {
			return
		}
	} else {
		if !streamPacedChunks(ctx, totalDuration, s.streamContentChunks(text), writeDelta) {
			return
		}
	}

	finalChunk := map[string]any{
		"id":      resp.ID,
		"object":  "response.completed",
		"created": resp.Created,
		"model":   resp.Model,
		"response": map[string]any{
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
	_ = writeChunk("data: [DONE]\n\n")
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
	paced := totalDuration > 0

	startEventType := "message_start"
	startEvent := map[string]any{
		"type": startEventType,
		"message": map[string]any{
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

	contentStartEventType := "content_block_start"
	var contentStartText string
	if !paced {
		contentStartText = resp.Content[0].Text
	}
	contentStartEvent := map[string]any{
		"type":  contentStartEventType,
		"index": 0,
		"content_block": map[string]any{
			"type": "text",
			"text": contentStartText,
		},
	}
	contentStartBytes, _ := json.Marshal(contentStartEvent)
	if !writeChunk(contentStartEventType, contentStartBytes) {
		return
	}

	deltaEventType := "content_block_delta"
	writeDelta := func(text string) bool {
		deltaEvent := map[string]any{
			"type":  deltaEventType,
			"index": 0,
			"delta": map[string]any{
				"type": "text_delta",
				"text": text,
			},
		}
		deltaBytes, _ := json.Marshal(deltaEvent)
		return writeChunk(deltaEventType, deltaBytes)
	}
	if !paced {
		if !writeDelta(resp.Content[0].Text) {
			return
		}
	} else {
		if !streamPacedChunks(ctx, totalDuration, s.streamContentChunks(resp.Content[0].Text), writeDelta) {
			return
		}
	}

	contentStopEventType := "content_block_stop"
	contentStopEvent := map[string]any{
		"type":  contentStopEventType,
		"index": 0,
	}
	contentStopBytes, _ := json.Marshal(contentStopEvent)
	if !writeChunk(contentStopEventType, contentStopBytes) {
		return
	}

	deltaMsgEventType := "message_delta"
	deltaMsgEvent := map[string]any{
		"type": deltaMsgEventType,
		"delta": map[string]any{
			"stop_reason":   resp.StopReason,
			"stop_sequence": resp.StopSequence,
		},
		"usage": resp.Usage,
	}
	deltaMsgBytes, _ := json.Marshal(deltaMsgEvent)
	if !writeChunk(deltaMsgEventType, deltaMsgBytes) {
		return
	}

	stopEventType := "message_stop"
	stopEvent := map[string]any{
		"type": stopEventType,
	}
	stopBytes, _ := json.Marshal(stopEvent)
	_ = writeChunk(stopEventType, stopBytes)
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
