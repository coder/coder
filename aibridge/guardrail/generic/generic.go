// Package generic implements a guardrail backed by any vendor that speaks
// litellm's Generic Guardrail API: extract texts -> POST -> one of BLOCKED /
// NONE / GUARDRAIL_INTERVENED, with intervened responses returning modified
// texts positionally aligned to the request. Any generic-API-compatible vendor
// (e.g. Pillar Security) works with zero per-vendor code by pointing this
// adapter at its endpoint.
//
// Contract: https://docs.litellm.ai/docs/adding_provider/generic_guardrail_api
package generic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/guardrail"
)

// defaultPath is litellm's Generic Guardrail API endpoint suffix.
const defaultPath = "/beta/litellm_basic_guardrail_api"

// Scope selects which extracted spans are sent to the vendor.
type Scope string

const (
	// ScopePrompt sends only the latest user prompt (masking-safe default).
	ScopePrompt Scope = "prompt"
	// ScopeConversation sends every role-tagged span (context-needing scanners).
	ScopeConversation Scope = "conversation"
)

// Config configures a generic-API guardrail.
type Config struct {
	// Name identifies the guardrail (namespaces annotations, labels metrics).
	Name string
	// APIBase is the vendor's base URL; the adapter POSTs to APIBase + Path.
	APIBase string
	// Path overrides the endpoint suffix (default defaultPath).
	Path string
	// Scope selects prompt-only (default) vs full-conversation extraction.
	Scope Scope
	// Headers are static headers sent on every request (e.g. a vendor key).
	Headers map[string]string
	// Params are forwarded verbatim as additional_provider_specific_params.
	Params map[string]any
	// HTTPClient defaults to http.DefaultClient.
	HTTPClient *http.Client
}

// jsonConfig is the persisted JSON shape of a generic guardrail's config.
type jsonConfig struct {
	APIBase string            `json:"api_base"`
	Path    string            `json:"path"`
	Scope   Scope             `json:"scope"`
	Headers map[string]string `json:"headers"`
	Params  map[string]any    `json:"params"`
}

// Guardrail is a generic-API-backed [guardrail.Guardrail].
type Guardrail struct {
	cfg        Config
	credential string
	client     *http.Client
}

var _ guardrail.Guardrail = (*Guardrail)(nil)

// NewFromConfig builds a generic guardrail from its persisted JSON config and
// decrypted credential (sent as a bearer token when non-empty). It is the shared
// entry point for both config validation (API) and runtime construction.
func NewFromConfig(name string, raw []byte, credential string) (*Guardrail, error) {
	var c jsonConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, xerrors.Errorf("parse generic guardrail config: %w", err)
	}
	return New(Config{
		Name:    name,
		APIBase: c.APIBase,
		Path:    c.Path,
		Scope:   c.Scope,
		Headers: c.Headers,
		Params:  c.Params,
	}, credential)
}

// New validates cfg and returns a ready guardrail.
func New(cfg Config, credential string) (*Guardrail, error) {
	if cfg.Name == "" {
		return nil, xerrors.New("generic guardrail: name required")
	}
	if cfg.APIBase == "" {
		return nil, xerrors.New("generic guardrail: api_base required")
	}
	if cfg.Path == "" {
		cfg.Path = defaultPath
	}
	switch cfg.Scope {
	case "":
		cfg.Scope = ScopePrompt
	case ScopePrompt, ScopeConversation:
	default:
		return nil, xerrors.Errorf("generic guardrail: unknown scope %q", cfg.Scope)
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Guardrail{cfg: cfg, credential: credential, client: client}, nil
}

// Name implements guardrail.Guardrail.
func (g *Guardrail) Name() string { return g.cfg.Name }

// wireReq is the litellm Generic Guardrail API request body.
type wireReq struct {
	Texts              []string       `json:"texts"`
	StructuredMessages []wireMsg      `json:"structured_messages,omitempty"`
	RequestData        map[string]any `json:"request_data"`
	InputType          string         `json:"input_type"`
	Params             map[string]any `json:"additional_provider_specific_params,omitempty"`
}

type wireMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// wireResp is the litellm Generic Guardrail API response body.
type wireResp struct {
	Action        string   `json:"action"`
	BlockedReason string   `json:"blocked_reason"`
	Texts         []string `json:"texts"`
}

// Evaluate sends the scoped spans to the vendor and maps the response onto a
// guardrail.Result. GUARDRAIL_INTERVENED responses are zipped back to the
// originating sjson pointers positionally (litellm's contract), so only changed
// spans become edits.
func (g *Guardrail) Evaluate(ctx context.Context, req guardrail.Request) (guardrail.Result, error) {
	spans := req.Prompt
	if g.cfg.Scope == ScopeConversation {
		spans = req.Conversation
	}

	in := wireReq{
		InputType:   "request",
		Params:      g.cfg.Params,
		RequestData: identityData(req.Identity),
	}
	for _, s := range spans {
		in.Texts = append(in.Texts, s.Value)
		in.StructuredMessages = append(in.StructuredMessages, wireMsg{Role: roleOrUser(s.Role), Content: s.Value})
	}

	var out wireResp
	if err := g.post(ctx, in, &out); err != nil {
		return guardrail.Result{}, err
	}

	switch strings.ToUpper(out.Action) {
	case "BLOCKED":
		reason := out.BlockedReason
		if reason == "" {
			reason = "request blocked by guardrail"
		}
		return guardrail.Result{Action: guardrail.ActionBlock, Reason: reason}, nil
	case "GUARDRAIL_INTERVENED":
		var res guardrail.Result
		for i, s := range spans {
			if i < len(out.Texts) && out.Texts[i] != s.Value {
				res.Edits = append(res.Edits, guardrail.Edit{Pointer: s.Pointer, Value: out.Texts[i]})
			}
		}
		return res, nil
	default: // NONE or unrecognized: proceed unchanged.
		return guardrail.Result{}, nil
	}
}

func (g *Guardrail) post(ctx context.Context, in, out any) error {
	payload, err := json.Marshal(in)
	if err != nil {
		return xerrors.Errorf("marshal request: %w", err)
	}
	url := strings.TrimRight(g.cfg.APIBase, "/") + g.cfg.Path
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return xerrors.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if g.credential != "" {
		httpReq.Header.Set("Authorization", "Bearer "+g.credential)
	}
	for k, v := range g.cfg.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return xerrors.Errorf("unexpected status %s: %s", resp.Status, strings.TrimSpace(string(snippet)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return xerrors.Errorf("decode response: %w", err)
	}
	return nil
}

// identityData mirrors litellm's request_data block (a subset).
func identityData(id guardrail.Identity) map[string]any {
	return map[string]any{
		"user_api_key_user_id": id.ID,
		"user_id":              id.Username,
		"groups":               id.Groups,
		"roles":                id.Roles,
	}
}

func roleOrUser(r guardrail.Role) string {
	if r == "" {
		return string(guardrail.RoleUser)
	}
	return string(r)
}
