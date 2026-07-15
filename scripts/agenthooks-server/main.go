// agenthooks-server is a reference consumer for Coder agent lifecycle hooks.
// It logs each verified event as one JSON object per line. Plain HTTP is useful
// for local development or behind a TLS terminator, but coderd requires the
// configured lifecycle hook URL to use HTTPS.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/agenthooks"
)

type config struct {
	listen          string
	secret          string
	tlsCert         string
	tlsKey          string
	logOnly         bool
	denyToolPattern string
	redactPrompt    string
}

type eventLog struct {
	Event      agenthooks.EventType `json:"event"`
	ChatID     string               `json:"chat_id"`
	TurnID     string               `json:"turn_id,omitempty"`
	ToolUseID  string               `json:"tool_use_id,omitempty"`
	ToolName   string               `json:"tool_name,omitempty"`
	Source     string               `json:"source,omitempty"`
	Prompt     string               `json:"prompt,omitempty"`
	ToolInput  json.RawMessage      `json:"tool_input,omitempty"`
	ToolOutput json.RawMessage      `json:"tool_output,omitempty"`
	ToolError  string               `json:"tool_error,omitempty"`
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := parseFlags()
	if cfg.secret == "" {
		return xerrors.New("secret is required through --secret or CODER_AGENTHOOKS_SECRET")
	}
	if (cfg.tlsCert == "") != (cfg.tlsKey == "") {
		return xerrors.New("TLS certificate and key must be configured together")
	}

	denyTool, err := compileOptionalPattern("deny tool", cfg.denyToolPattern)
	if err != nil {
		return err
	}
	redactPrompt, err := compileOptionalPattern("redact prompt", cfg.redactPrompt)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(os.Stdout)
	logEvent := func(event eventLog) {
		if err := encoder.Encode(event); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error encoding event: %v\n", err)
		}
	}
	baseEvent := func(event agenthooks.EventType, meta agenthooks.Meta) eventLog {
		entry := eventLog{Event: event, ChatID: meta.ChatID.String()}
		if meta.TurnID != nil {
			entry.TurnID = meta.TurnID.String()
		}
		return entry
	}

	hooks := agenthooks.Hooks{
		SessionStart: func(_ context.Context, meta agenthooks.Meta, data agenthooks.SessionStartData) (agenthooks.Response, error) {
			entry := baseEvent(agenthooks.EventSessionStart, meta)
			entry.Source = data.Source
			logEvent(entry)
			return agenthooks.Response{}, nil
		},
		UserPromptSubmit: func(_ context.Context, meta agenthooks.Meta, data agenthooks.UserPromptSubmitData) (agenthooks.Response, error) {
			entry := baseEvent(agenthooks.EventUserPromptSubmit, meta)
			entry.Prompt = data.Prompt
			logEvent(entry)
			if cfg.logOnly || !redactPrompt.MatchString(data.Prompt) {
				return agenthooks.Response{}, nil
			}
			override, err := json.Marshal(map[string]string{"prompt": redactPrompt.ReplaceAllString(data.Prompt, "[REDACTED]")})
			if err != nil {
				return agenthooks.Response{}, xerrors.Errorf("marshal prompt override: %w", err)
			}
			return agenthooks.Response{Permission: &agenthooks.Permission{
				Decision:      agenthooks.PermissionAllow,
				InputOverride: override,
			}}, nil
		},
		PreToolUse: func(_ context.Context, meta agenthooks.Meta, data agenthooks.PreToolUseData) (agenthooks.Response, error) {
			entry := baseEvent(agenthooks.EventPreToolUse, meta)
			entry.ToolUseID = data.ToolUseID
			entry.ToolName = data.ToolName
			entry.ToolInput = data.ToolInput
			logEvent(entry)
			if cfg.logOnly || !denyTool.MatchString(data.ToolName) {
				return agenthooks.Response{}, nil
			}
			return agenthooks.Response{Permission: &agenthooks.Permission{
				Decision: agenthooks.PermissionDeny,
				Reason:   "tool name matched the configured deny pattern",
			}}, nil
		},
		PostToolUse: func(_ context.Context, meta agenthooks.Meta, data agenthooks.PostToolUseData) (agenthooks.Response, error) {
			entry := baseEvent(agenthooks.EventPostToolUse, meta)
			entry.ToolUseID = data.ToolUseID
			entry.ToolName = data.ToolName
			entry.ToolOutput = data.ToolResponse
			entry.ToolError = data.ToolError
			logEvent(entry)
			return agenthooks.Response{}, nil
		},
		PreCompact: func(_ context.Context, meta agenthooks.Meta, _ agenthooks.PreCompactData) (agenthooks.Response, error) {
			logEvent(baseEvent(agenthooks.EventPreCompact, meta))
			return agenthooks.Response{}, nil
		},
		PostCompact: func(_ context.Context, meta agenthooks.Meta, _ agenthooks.PostCompactData) (agenthooks.Response, error) {
			logEvent(baseEvent(agenthooks.EventPostCompact, meta))
			return agenthooks.Response{}, nil
		},
		Stop: func(_ context.Context, meta agenthooks.Meta, _ agenthooks.StopData) (agenthooks.Response, error) {
			logEvent(baseEvent(agenthooks.EventStop, meta))
			return agenthooks.Response{}, nil
		},
	}

	server := &http.Server{
		Addr:              cfg.listen,
		Handler:           agenthooks.NewHTTPHandler([]byte(cfg.secret), hooks),
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()

	_, _ = fmt.Fprintf(os.Stdout, "Agent hooks server listening on %s\n", cfg.listen)
	if cfg.tlsCert != "" {
		err = server.ListenAndServeTLS(cfg.tlsCert, cfg.tlsKey)
	} else {
		err = server.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return xerrors.Errorf("serve lifecycle hooks: %w", err)
	}
	return nil
}

