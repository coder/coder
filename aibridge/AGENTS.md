# AI Agent Guidelines for aibridge

> This is a package-level guide for the `aibridge/` subdirectory inside
> the coder/coder repository.
>
> Read the repo-root `AGENTS.md` and `CLAUDE.md` first. They are the
> source of truth for all shared conventions: tone, foundational rules,
> essential commands, git hooks, code style, Go patterns, testing
> patterns, LSP navigation, and PR style. This file documents only what
> is specific to the `aibridge/` package; it never relaxes a root rule.
>
> For local overrides, create `AGENTS.local.md` (gitignored).

## Architecture Overview

AI Bridge is a smart gateway that sits between AI clients (Claude Code,
Cursor, etc.) and upstream providers (Anthropic, OpenAI). It intercepts
all AI traffic to provide centralized authn/z, auditing, token
attribution, and MCP tool administration. It runs as part of `coderd`
(the Coder control plane). Users authenticate with their Coder session
tokens.

```text
┌─────────────┐     ┌──────────────────────────────────────────┐
│  AI Client   │     │               aibridge                   │
│ (Claude Code,│────▶│  RequestBridge (http.Handler)             │
│  Cursor)     │     │    ├── Provider (Anthropic/OpenAI)        │
└─────────────┘     │    ├── Interceptor (streaming/blocking)   │
                    │    ├── Recorder (tokens, prompts, tools)  │
                    │    └── MCP Proxy (tool injection)         │
                    └──────────────┬───────────────────────────┘
                                   │
                                   ▼
                           ┌──────────────┐
                           │ Upstream API  │
                           │ (Anthropic,   │
                           │  OpenAI)      │
                           └──────────────┘
```

The wire-up between aibridge and coderd lives in
`enterprise/aibridged/`. That package is outside the scope of this
guide.

Key packages within `aibridge/`:

- `intercept/`: request/response interception, per-provider subdirs
  (`messages/`, `responses/`, `chatcompletions/`)
- `provider/`: upstream provider definitions (Anthropic, OpenAI,
  Copilot)
- `mcp/`: MCP protocol integration
- `circuitbreaker/`: circuit breaker for upstream calls
- `context/`: request-scoped context helpers
- `internal/integrationtest/`: integration tests with mock upstreams

## Commands

Use the repo-root commands documented in the root `AGENTS.md`. The
notes below are aibridge-specific:

- Run only aibridge tests with `go test ./aibridge/...`. The root
  `make test` runs the full coder/coder suite.
- Regenerate the MCP mock with `go generate ./aibridge/mcpmock/` after
  changing `aibridge/mcp/api.go`. The repo-root `make gen` does not
  include this target.

## Streaming Code

This package heavily uses SSE streaming. When modifying interceptors:

- Always handle both blocking and streaming paths.
- Test with `*_test.go` files in the same package. They cover edge
  cases for chunked responses.
- Be careful with goroutine lifecycle. Ensure proper cleanup on context
  cancellation.

## Commit and PR Scope

Follow the commit and PR style in the root `AGENTS.md` and
`.claude/docs/PR_STYLE_GUIDE.md`. Format: `type(scope): message`. The
scope must be a real filesystem path containing every changed file.

For changes inside `aibridge/`, the scope is the path from the repo
root, for example:

- `feat(aibridge/intercept/messages): add cache token tracking`
- `fix(aibridge/provider): handle nil response body`
- `refactor(aibridge/mcp): extract tool filtering`

Use a broader scope, or omit the scope, when changes span beyond
`aibridge/`.

## Common Pitfalls

| Problem                 | Fix                                                                         |
|-------------------------|-----------------------------------------------------------------------------|
| Race in streaming tests | Use `t.Cleanup()` and proper synchronization, never `time.Sleep`.           |
| `mcpmock` out of date   | Run `go generate ./aibridge/mcpmock/` after changing `aibridge/mcp/api.go`. |
| Formatting failures     | Run `make fmt` from the repo root before committing.                        |
