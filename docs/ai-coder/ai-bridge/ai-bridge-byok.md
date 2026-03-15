# RFC Bring your own keys (BYOK)

## Problem Statement

[Own keys can only be provided *in addition to* a Coder API key; the latter authenticates the request with AI Bridge and ties the request to a known identity.](https://www.notion.so/Own-keys-can-only-be-provided-in-addition-to-a-Coder-API-key-the-latter-authenticates-the-request-w-314d579be59280e78626e885ec1bb636?pvs=21)

AI Bridge currently supports only a centralized LLM key configured by admins
(`CODER_AIBRIDGE_ANTHROPIC_KEY`). Users with their own Anthropic credentials
(Claude Max/Pro subscription or a personal API key) have no way to use them
through AI Bridge.

## Requirements

### Requirement 1: BYOK key forwarding

- Users can optionally supply their own LLM provider API keys.
- When a user key is present, it is used instead of the centralized key.
- When no user key is present, the system falls back to the centralized key —
  existing behavior is unchanged.
- It applies to both intercepted routes (`/v1/messages`) and
  passthrough routes (`/v1/models`, etc.).
- The user-supplied `X-Coder-AI-Governance-BYOK-Token` header is stripped before forwarding and never reaches the upstream provider as a custom header.
- Deployment-level boolean flag `--allow-byok` to control whether BYOK is allowed.
	- If flag is false (default) - only centralized key is used.
	- if flag is true - users are allowed to use BYOK. If BYOK is not set - fallback to centralized key.
- Support subscription-based models (Claude Pro/Max, ChatGPT Plus/Pro)

### Requirement 2: Key-type identifier in observability data

As a platform admin, I want to see whether each AI request used the centralized
deployment key or a user-supplied BYOK key in Bridge logs, so that I can audit
key usage patterns.

- Every `aibridge_interceptions` record indicates which key type was used for
  the upstream request: `centralized` or `byok`.
- The key type should show up in all relevant contexts, including:
	- Bridge database records
	- UI
	- Structured logs

## Mode Definition and Detection

The goal is to support three modes without breaking the existing centralized
flow.

| Mode | Who sets the upstream key | How the Coder token is sent |
| --- | --- | --- |
| **1. Centralized** (current) | Admin (`CODER_AIBRIDGE_ANTHROPIC_KEY`) | `Authorization: Bearer` |
| **2. BYOK – Claude Max/Pro** | Client adds `Authorization` header | `X-Coder-AI-Governance-BYOK-Token` (via `ANTHROPIC_CUSTOM_HEADERS`) |
| **3. BYOK – Personal API key** | User's `ANTHROPIC_API_KEY`, passed as `X-Api-Key`  | `X-Coder-AI-Governance-BYOK-Token` (via `ANTHROPIC_CUSTOM_HEADERS`) |

Detection is binary, based on a single dedicated header:

- `X-Coder-AI-Governance-BYOK-Token` **absent** → **Centralized**: AI Bridge injects the
  centralized key. The Coder session token is in `Authorization: Bearer`.
- `X-Coder-AI-Governance-BYOK-Token` **present** → **BYOK**: AI Bridge forwards the
  user's LLM credentials (`Authorization: Bearer` or `X-Api-Key`) unchanged.
  The Coder session token is in `X-Coder-AI-Governance-BYOK-Token`.

`X-Coder-AI-Governance-BYOK-Token` is only ever set by clients explicitly opting into
BYOK. Neither AI Bridge nor the proxy injects this header for centralized
requests.

## Mechanism

> Examples below use Anthropic and Claude Code. The same mechanism applies to
other providers (e.g. OpenAI) in a similar way.
>

### Centralized

The user sets `ANTHROPIC_AUTH_TOKEN` to their Coder session token. Claude Code
sends it as `Authorization: Bearer`. AI Bridge extracts it, validates it,
strips it, then injects the centralized key.

```
Claude Code
  │  Authorization: Bearer <coder-session-token>   ← via ANTHROPIC_AUTH_TOKEN
  ▼
aibridged
  1. X-Coder-AI-Governance-BYOK-Token absent → centralized
  2. Extracts Coder token from Authorization: Bearer
  3. Validates token, strips Authorization header
  4. Injects X-Api-Key: <CODER_AIBRIDGE_ANTHROPIC_KEY>
  ▼
Anthropic API
  X-Api-Key: <centralized-key>
```

### BYOK – Claude Max/Pro subscription

The user runs `claude login` once. Claude Code stores the OAuth token in
`~/.claude.json` and sends it as `Authorization: Bearer`. The Coder session
token is sent separately via `X-Coder-AI-Governance-BYOK-Token`.

AI Bridge extracts the Coder token from `X-Coder-AI-Governance-BYOK-Token`, strips it,
and forwards `Authorization: Bearer` to Anthropic unchanged. No centralized
key is injected.

```
Claude Code
  │  Authorization: Bearer sk-ant-oat01-...    ← Anthropic OAuth (from ~/.claude.json)
  │  X-Coder-AI-Governance-BYOK-Token: <coder-token>. ← via ANTHROPIC_CUSTOM_HEADERS
  ▼
aibridged
  1. X-Coder-AI-Governance-BYOK-Token present → BYOK
  2. Extracts Coder token from X-Coder-AI-Governance-BYOK-Token
  3. Validates token, strips X-Coder-AI-Governance-BYOK-Token header
  4. Forwards Authorization: Bearer to Anthropic unchanged
  ▼
Anthropic API
  Authorization: Bearer sk-ant-oat01-...
```

### BYOK – Personal Anthropic API key

The user sets `ANTHROPIC_API_KEY`. Claude Code sends it as `X-Api-Key`.
The Coder session token is sent separately via `X-Coder-AI-Governance-BYOK-Token`.

AI Bridge extracts the Coder token from `X-Coder-AI-Governance-BYOK-Token`, strips it,
and forwards `X-Api-Key` to Anthropic unchanged. No centralized key is injected.

```
Claude Code
  │  X-Api-Key: sk-ant-api03-...              ← Anthropic API key
  │  X-Coder-AI-Governance-BYOK-Token: <coder-token>  ← via ANTHROPIC_CUSTOM_HEADERS
  ▼
aibridged
  1. X-Coder-AI-Governance-BYOK-Token present → BYOK
  2. Extracts Coder token from X-Coder-AI-Governance-BYOK-Token
  3. Validates token, strips X-Coder-AI-Governance-BYOK-Token header
  4. Forwards X-Api-Key to Anthropic unchanged
  ▼
Anthropic API
  X-Api-Key: sk-ant-api03-...
```

> **Note:** In the implementation, AI Bridge does not need to differentiate between
Claude Max/Pro and Personal API key modes. When `X-Coder-AI-Governance-BYOK-Token` is
present, it is BYOK — the bridge strips that header and forwards both
`Authorization` and `X-Api-Key` unchanged. Which one Anthropic actually uses
depends on what the client sent.
>

## Client Configuration

### Claude Code

### Mode 1: Centralized

```bash
export ANTHROPIC_BASE_URL="<https://coder.example.com/api/v2/aibridge/anthropic>"
export ANTHROPIC_AUTH_TOKEN="$CODER_SESSION_TOKEN"
```

### Mode 2: BYOK – Claude Max/Pro subscription

Log in to Anthropic interactively (once): `claude login`

Then inject the Coder token as `X-Coder-AI-Governance-BYOK-Token`.

```bash
export ANTHROPIC_BASE_URL="<https://coder.example.com/api/v2/aibridge/anthropic>"
export ANTHROPIC_CUSTOM_HEADERS="X-Coder-AI-Governance-BYOK-Token: $CODER_SESSION_TOKEN"
```

Claude Code reads the Anthropic OAuth token from `~/.claude.json` automatically.

### Mode 3: BYOK – Personal Anthropic API key

```bash
export ANTHROPIC_BASE_URL="<https://coder.example.com/api/v2/aibridge/anthropic>"
export ANTHROPIC_API_KEY="sk-ant-api03-..."
export ANTHROPIC_CUSTOM_HEADERS="X-Coder-AI-Governance-BYOK-Token: $CODER_SESSION_TOKEN"
```

`ANTHROPIC_CUSTOM_HEADERS` is a Claude Code feature for injecting arbitrary headers on every request. See [Claude Code settings](https://docs.claude.com/en/docs/claude-code/settings#environment-variables).

### Codex CLI

### Mode 1: Centralized

```toml
[model_providers.coder]
name     = "AI Bridge"
base_url = "<https://coder.example.com/api/v2/aibridge/openai/v1>"
env_key  = "CODER_SESSION_TOKEN"
```

### Mode 2: BYOK – ChatGPT Plus/Pro subscription

`NOTE`: Should work similarly to Claude Pro/Max subscriptions, but this needs to be validated.

Log in to OpenAI interactively (once): `codex login`

Then inject the Coder token as `X-Coder-AI-Governance-BYOK-Token`.

```toml
[model_providers.coder]
name             = "AI Bridge"
base_url         = "<https://coder.example.com/api/v2/aibridge/openai/v1>"
env_http_headers = { "X-Coder-AI-Governance-BYOK-Token" = "CODER_SESSION_TOKEN" }
```

### Mode 3: BYOK – Personal OpenAI API key

```toml
[model_providers.coder]
name             = "AI Bridge"
base_url         = "<https://coder.example.com/api/v2/aibridge/openai/v1>"
env_key          = "OPENAI_API_KEY"
env_http_headers = { "X-Coder-AI-Governance-BYOK-Token" = "CODER_SESSION_TOKEN" }
```

```bash
export OPENAI_API_KEY="sk-..."
```

## Supported Clients

`NOTE`: This table is based on a quick review of the documentation.

| Client | Supports custom headers | Config option | Docs |
| --- | --- | --- | --- |
| **Claude Code** | Yes | `ANTHROPIC_CUSTOM_HEADERS` env var | [Claude Code settings](https://docs.anthropic.com/en/docs/claude-code/settings#environment-variables) |
| **Codex CLI** | Yes | `env_http_headers` in provider config | [Codex custom providers](https://developers.openai.com/codex/config-advanced#custom-model-providers) |
| **opencode** | Yes | `options.headers` in provider config | [opencode custom headers](https://opencode.ai/docs/providers/#custom-headers) |
| **Factory** | Yes | `extraHeaders` in model config | [Factory BYOK](https://docs.factory.ai/cli/byok/overview#supported-fields) |
| **Cline** | Not clear, needs testing | supported only in VSCode Extension? | [issue #8724](https://github.com/cline/cline/issues/8724), [discussion #2725](https://github.com/cline/cline/discussions/2725) |
| **Kilo Code** | Not clear, needs testing | supported only in VSCode Extension? | [issue #4864](https://github.com/Kilo-Org/kilocode/issues/4864) |
| **Roo Code** | Partial, needs testing | Custom headers field in API config | [issue #11450](https://github.com/RooCodeInc/Roo-Code/issues/11450) |
| **GitHub Copilot** | Seems no |  |  |

## Bridge Proxy Integration

aibridgeproxyd uses `X-Coder-Token` to forward the Coder session token to
aibridged. BYOK uses `X-Coder-AI-Governance-BYOK-Token`. Because these are distinct
headers, the two mechanisms do not interfere:

- Centralized via proxy: `X-Coder-Token` is set, `X-Coder-AI-Governance-BYOK-Token` is
  absent → aibridged detects centralized mode and injects the admin key.
- BYOK via proxy: client sets `X-Coder-AI-Governance-BYOK-Token` (via
  `ANTHROPIC_CUSTOM_HEADERS`), proxy forwards it unchanged alongside
  `X-Coder-Token` → aibridged detects BYOK mode and forwards the user's LLM
  credentials.

No changes to aibridgeproxyd are required for BYOK support.

**Note:** We plan to rename the `X-Coder-Token` header to `X-Coder-AI-Bridge-Proxy-Token` to better convey its intent and usage.

## Key-Type Observability: Required Changes

To satisfy Requirement 2, the key type must flow from the point of key
resolution all the way to the DB record. There are three layers to update.

### 1. DB migration

Add a `key_type` column to `aibridge_interceptions`:

```sql
CREATE TYPE aibridge_key_type AS ENUM ('centralized', 'byok');

ALTER TABLE aibridge_interceptions
    ADD COLUMN key_type aibridge_key_type NOT NULL DEFAULT 'centralized';
```

### 2. Proto

Add `key_type` to `RecordInterceptionRequest` in
`enterprise/aibridged/proto/aibridged.proto`:

```protobuf
message RecordInterceptionRequest {
  string key_type; // "centralized" or "byok"
}
```

### 3. aibridge library

The key type can be resolved in `newInterceptionProcessor` in `bridge.go` by
peeking at the `X-Coder-AI-Governance-BYOK-Token` header.

```go
keyType := "centralized"
if r.Header.Get("X-Coder-AI-Governance-BYOK-Token") != "" {
    keyType = "byok"
}
```

Add `KeyType` to `InterceptionRecord` in `recorder/types.go`:

```go
rec.RecordInterception(ctx, &recorder.InterceptionRecord{
    KeyType: keyType,
})
```
