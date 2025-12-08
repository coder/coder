package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"

	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
	"github.com/coder/coder/v2/scaletest/harness"
	"github.com/coder/coder/v2/scaletest/loadtestutil"
	"github.com/coder/quartz"
)

type Runner struct {
	client *codersdk.Client
	cfg    Config

	createUserRunner *createusers.Runner

	clock      quartz.Clock
	httpClient *http.Client

	// Metrics tracking
	requestCount  int64
	successCount  int64
	failureCount  int64
	totalDuration time.Duration
	totalTokens   int64
}

func NewRunner(client *codersdk.Client, cfg Config) *Runner {
	return &Runner{
		client:     client,
		cfg:        cfg,
		clock:      quartz.NewReal(),
		httpClient: &http.Client{Timeout: 30 * time.Second},
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

	var token string
	var requestURL string

	// Determine mode: direct or bridge
	if r.cfg.Mode == RequestModeDirect {
		// Direct mode: skip user creation, use upstream URL directly
		requestURL = r.cfg.UpstreamURL
		if r.cfg.DirectToken != "" {
			token = r.cfg.DirectToken
		} else if r.client.SessionToken() != "" {
			token = r.client.SessionToken()
		}
		logger.Info(ctx, "bridge runner in direct mode", slog.F("url", requestURL))
	} else {
		// Bridge mode: create user and use AI Bridge endpoint
		r.client.SetLogger(logger)
		r.client.SetLogBodies(true)

		r.createUserRunner = createusers.NewRunner(r.client, r.cfg.User)
		newUserAndToken, err := r.createUserRunner.RunReturningUser(ctx, id, logs)
		if err != nil {
			r.cfg.Metrics.AddError("create_user")
			return xerrors.Errorf("create user: %w", err)
		}
		newUser := newUserAndToken.User
		token = newUserAndToken.SessionToken

		logger.Info(ctx, "runner user created", slog.F("username", newUser.Username), slog.F("user_id", newUser.ID.String()))

		// Construct AI Bridge URL
		requestURL = fmt.Sprintf("%s/api/v2/aibridge/openai/v1/chat/completions", r.client.URL)
		logger.Info(ctx, "bridge runner in bridge mode", slog.F("url", requestURL))
	}

	// Set defaults if not provided
	requestCount := r.cfg.RequestCount
	if requestCount <= 0 {
		requestCount = 1
	}
	model := r.cfg.Model
	if model == "" {
		model = "gpt-4"
	}

	logger.Info(ctx, "bridge runner is ready",
		slog.F("request_count", requestCount),
		slog.F("model", model),
		slog.F("stream", r.cfg.Stream),
	)

	// Make requests
	for i := 0; i < requestCount; i++ {
		if err := r.makeRequest(ctx, logger, requestURL, token, model, i); err != nil {
			logger.Warn(ctx, "request failed", slog.F("request_num", i+1), slog.Error(err))
			r.cfg.Metrics.AddError("request")
			r.failureCount++
			r.cfg.Metrics.AddRequest("failure")
			// Continue making requests even if one fails
			continue
		}
		r.successCount++
		r.cfg.Metrics.AddRequest("success")
		r.requestCount++
	}

	logger.Info(ctx, "bridge runner completed",
		slog.F("total_requests", r.requestCount),
		slog.F("success", r.successCount),
		slog.F("failure", r.failureCount),
	)

	return nil
}

func (r *Runner) makeRequest(ctx context.Context, logger slog.Logger, url, token, model string, requestNum int) error {
	start := r.clock.Now()

	// Prepare request body
	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": fmt.Sprintf("Hello, this is test request #%d from the bridge load generator.", requestNum+1),
			},
		},
		"stream": r.cfg.Stream,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return xerrors.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
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
		return xerrors.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	duration := r.clock.Since(start)
	r.totalDuration += duration
	r.cfg.Metrics.ObserveDuration(duration.Seconds())

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return xerrors.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Handle response
	if r.cfg.Stream {
		return r.handleStreamingResponse(ctx, logger, resp)
	}

	return r.handleNonStreamingResponse(ctx, logger, resp)
}

func (r *Runner) handleNonStreamingResponse(ctx context.Context, logger slog.Logger, resp *http.Response) error {
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
		logger.Debug(ctx, "received response",
			slog.F("response_id", response.ID),
			slog.F("content_length", len(response.Choices[0].Message.Content)),
		)
	}

	// Track token usage if available
	if response.Usage.TotalTokens > 0 {
		r.totalTokens += int64(response.Usage.TotalTokens)
		r.cfg.Metrics.AddTokens("input", int64(response.Usage.PromptTokens))
		r.cfg.Metrics.AddTokens("output", int64(response.Usage.CompletionTokens))
	}

	return nil
}

func (r *Runner) handleStreamingResponse(ctx context.Context, logger slog.Logger, resp *http.Response) error {
	// For streaming, we just read until the stream ends
	// The mock server sends a simple stream format
	buf := make([]byte, 4096)
	totalRead := 0
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			totalRead += n
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return xerrors.Errorf("read stream: %w", err)
		}
	}

	logger.Debug(ctx, "received streaming response", slog.F("bytes_read", totalRead))
	return nil
}

func (r *Runner) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	// Only cleanup user in bridge mode
	if r.cfg.Mode == RequestModeBridge && r.createUserRunner != nil {
		_, _ = fmt.Fprintln(logs, "Cleaning up user...")
		if err := r.createUserRunner.Cleanup(ctx, id, logs); err != nil {
			return xerrors.Errorf("cleanup user: %w", err)
		}
	}

	return nil
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