func parseFlags() config {
	cfg := config{}
	flag.StringVar(&cfg.listen, "listen", envOrDefault("CODER_AGENTHOOKS_LISTEN", "127.0.0.1:8081"), "Listen address (CODER_AGENTHOOKS_LISTEN)")
	flag.StringVar(&cfg.secret, "secret", os.Getenv("CODER_AGENTHOOKS_SECRET"), "Shared HS256 secret, required (CODER_AGENTHOOKS_SECRET)")
	flag.StringVar(&cfg.tlsCert, "tls-cert", os.Getenv("CODER_AGENTHOOKS_TLS_CERT"), "TLS certificate path (CODER_AGENTHOOKS_TLS_CERT)")
	flag.StringVar(&cfg.tlsKey, "tls-key", os.Getenv("CODER_AGENTHOOKS_TLS_KEY"), "TLS private key path (CODER_AGENTHOOKS_TLS_KEY)")
	flag.BoolVar(&cfg.logOnly, "log-only", envBool("CODER_AGENTHOOKS_LOG_ONLY", true), "Return an empty response for every event (CODER_AGENTHOOKS_LOG_ONLY)")
	flag.StringVar(&cfg.denyToolPattern, "deny-tool-pattern", os.Getenv("CODER_AGENTHOOKS_DENY_TOOL_PATTERN"), "Example regexp for denied tool names (CODER_AGENTHOOKS_DENY_TOOL_PATTERN)")
	flag.StringVar(&cfg.redactPrompt, "redact-prompt-pattern", os.Getenv("CODER_AGENTHOOKS_REDACT_PROMPT_PATTERN"), "Example regexp to redact in prompts (CODER_AGENTHOOKS_REDACT_PROMPT_PATTERN)")
	flag.Parse()
	return cfg
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

type optionalPattern struct {
	regexp *regexp.Regexp
}

func (p optionalPattern) MatchString(value string) bool {
	return p.regexp != nil && p.regexp.MatchString(value)
}

func (p optionalPattern) ReplaceAllString(value, replacement string) string {
	if p.regexp == nil {
		return value
	}
	return p.regexp.ReplaceAllString(value, replacement)
}

func compileOptionalPattern(name, pattern string) (optionalPattern, error) {
	if pattern == "" {
		return optionalPattern{}, nil
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return optionalPattern{}, xerrors.Errorf("compile %s pattern: %w", name, err)
	}
	return optionalPattern{regexp: compiled}, nil
}
