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

	"github.com/coder/coder/v2/codersdk/x/agenthooks"
)

type config struct {
	listen          string
	audience        string
	secret          string
	issuer          string
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
	Duplicate  bool                 `json:"duplicate,omitempty"`
}

// consumerState demonstrates consumer-owned hook state. Coder persists no
// hook decisions and delivery is best-effort, so consumers that need memory
// keep it themselves, keyed by the stable payload identifiers: chat_id, the
// event type, and tool_use_id.
type consumerState struct {
	mu sync.Mutex
	// preToolDecisions reuses responses for duplicate (chat_id, tool_use_id)
	// deliveries while they remain cached.
	preToolDecisions map[string]agenthooks.Response
	// blockedTools records tool names this consumer denied per chat, so
	// the policy outlives any single dispatch. Evicted with preToolDecisions.
	blockedTools map[string]map[string]struct{}
}

const maxRememberedDecisions = 8192

func newConsumerState() *consumerState {
	return &consumerState{
		preToolDecisions: make(map[string]agenthooks.Response),
		blockedTools:     make(map[string]map[string]struct{}),
	}
}

// decidePreToolUse resolves one pre_tool_use delivery and reports whether the
// response was already remembered. The lookup, the policy decision, and the
// store share one lock, so concurrent duplicate deliveries of the same
// tool_use_id cannot each decide and then overwrite each other.
func (s *consumerState) decidePreToolUse(chatID, toolUseID, toolName string, logOnly bool, denyTool *regexp.Regexp) (agenthooks.Response, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := chatID + "\x00" + toolUseID
	if response, ok := s.preToolDecisions[key]; ok {
		return response, true
	}

	var response agenthooks.Response
	deniedTool := ""
	switch {
	case logOnly:
	case s.isBlockedLocked(chatID, toolName):
		response = agenthooks.Response{Permission: &agenthooks.Permission{
			Decision: agenthooks.PermissionDeny,
			Reason:   "use of this tool is blocked for this chat",
		}}
	case denyTool != nil && denyTool.MatchString(toolName):
		deniedTool = toolName
		response = agenthooks.Response{Permission: &agenthooks.Permission{
			Decision: agenthooks.PermissionDeny,
			Reason:   "use of this tool is denied by this deployment's policy",
		}}
	}

	if len(s.preToolDecisions) >= maxRememberedDecisions {
		// Both maps grow per chat, so evict them together to keep a
		// long-running consumer bounded.
		s.preToolDecisions = make(map[string]agenthooks.Response)
		s.blockedTools = make(map[string]map[string]struct{})
	}
	s.preToolDecisions[key] = response
	if deniedTool != "" {
		blocked := s.blockedTools[chatID]
		if blocked == nil {
			blocked = make(map[string]struct{})
			s.blockedTools[chatID] = blocked
		}
		blocked[deniedTool] = struct{}{}
	}
	return response, false
}

func (s *consumerState) isBlockedLocked(chatID, toolName string) bool {
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
	// The bind address is not the audience: it may be a wildcard or an
	// ephemeral port, and behind a proxy the signed audience is the proxy URL.
	if cfg.audience == "" {
		return xerrors.New("audience is required through --audience or CODER_AGENTHOOKS_AUDIENCE")
	}
	if len(cfg.secret) < agenthooks.MinSecretLen {
		return xerrors.Errorf("secret must be at least %d bytes", agenthooks.MinSecretLen)
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

	consumerHooks := agenthooks.Hooks{
		SessionStart: func(_ context.Context, meta agenthooks.Meta, data agenthooks.SessionStartData) (agenthooks.Response, error) {
			entry := baseEvent(agenthooks.EventSessionStart, meta)
			entry.Source = data.Source
			return agenthooks.Response{}, logEvent(entry)
		},
		UserPromptSubmit: func(_ context.Context, meta agenthooks.Meta, data agenthooks.UserPromptSubmitData) (agenthooks.Response, error) {
			entry := baseEvent(agenthooks.EventUserPromptSubmit, meta)
			entry.Prompt = data.Prompt
			matches := redactPrompt != nil && redactPrompt.MatchString(data.Prompt)
			if matches {
				entry.Prompt = redactPrompt.ReplaceAllString(data.Prompt, "[REDACTED]")
			}
			if err := logEvent(entry); err != nil {
				return agenthooks.Response{}, err
			}
			// Log-only mode still redacts the log entry above; it only
			// suppresses the prompt override response.
			if cfg.logOnly || !matches {
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
			response, duplicate := state.decidePreToolUse(entry.ChatID, data.ToolUseID, data.ToolName, cfg.logOnly, denyTool)
			entry.Duplicate = duplicate
			return response, logEvent(entry)
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

	var handlerOpts []agenthooks.HandlerOption
	if cfg.issuer != "" {
		handlerOpts = append(handlerOpts, agenthooks.WithExpectedIssuer(cfg.issuer))
	}
	handler := agenthooks.NewHTTPHandler([]byte(cfg.secret), cfg.audience, consumerHooks, handlerOpts...)
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

	mode := "enforcing"
	if cfg.logOnly {
		mode = "log-only"
	}
	_, _ = fmt.Fprintf(os.Stderr, "Agent hooks server listening on %s in %s mode\n", cfg.listen, mode)
	if cfg.logOnly && (cfg.denyToolPattern != "" || cfg.redactPrompt != "") {
		_, _ = fmt.Fprintln(os.Stderr, "Warning: log-only mode ignores -deny-tool-pattern and -redact-prompt-pattern; pass -log-only=false to act on them")
	}
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
	flag.StringVar(&cfg.audience, "audience", os.Getenv("CODER_AGENTHOOKS_AUDIENCE"), "Expected aud claim, which is the deployment's CODER_CHAT_HOOK_URL, required (CODER_AGENTHOOKS_AUDIENCE)")
	flag.StringVar(&cfg.secret, "secret", os.Getenv("CODER_AGENTHOOKS_SECRET"), "Shared HS256 secret, required (CODER_AGENTHOOKS_SECRET)")
	flag.StringVar(&cfg.issuer, "issuer", os.Getenv("CODER_AGENTHOOKS_ISSUER"), "Expected iss claim, normally the Coder deployment ID (CODER_AGENTHOOKS_ISSUER)")
	flag.StringVar(&cfg.tlsCert, "tls-cert", os.Getenv("CODER_AGENTHOOKS_TLS_CERT"), "TLS certificate path (CODER_AGENTHOOKS_TLS_CERT)")
	flag.StringVar(&cfg.tlsKey, "tls-key", os.Getenv("CODER_AGENTHOOKS_TLS_KEY"), "TLS private key path (CODER_AGENTHOOKS_TLS_KEY)")
	flag.BoolVar(&cfg.logOnly, "log-only", cfg.logOnly, "Return an empty response for every event, on by default (CODER_AGENTHOOKS_LOG_ONLY)")
	flag.StringVar(&cfg.denyToolPattern, "deny-tool-pattern", os.Getenv("CODER_AGENTHOOKS_DENY_TOOL_PATTERN"), "Example regexp for denied tool names, requires -log-only=false (CODER_AGENTHOOKS_DENY_TOOL_PATTERN)")
	flag.StringVar(&cfg.redactPrompt, "redact-prompt-pattern", os.Getenv("CODER_AGENTHOOKS_REDACT_PROMPT_PATTERN"), "Example regexp to redact in prompts, requires -log-only=false to override the prompt (CODER_AGENTHOOKS_REDACT_PROMPT_PATTERN)")
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
