package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.14.0"
	"go.opentelemetry.io/otel/semconv/v1.14.0/httpconv"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/quartz"
)

type (
	tracingContextKey struct{}
	tracingContext    struct {
		provider   string
		model      string
		stream     bool
		requestNum int
		mode       RequestMode
	}
)

type tracingTransport struct {
	cfg        Config
	underlying http.RoundTripper
}

func newTracingTransport(cfg Config, underlying http.RoundTripper) *tracingTransport {
	if underlying == nil {
		underlying = http.DefaultTransport
	}
	return &tracingTransport{
		cfg:        cfg,
		underlying: otelhttp.NewTransport(underlying),
	}
}

func (t *tracingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	aibridgeCtx, hasAIBridgeCtx := req.Context().Value(tracingContextKey{}).(tracingContext)

	resp, err := t.underlying.RoundTrip(req)

	if hasAIBridgeCtx {
		ctx := req.Context()
		if resp != nil && resp.Request != nil {
			ctx = resp.Request.Context()
		}
		span := trace.SpanFromContext(ctx)
		if span.IsRecording() {
			span.SetAttributes(
				attribute.String("aibridge.provider", aibridgeCtx.provider),
				attribute.String("aibridge.model", aibridgeCtx.model),
				attribute.Bool("aibridge.stream", aibridgeCtx.stream),
				attribute.Int("aibridge.request_num", aibridgeCtx.requestNum),
				attribute.String("aibridge.mode", string(aibridgeCtx.mode)),
			)
		}
	}

	return resp, err
}

type Runner struct {
	client           *codersdk.Client
	cfg              Config
	strategy         requestModeStrategy
	providerStrategy ProviderStrategy

	clock      quartz.Clock
	httpClient *http.Client

	requestCount  int64
	successCount  int64
	failureCount  int64
	totalDuration time.Duration
	totalTokens   int64
}

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	httpTimeout := cfg.HTTPTimeout
	if httpTimeout <= 0 {
		httpTimeout = 30 * time.Second
	}
	return &Runner{
		client:           client,
		cfg:              cfg,
		strategy:         cfg.NewStrategy(client),
		providerStrategy: NewProviderStrategy(cfg.Provider),
		clock:            quartz.NewReal(),
		httpClient: &http.Client{
			Timeout:   httpTimeout,
			Transport: newTracingTransport(cfg, http.DefaultTransport),
		},
	}
}

func (r *Runner) WithClock(clock quartz.Clock) *Runner {
	r.clock = clock
	return r
}

var (
	_ harness.Runnable    = &Runner{}
	_ harness.Cleanable   = &Runner{}
	_ harness.Collectable = &Runner{}
)

func (r *Runner) Run(ctx context.Context, id string, logs io.Writer) error {
	ctx, span := tracing.StartSpan(ctx)
	defer span.End()

	logs = loadtestutil.NewSyncWriter(logs)
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)

	requestURL, token, err := r.strategy.Setup(ctx, id, logs)
	if err != nil {
		return xerrors.Errorf("strategy setup: %w", err)
	}

	model := r.providerStrategy.DefaultModel()

	logger.Info(ctx, "bridge runner is ready",
		slog.F("request_count", r.cfg.RequestCount),
		slog.F("model", model),
		slog.F("stream", r.cfg.Stream),
	)

	for i := 0; i < r.cfg.RequestCount; i++ {
		r.requestCount++
		if err := r.makeRequest(ctx, logger, requestURL, token, model, i); err != nil {
			logger.Warn(ctx, "bridge request failed",
				slog.F("request_num", i+1),
				slog.F("error_type", "request_failed"),
				slog.Error(err),
			)
			r.cfg.Metrics.AddError("request")
			r.cfg.Metrics.AddRequest("failure")
			r.failureCount++

			// Continue making requests even if one fails
			continue
		}
		r.successCount++
		r.cfg.Metrics.AddRequest("success")
	}

	logger.Info(ctx, "bridge runner completed",
		slog.F("total_requests", r.requestCount),
		slog.F("success", r.successCount),
		slog.F("failure", r.failureCount),
	)

	// Fail the run if any request failed
	if r.failureCount > 0 {
		return xerrors.Errorf("bridge runner failed: %d out of %d requests failed", r.failureCount, r.cfg.RequestCount)
	}

	return nil
}

