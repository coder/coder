# BYOK Implementation

## Problem Statement

AI Bridge currently requires a centralized LLM provider key configured by
admins (`CODER_AIBRIDGE_ANTHROPIC_KEY`). Users have no way to
supply their own key. The existing authentication convention
(`ANTHROPIC_API_KEY=<coder-session-token>`) cannot be changed without breaking
existing clients.

## Requirements

### Initial Functional Requirements

- Users can optionally supply their own LLM provider API keys.
- When a user key is present, it is used instead of the centralized key.
- When no user key is present, the system falls back to the centralized key —
  existing behavior is unchanged.
- It applies to both intercepted routes (`/v1/messages`) and
  passthrough routes (`/v1/models`, etc.).
- The existing `ANTHROPIC_API_KEY=<coder-session-token>` convention is
  preserved.
- The user-supplied `X-Coder-LLM-Key` header is stripped before forwarding and never reaches the
  upstream provider as a custom header.
- Admin policy to allow or disallow BYOK at the organization level.

### Initial Non-functional Requirements

- No overhead for users not using BYOK — the centralized code path is
  unchanged.
- The user-supplied key must not appear in logs or be forwarded upstream in any
  header other than the provider's standard auth header.

## Mechanism

The existing auth convention is preserved: `ANTHROPIC_API_KEY` is set to the
user's Coder session token, which AI Bridge uses to authenticate the request.

The real Anthropic API key is passed separately via a dedicated header,
`X-Coder-LLM-Key`. When this header is present, AI Bridge uses it as the
upstream key instead of the centralized `CODER_AIBRIDGE_ANTHROPIC_KEY`. When
it is absent, the bridge falls back to the centralized key.

```
Claude Code
  │  ANTHROPIC_API_KEY=<coder-session-token>         ← Coder auth (unchanged)
  │  ANTHROPIC_CUSTOM_HEADERS="X-Coder-LLM-Key: sk-ant-..."
  ▼
AI Bridge
  1. Extracts and validates Coder session token from X-Api-Key
  2. Checks for X-Coder-LLM-Key header
     ├─ Present  → uses it as the upstream key, strips it before forwarding
     └─ Absent   → falls back to CODER_AIBRIDGE_ANTHROPIC_KEY
  ▼
Anthropic API
```

## Required Changes

There are two independent code paths that both inject the upstream provider key,
and both need to be updated.

### Bridged routes (`/v1/messages`)

For intercepted routes, the provider key is not injected via an HTTP header at
call time. Instead, it is baked into `config.Anthropic` when the provider is
constructed in `enterprise/cli/aibridged.go`:

```go
aibridge.NewAnthropicProvider(aibridge.AnthropicConfig{
    Key: cfg.Anthropic.Key.String(), // centralized key set here at startup
    ...
})
```

This config is passed through `CreateInterceptor` → `NewStreamingInterceptor` /
`NewBlockingInterceptor` → `interceptionBase`, where it is consumed by
`newMessagesService` in `intercept/messages/base.go`:

```go
func (i *interceptionBase) newMessagesService(ctx context.Context, opts ...option.RequestOption) (anthropic.MessageService, error) {
    opts = append(opts, option.WithAPIKey(i.cfg.Key)) // ← centralized key injected here
    ...
}
```

To support BYOK, `CreateInterceptor` (in `provider/anthropic.go`) must extract
the `X-Coder-LLM-Key` header from the incoming request before it is
forwarded, and pass a modified config with the per-user key to the interceptor
constructors:

```go
func (p *Anthropic) CreateInterceptor(w http.ResponseWriter, r *http.Request, tracer trace.Tracer) (_ intercept.Interceptor, outErr error) {
    cfg := p.cfg
    if byok := r.Header.Get("X-Coder-LLM-Key"); byok != "" {
        r.Header.Del("X-Coder-LLM-Key")
        cfg.Key = byok
    }

    // pass cfg (not p.cfg) to interceptor constructors
    interceptor = messages.NewStreamingInterceptor(id, &req, payload, cfg, ...)
}
```

### Passthrough routes (`/v1/models`, etc.)

For passthrough routes, the key is injected in the reverse proxy request
callback inside `newPassthroughRouter` (`passthrough.go`) via
`InjectAuthHeader`:

```go
provider.InjectAuthHeader(&req.Header) // unconditionally overwrites X-Api-Key
```

`InjectAuthHeader` in `provider/anthropic.go` needs to check for a pre-existing
BYOK key in the headers and prefer it:

```go
func (p *Anthropic) InjectAuthHeader(headers *http.Header) {
    if byok := headers.Get("X-Coder-LLM-Key"); byok != "" {
        headers.Del("X-Coder-LLM-Key")
        headers.Set(p.AuthHeader(), byok)
        return
    }
    headers.Set(p.AuthHeader(), p.cfg.Key)
}
```

Note that `X-Coder-LLM-Key` must be stripped before the request is
forwarded upstream in both paths, so Anthropic never sees it.

## Client Configuration

Users set the following environment variables in their workspace:

```sh
export ANTHROPIC_BASE_URL="https://coder.example.com/api/v2/aibridge/anthropic"
export ANTHROPIC_API_KEY="$(coder login token)"
export ANTHROPIC_CUSTOM_HEADERS="X-Coder-LLM-Key: sk-ant-..."
```

`ANTHROPIC_CUSTOM_HEADERS` is a Claude Code feature that injects arbitrary
headers on every request. See
[Claude Code settings](https://docs.claude.com/en/docs/claude-code/settings#environment-variables).
