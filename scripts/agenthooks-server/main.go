// agenthooks-server is a reference consumer for Coder agent lifecycle hooks.
// It logs each verified event as one JSON object per line. The SDK handler
// honors X-Forwarded-Proto, so a TLS terminator can forward plain HTTP and
// the HTTPS audience required by coderd still verifies.
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
	"sync"
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
	DispatchID string               `json:"dispatch_id"`
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
	cfg, err := parseFlags()
	if err != nil {
		return err
	}
	if cfg.secret == "" {
		return xerrors.New("secret is required through --secret or CODER_AGENTHOOKS_SECRET")
	}
	if (cfg.tlsCert == "") != (cfg.tlsKey == "") {
		return xerrors.New("TLS certificate and key must be configured together")
	}

	var denyTool *regexp.Regexp
	if cfg.denyToolPattern != "" {
		denyTool, err = regexp.Compile(cfg.denyToolPattern)
		if err != nil {
			return xerrors.Errorf("compile deny tool pattern: %w", err)
		}
	}
	var redactPrompt *regexp.Regexp
	if cfg.redactPrompt != "" {
		redactPrompt, err = regexp.Compile(cfg.redactPrompt)
		if err != nil {
			return xerrors.Errorf("compile redact prompt pattern: %w", err)
		}
	}

	var logMu sync.Mutex
	encoder := json.NewEncoder(os.Stdout)
	logEvent := func(event eventLog) error {
		logMu.Lock()
		defer logMu.Unlock()
		if err := encoder.Encode(event); err != nil {
			return xerrors.Errorf("encode event: %w", err)
		}
		return nil
	}
	baseEvent := func(event agenthooks.EventType, meta agenthooks.Meta) eventLog {
		entry := eventLog{
			Event:      event,
			DispatchID: meta.DispatchID.String(),
			ChatID:     meta.ChatID.String(),
		}
		if meta.TurnID != nil {
			entry.TurnID = meta.TurnID.String()
		}
		return entry
	}

	hooks := agenthooks.Hooks{
		SessionStart: func(_ context.Context, meta agenthooks.Meta, data agenthooks.SessionStartData) (agenthooks.Response, error) {
			entry := baseEvent(agenthooks.EventSessionStart, meta)
			entry.Source = data.Source
			return agenthooks.Response{}, logEvent(entry)
		},
		UserPromptSubmit: func(_ context.Context, meta agenthooks.Meta, data agenthooks.UserPromptSubmitData) (agenthooks.Response, error) {
			entry := baseEvent(agenthooks.EventUserPromptSubmit, meta)
			entry.Prompt = data.Prompt
			matches := !cfg.logOnly && redactPrompt != nil && redactPrompt.MatchString(data.Prompt)
			if matches {
				entry.Prompt = redactPrompt.ReplaceAllString(data.Prompt, "[REDACTED]")
			}
			if err := logEvent(entry); err != nil {
				return agenthooks.Response{}, err
			}
			if !matches {
				return agenthooks.Response{}, nil
			}
			override, err := json.Marshal(map[string]string{"prompt": entry.Prompt})
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
			if err := logEvent(entry); err != nil {
				return agenthooks.Response{}, err
			}
			if cfg.logOnly || denyTool == nil || !denyTool.MatchString(data.ToolName) {
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
			return agenthooks.Response{}, logEvent(entry)
		},
		PreCompact: func(_ context.Context, meta agenthooks.Meta, _ agenthooks.PreCompactData) (agenthooks.Response, error) {
			return agenthooks.Response{}, logEvent(baseEvent(agenthooks.EventPreCompact, meta))
		},
		PostCompact: func(_ context.Context, meta agenthooks.Meta, _ agenthooks.PostCompactData) (agenthooks.Response, error) {
			return agenthooks.Response{}, logEvent(baseEvent(agenthooks.EventPostCompact, meta))
		},
		Stop: func(_ context.Context, meta agenthooks.Meta, _ agenthooks.StopData) (agenthooks.Response, error) {
			return agenthooks.Response{}, logEvent(baseEvent(agenthooks.EventStop, meta))
		},
	}

	handler := agenthooks.NewHTTPHandler([]byte(cfg.secret), hooks)
	server := &http.Server{
		Addr:              cfg.listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()

	_, _ = fmt.Fprintf(os.Stderr, "Agent hooks server listening on %s\n", cfg.listen)
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

func parseFlags() (config, error) {
	logOnly, err := envBool("CODER_AGENTHOOKS_LOG_ONLY", true)
	if err != nil {
		return config{}, err
	}
	var cfg config
	cfg.logOnly = logOnly
	flag.StringVar(&cfg.listen, "listen", envOrDefault("CODER_AGENTHOOKS_LISTEN", "127.0.0.1:8081"), "Listen address (CODER_AGENTHOOKS_LISTEN)")
	flag.StringVar(&cfg.secret, "secret", os.Getenv("CODER_AGENTHOOKS_SECRET"), "Shared HS256 secret, required (CODER_AGENTHOOKS_SECRET)")
	flag.StringVar(&cfg.tlsCert, "tls-cert", os.Getenv("CODER_AGENTHOOKS_TLS_CERT"), "TLS certificate path (CODER_AGENTHOOKS_TLS_CERT)")
	flag.StringVar(&cfg.tlsKey, "tls-key", os.Getenv("CODER_AGENTHOOKS_TLS_KEY"), "TLS private key path (CODER_AGENTHOOKS_TLS_KEY)")
	flag.BoolVar(&cfg.logOnly, "log-only", cfg.logOnly, "Return an empty response for every event (CODER_AGENTHOOKS_LOG_ONLY)")
	flag.StringVar(&cfg.denyToolPattern, "deny-tool-pattern", os.Getenv("CODER_AGENTHOOKS_DENY_TOOL_PATTERN"), "Example regexp for denied tool names (CODER_AGENTHOOKS_DENY_TOOL_PATTERN)")
	flag.StringVar(&cfg.redactPrompt, "redact-prompt-pattern", os.Getenv("CODER_AGENTHOOKS_REDACT_PROMPT_PATTERN"), "Example regexp to redact in prompts (CODER_AGENTHOOKS_REDACT_PROMPT_PATTERN)")
	flag.Parse()
	return cfg, nil
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) (bool, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, xerrors.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}