func (r *Runner) makeRequest(ctx context.Context, logger slog.Logger, url, token, model string, requestNum int) error {
	start := r.clock.Now()

	ctx = context.WithValue(ctx, tracingContextKey{}, tracingContext{
		provider:   r.cfg.Provider,
		model:      model,
		stream:     r.cfg.Stream,
		requestNum: requestNum + 1,
		mode:       r.cfg.Mode,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(r.cfg.RequestBody))
	if err != nil {
		return xerrors.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	logger.Debug(ctx, "making bridge request",
		slog.F("url", url),
		slog.F("request_num", requestNum+1),
		slog.F("model", model),
	)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		span := trace.SpanFromContext(req.Context())
		if span.IsRecording() {
			span.RecordError(err)
		}
		logger.Warn(ctx, "request failed during execution",
			slog.F("request_num", requestNum+1),
			slog.Error(err),
		)
		return xerrors.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	span := trace.SpanFromContext(req.Context())
	if span.IsRecording() {
		span.SetAttributes(semconv.HTTPStatusCodeKey.Int(resp.StatusCode))
		span.SetStatus(httpconv.ClientStatus(resp.StatusCode))
	}

	duration := r.clock.Since(start)
	r.totalDuration += duration
	r.cfg.Metrics.ObserveDuration(duration.Seconds())

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err := xerrors.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
		span.RecordError(err)
		return err
	}

	if r.cfg.Stream {
		err := r.handleStreamingResponse(ctx, logger, resp)
		if err != nil {
			span.RecordError(err)
			return err
		}
		return nil
	}

	return r.handleNonStreamingResponse(ctx, logger, resp)
}

func (r *Runner) handleNonStreamingResponse(ctx context.Context, logger slog.Logger, resp *http.Response) error {
	if r.cfg.Provider == "anthropic" {
		return r.handleAnthropicResponse(ctx, logger, resp)
	}
	return r.handleOpenAIResponse(ctx, logger, resp)
}

func (r *Runner) handleOpenAIResponse(ctx context.Context, logger slog.Logger, resp *http.Response) error {
	var response struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return xerrors.Errorf("decode response: %w", err)
	}

	if len(response.Choices) > 0 {
		assistantContent := response.Choices[0].Message.Content
		logger.Debug(ctx, "received response",
			slog.F("response_id", response.ID),
			slog.F("content_length", len(assistantContent)),
		)
	}

	if response.Usage.TotalTokens > 0 {
		r.totalTokens += int64(response.Usage.TotalTokens)
		r.cfg.Metrics.AddTokens("input", int64(response.Usage.PromptTokens))
		r.cfg.Metrics.AddTokens("output", int64(response.Usage.CompletionTokens))
	}

	return nil
}

func (r *Runner) handleAnthropicResponse(ctx context.Context, logger slog.Logger, resp *http.Response) error {
	var response struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return xerrors.Errorf("decode response: %w", err)
	}

	var assistantContent string
	if len(response.Content) > 0 {
		assistantContent = response.Content[0].Text
		logger.Debug(ctx, "received response",
			slog.F("response_id", response.ID),
			slog.F("content_length", len(assistantContent)),
		)
	}

	totalTokens := response.Usage.InputTokens + response.Usage.OutputTokens
	if totalTokens > 0 {
		r.totalTokens += int64(totalTokens)
		r.cfg.Metrics.AddTokens("input", int64(response.Usage.InputTokens))
		r.cfg.Metrics.AddTokens("output", int64(response.Usage.OutputTokens))
	}

	return nil
}

func (*Runner) handleStreamingResponse(ctx context.Context, logger slog.Logger, resp *http.Response) error {
	buf := make([]byte, 4096)
	totalRead := 0
	for {
		// Check for context cancellation before each read
		if ctx.Err() != nil {
			logger.Warn(ctx, "streaming response canceled",
				slog.F("bytes_read", totalRead),
				slog.Error(ctx.Err()),
			)
			return xerrors.Errorf("stream canceled: %w", ctx.Err())
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			totalRead += n
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			// Check if error is due to context cancellation
			if xerrors.Is(err, context.Canceled) || xerrors.Is(err, context.DeadlineExceeded) {
				logger.Warn(ctx, "streaming response read canceled",
					slog.F("bytes_read", totalRead),
					slog.Error(err),
				)
				return xerrors.Errorf("stream read canceled: %w", err)
			}
			logger.Warn(ctx, "streaming response read error",
				slog.F("bytes_read", totalRead),
				slog.Error(err),
			)
			return xerrors.Errorf("read stream: %w", err)
		}
	}

	logger.Debug(ctx, "received streaming response", slog.F("bytes_read", totalRead))
	return nil
}

func (r *Runner) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	return r.strategy.Cleanup(ctx, id, logs)
}

func (r *Runner) GetMetrics() map[string]any {
	avgDuration := time.Duration(0)
	if r.requestCount > 0 {
		avgDuration = r.totalDuration / time.Duration(r.requestCount)
	}

	return map[string]any{
		"request_count":  r.requestCount,
		"success_count":  r.successCount,
		"failure_count":  r.failureCount,
		"total_duration": r.totalDuration.String(),
		"avg_duration":   avgDuration.String(),
		"total_tokens":   r.totalTokens,
	}
}
