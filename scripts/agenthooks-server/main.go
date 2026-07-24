// agenthooks-server is a reference consumer that logs verified lifecycle
// events as JSON.
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

	"github.com/coder/coder/v2/coderd/x/hooks"
)

type config struct {
	listen          string
	secret          string
	issuer          string
	tlsCert         string
	tlsKey          string
	logOnly         bool
	denyToolPattern string
	redactPrompt    string
}

type eventLog struct {
	Event      hooks.EventType `json:"event"`
	DispatchID string          `json:"dispatch_id"`
	ChatID     string          `json:"chat_id"`
	TurnID     string          `json:"turn_id,omitempty"`
	ToolUseID  string          `json:"tool_use_id,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
	Source     string          `json:"source,omitempty"`
	Prompt     string          `json:"prompt,omitempty"`
	ToolInput  json.RawMessage `json:"tool_input,omitempty"`
	ToolOutput json.RawMessage `json:"tool_output,omitempty"`
	ToolError  string          `json:"tool_error,omitempty"`
	Duplicate  bool            `json:"duplicate,omitempty"`
}

// consumerState demonstrates consumer-owned hook state. Coder persists no
// hook decisions and delivery is at-least-once, so consumers that need
// memory keep it themselves, keyed by the stable payload identifiers:
// chat_id, the event type, and tool_use_id.
type consumerState struct {
	mu sync.Mutex
	// preToolDecisions reuses responses for duplicate (chat_id, tool_use_id)
	// deliveries while they remain cached.
	preToolDecisions map[string]hooks.Response
	// blockedTools records tool names this consumer denied per chat, so
	// the policy outlives any single dispatch.
	blockedTools map[string]map[string]struct{}
}

const maxRememberedDecisions = 8192

func newConsumerState() *consumerState {
	return &consumerState{
		preToolDecisions: make(map[string]hooks.Response),
		blockedTools:     make(map[string]map[string]struct{}),
	}
}

func (s *consumerState) rememberedDecision(chatID, toolUseID string) (hooks.Response, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	response, ok := s.preToolDecisions[chatID+"\x00"+toolUseID]
	return response, ok
}

func (s *consumerState) rememberDecision(chatID, toolUseID string, response hooks.Response, deniedTool string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.preToolDecisions) >= maxRememberedDecisions {
		s.preToolDecisions = make(map[string]hooks.Response)
	}
	s.preToolDecisions[chatID+"\x00"+toolUseID] = response
	if deniedTool == "" {
		return
	}
	blocked := s.blockedTools[chatID]
	if blocked == nil {
		blocked = make(map[string]struct{})
		s.blockedTools[chatID] = blocked
	}
	blocked[deniedTool] = struct{}{}
}

func (s *consumerState) isBlocked(chatID, toolName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.blockedTools[chatID][toolName]
	return ok
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

	state := newConsumerState()
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
	baseEvent := func(event hooks.EventType, meta hooks.Meta) eventLog {
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

	consumerHooks := hooks.Hooks{
		SessionStart: func(_ context.Context, meta hooks.Meta, data hooks.SessionStartData) (hooks.Response, error) {
			entry := baseEvent(hooks.EventSessionStart, meta)
			entry.Source = data.Source
			return hooks.Response{}, logEvent(entry)
		},
		UserPromptSubmit: func(_ context.Context, meta hooks.Meta, data hooks.UserPromptSubmitData) (hooks.Response, error) {
			entry := baseEvent(hooks.EventUserPromptSubmit, meta)
			entry.Prompt = data.Prompt
			matches := redactPrompt != nil && redactPrompt.MatchString(data.Prompt)
			if matches {
				entry.Prompt = redactPrompt.ReplaceAllString(data.Prompt, "[REDACTED]")
			}
			if err := logEvent(entry); err != nil {
				return hooks.Response{}, err
			}
			// Log-only mode still redacts the log entry above; it only
			// suppresses the prompt override response.
			if cfg.logOnly || !matches {
				return hooks.Response{}, nil
			}
			override, err := json.Marshal(map[string]string{"prompt": entry.Prompt})
			if err != nil {
				return hooks.Response{}, xerrors.Errorf("marshal prompt override: %w", err)
			}
			return hooks.Response{Permission: &hooks.Permission{
				Decision:      hooks.PermissionAllow,
				InputOverride: override,
			}}, nil
		},
		PreToolUse: func(_ context.Context, meta hooks.Meta, data hooks.PreToolUseData) (hooks.Response, error) {
			entry := baseEvent(hooks.EventPreToolUse, meta)
			entry.ToolUseID = data.ToolUseID
			entry.ToolName = data.ToolName
			entry.ToolInput = data.ToolInput
			if response, ok := state.rememberedDecision(entry.ChatID, data.ToolUseID); ok {
				entry.Duplicate = true
				return response, logEvent(entry)
			}
			if err := logEvent(entry); err != nil {
				return hooks.Response{}, err
			}
			var response hooks.Response
			deniedTool := ""
			switch {
			case cfg.logOnly:
			case state.isBlocked(entry.ChatID, data.ToolName):
				response = hooks.Response{Permission: &hooks.Permission{
					Decision: hooks.PermissionDeny,
					Reason:   "tool is blocked for this chat",
				}}
			case denyTool != nil && denyTool.MatchString(data.ToolName):
				deniedTool = data.ToolName
				response = hooks.Response{Permission: &hooks.Permission{
					Decision: hooks.PermissionDeny,
					Reason:   "tool name matched the configured deny pattern",
				}}
			}
			state.rememberDecision(entry.ChatID, data.ToolUseID, response, deniedTool)
			return response, nil
		},
		PostToolUse: func(_ context.Context, meta hooks.Meta, data hooks.PostToolUseData) (hooks.Response, error) {
			entry := baseEvent(hooks.EventPostToolUse, meta)
			entry.ToolUseID = data.ToolUseID
			entry.ToolName = data.ToolName
			entry.ToolOutput = data.ToolResponse
			entry.ToolError = data.ToolError
			return hooks.Response{}, logEvent(entry)
		},
		PreCompact: func(_ context.Context, meta hooks.Meta, _ hooks.PreCompactData) (hooks.Response, error) {
			return hooks.Response{}, logEvent(baseEvent(hooks.EventPreCompact, meta))
		},
		PostCompact: func(_ context.Context, meta hooks.Meta, _ hooks.PostCompactData) (hooks.Response, error) {
			return hooks.Response{}, logEvent(baseEvent(hooks.EventPostCompact, meta))
		},
		Stop: func(_ context.Context, meta hooks.Meta, _ hooks.StopData) (hooks.Response, error) {
			return hooks.Response{}, logEvent(baseEvent(hooks.EventStop, meta))
		},
	}

	var handlerOpts []hooks.HandlerOption
	if cfg.issuer != "" {
		handlerOpts = append(handlerOpts, hooks.WithExpectedIssuer(cfg.issuer))
	}
	handler := hooks.NewHTTPHandler([]byte(cfg.secret), consumerHooks, handlerOpts...)
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
	flag.StringVar(&cfg.issuer, "issuer", os.Getenv("CODER_AGENTHOOKS_ISSUER"), "Expected iss claim, normally the Coder deployment ID (CODER_AGENTHOOKS_ISSUER)")
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
