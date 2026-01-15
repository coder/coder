package llmmock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// Server wraps the LLM mock server and provides an HTTP API to retrieve requests.
type Server struct {
	httpServer   *http.Server
	httpListener net.Listener
	logger       slog.Logger

	address             string
	artificialLatency   time.Duration
	responsePayloadSize int

	tracerProvider trace.TracerProvider
	closeTracing   func(context.Context) error
}

type Config struct {
	Address             string
	Logger              slog.Logger
	ArtificialLatency   time.Duration
	ResponsePayloadSize int

	PprofEnable  bool
	PprofAddress string

	TraceEnable bool
}

type llmRequest struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream,omitempty"`
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
	s.address = cfg.Address
	s.logger = cfg.Logger
	s.artificialLatency = cfg.ArtificialLatency
	s.responsePayloadSize = cfg.ResponsePayloadSize

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

func (s *Server) startAPIServer(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/chat/completions", s.handleOpenAI)
	mux.HandleFunc("POST /v1/messages", s.handleAnthropic)

	var handler http.Handler = mux
	if s.tracerProvider != nil {
		handler = s.tracingMiddleware(handler)
	}

	s.httpServer = &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
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

	var resp openAIResponse
	resp.ID = fmt.Sprintf("chatcmpl-%s", requestID.String()[:8])
	resp.Object = "chat.completion"
	resp.Created = now.Unix()
	resp.Model = req.Model

	var responseContent string
	if s.responsePayloadSize > 0 {
		pattern := "x"
		repeated := strings.Repeat(pattern, s.responsePayloadSize)
		responseContent = repeated[:s.responsePayloadSize]
	} else {
		responseContent = "This is a mock response from OpenAI."
	}

	resp.Choices = []struct {
		Index        int           `json:"index"`
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	}{
		{
			Index: 0,
			Message: openAIMessage{
				Role:    "assistant",
				Content: responseContent,
			},
			FinishReason: "stop",
		},
	}

	resp.Usage.PromptTokens = 10
	resp.Usage.CompletionTokens = 5
	resp.Usage.TotalTokens = 15

	responseBody, _ := json.Marshal(resp)

	if req.Stream {
		s.sendOpenAIStream(ctx, w, resp)
	} else {
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
}

func (s *Server) handleAnthropic(w http.ResponseWriter, r *http.Request) {
	pproflabel.Do(r.Context(), pproflabel.Service("llm-mock"), func(ctx context.Context) {
		s.handleAnthropicWithLabels(w, r.WithContext(ctx))
	})
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

	var responseText string
	if s.responsePayloadSize > 0 {
		pattern := "x"
		repeated := strings.Repeat(pattern, s.responsePayloadSize)
		responseText = repeated[:s.responsePayloadSize]
	} else {
		responseText = "This is a mock response from Anthropic."
	}

	resp.Content = []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{
		{
			Type: "text",
			Text: responseText,
		},
	}
	resp.Model = req.Model
	resp.StopReason = "end_turn"
	resp.Usage.InputTokens = 10
	resp.Usage.OutputTokens = 5

	responseBody, _ := json.Marshal(resp)

	if req.Stream {
		s.sendAnthropicStream(ctx, w, resp)
	} else {
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
	if !writeChunk(fmt.Sprintf("data: %s\n\n", chunkBytes)) {
		return
	}

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
	if !writeChunk(fmt.Sprintf("data: %s\n\n", finalChunkBytes)) {
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

	writeChunk := func(data string) bool {
		if _, err := fmt.Fprintf(w, "%s", data); err != nil {
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
	if !writeChunk(fmt.Sprintf("data: %s\n\n", startBytes)) {
		return
	}

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
	if !writeChunk(fmt.Sprintf("data: %s\n\n", contentStartBytes)) {
		return
	}

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
	if !writeChunk(fmt.Sprintf("data: %s\n\n", deltaBytes)) {
		return
	}

	// Send content_block_stop event
	contentStopEvent := map[string]interface{}{
		"type":  "content_block_stop",
		"index": 0,
	}
	contentStopBytes, _ := json.Marshal(contentStopEvent)
	if !writeChunk(fmt.Sprintf("data: %s\n\n", contentStopBytes)) {
		return
	}

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
	if !writeChunk(fmt.Sprintf("data: %s\n\n", deltaMsgBytes)) {
		return
	}

	// Send message_stop event
	stopEvent := map[string]interface{}{
		"type": "message_stop",
	}
	stopBytes, _ := json.Marshal(stopEvent)
	writeChunk(fmt.Sprintf("data: %s\n\n", stopBytes))
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
