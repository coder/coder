package bridge

import (
	"context"
	"fmt"
	"io"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scaletest/createusers"
)

type requestModeStrategy interface {
	Setup(ctx context.Context, id string, logs io.Writer) (url string, token string, err error)
	Cleanup(ctx context.Context, id string, logs io.Writer) error
}

// bridgeStrategy creates users via Coder and routes requests through AI Bridge.
type bridgeStrategy struct {
	client   *codersdk.Client
	provider string
	metrics  *Metrics

	userConfig       createusers.Config
	createUserRunner *createusers.Runner
}

type bridgeStrategyConfig struct {
	Client   *codersdk.Client
	Provider string
	Metrics  *Metrics
	User     createusers.Config
}

func newBridgeStrategy(cfg bridgeStrategyConfig) *bridgeStrategy {
	return &bridgeStrategy{
		client:     cfg.Client,
		provider:   cfg.Provider,
		metrics:    cfg.Metrics,
		userConfig: cfg.User,
	}
}

func (s *bridgeStrategy) Setup(ctx context.Context, id string, logs io.Writer) (requestURL string, token string, err error) {
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)

	s.client.SetLogger(logger)
	s.client.SetLogBodies(true)

	s.createUserRunner = createusers.NewRunner(s.client, s.userConfig)
	newUserAndToken, err := s.createUserRunner.RunReturningUser(ctx, id, logs)
	if err != nil {
		s.metrics.AddError("create_user")
		return "", "", xerrors.Errorf("create user: %w", err)
	}
	newUser := newUserAndToken.User
	token = newUserAndToken.SessionToken

	logger.Info(ctx, "runner user created",
		slog.F("username", newUser.Username),
		slog.F("user_id", newUser.ID.String()),
	)

	switch s.provider {
	case "messages":
		requestURL = fmt.Sprintf("%s/api/v2/aibridge/anthropic/v1/messages", s.client.URL)
	case "responses":
		requestURL = fmt.Sprintf("%s/api/v2/aibridge/openai/v1/responses", s.client.URL)
	case "completions":
		requestURL = fmt.Sprintf("%s/api/v2/aibridge/openai/v1/chat/completions", s.client.URL)
	}
	logger.Info(ctx, "bridge runner in bridge mode",
		slog.F("url", requestURL),
		slog.F("provider", s.provider),
	)

	return requestURL, token, nil
}

func (s *bridgeStrategy) Cleanup(ctx context.Context, id string, logs io.Writer) error {
	if s.createUserRunner == nil {
		return nil
	}

	_, _ = fmt.Fprintln(logs, "Cleaning up user...")
	if err := s.createUserRunner.Cleanup(ctx, id, logs); err != nil {
		return xerrors.Errorf("cleanup user: %w", err)
	}
	return nil
}

// directStrategy makes requests directly to an upstream URL.
type directStrategy struct {
	upstreamURL string
}

type directStrategyConfig struct {
	UpstreamURL string
}

func newDirectStrategy(cfg directStrategyConfig) *directStrategy {
	return &directStrategy{
		upstreamURL: cfg.UpstreamURL,
	}
}

func (s *directStrategy) Setup(ctx context.Context, _ string, logs io.Writer) (requestURL string, _ string, err error) {
	logger := slog.Make(sloghuman.Sink(logs)).Leveled(slog.LevelDebug)

	logger.Info(ctx, "bridge runner in direct mode", slog.F("url", s.upstreamURL))
	return s.upstreamURL, "", err
}

func (*directStrategy) Cleanup(_ context.Context, _ string, _ io.Writer) error {
	// Direct mode has no resources to clean up.
	return nil
}
