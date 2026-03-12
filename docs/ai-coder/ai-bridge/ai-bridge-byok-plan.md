# BYOK Implementation Plan

## Overview

Changes span two repos: **coder/coder** (control plane) and **aibridge**
(bridge library). The core idea is simple: the presence of
`X-Coder-AI-Governance-BYOK-Token` header signals BYOK mode. When present,
extract the Coder session token from it and forward user-supplied LLM
credentials unchanged. When absent, it's centralized mode (current behavior).

## Phase 1: coder/coder — Header constants and token extraction

### Step 1: Update `coderd/aibridge/aibridge.go`

Add the new BYOK header constant and update `ExtractAuthToken` to check it
first:

```go
const HeaderCoderBYOKToken = "X-Coder-AI-Governance-BYOK-Token"
```

Update `ExtractAuthToken` to check `HeaderCoderBYOKToken` before
`HeaderCoderAuth` and `Authorization`:

- If `X-Coder-AI-Governance-BYOK-Token` is present → return its value (the
  Coder session token).
- Otherwise → fall through to current logic (`X-Coder-Token` →
  `Authorization: Bearer` → `X-Api-Key`).

Add a helper `IsBYOK(header http.Header) bool` that checks for the presence
of the BYOK header.

### Step 2: Update `enterprise/aibridged/http.go` — ServeHTTP

This is the critical entrypoint. Currently it:

1. Extracts Coder token via `ExtractAuthToken`.
2. Strips `X-Coder-Token`.
3. Validates token and creates actor context.

Add BYOK-aware logic after token extraction:

```go
isBYOK := agplaibridge.IsBYOK(r.Header)

if isBYOK {
    // TODO: check --allow-byok flag, reject if disabled.
    // Strip the BYOK header (contains Coder session token).
    r.Header.Del(agplaibridge.HeaderCoderBYOKToken)
    // Leave Authorization and X-Api-Key intact — they carry user's LLM
    // credentials.
} else {
    // Centralized: strip the header that carried the Coder token.
    r.Header.Del(agplaibridge.HeaderCoderAuth)
    // Also strip Authorization if that's where the Coder token was
    // (direct centralized mode sends Coder token as Authorization: Bearer).
    r.Header.Del("Authorization")
    r.Header.Del("X-Api-Key")
}
```

Pass `isBYOK` into the context (via a new context key or by extending the
actor) so downstream code in the aibridge library can read it.

### Step 3: Add `--allow-byok` deployment flag

Add the boolean flag to the aibridged server configuration. Default: `false`.
When `false` and a request has `X-Coder-AI-Governance-BYOK-Token`, reject
with 403. Location: wherever other aibridged flags are defined (likely the
enterprise server setup or serpent config).

### Step 4: DB migration — add `key_type` column

Create migration files:

```sql
-- up
CREATE TYPE aibridge_key_type AS ENUM ('centralized', 'byok');
ALTER TABLE aibridge_interceptions
    ADD COLUMN key_type aibridge_key_type NOT NULL DEFAULT 'centralized';

-- down
ALTER TABLE aibridge_interceptions DROP COLUMN key_type;
DROP TYPE aibridge_key_type;
```

### Step 5: Proto — add `key_type` field

In `enterprise/aibridged/proto/aibridged.proto`, add to
`RecordInterceptionRequest`:

```protobuf
string key_type = 12; // "centralized" or "byok"
```

Run `make gen` to regenerate Go code.

### Step 6: Update DRPC server to persist `key_type`

The DRPC server that handles `RecordInterception` RPCs needs to pass
`key_type` through to the database INSERT query. Update the relevant SQL
query in `coderd/database/queries/` and the Go handler.

## Phase 2: aibridge library — BYOK-aware credential handling

### Step 7: Add `KeyType` to context and `InterceptionRecord`

In `context/context.go`, add a `keyType` context key and helpers:

```go
func WithKeyType(ctx context.Context, keyType string) context.Context
func KeyTypeFromContext(ctx context.Context) string // "centralized" or "byok"
```

In `recorder/types.go`, add to `InterceptionRecord`:

```go
KeyType string
```

### Step 8: Update `config.Anthropic` and `config.OpenAI`

Add fields to support the OAuth/subscription case where the user's credential
is `Authorization: Bearer <anthropic-oauth-token>` rather than `X-Api-Key`:

```go
type Anthropic struct {
    // ... existing fields ...
    BYOKBearerToken string // Set when BYOK uses Authorization: Bearer (Claude Max/Pro).
    BYOK            bool   // True when in BYOK mode.
}
```

Same for `config.OpenAI`.

### Step 9: Update providers to be BYOK-aware

**`provider/anthropic.go`:**

- `CreateInterceptor`: Detect BYOK from request headers. If BYOK, extract
  the user's `Authorization: Bearer` or `X-Api-Key` from the request and set
  them on the config copy:

  ```go
  cfg := p.cfg
  if isBYOK(r) {
      if bearer := r.Header.Get("Authorization"); bearer != "" {
          cfg.BYOKBearerToken = strings.TrimPrefix(bearer, "Bearer ")
          cfg.Key = ""
      } else if apiKey := r.Header.Get("X-Api-Key"); apiKey != "" {
          cfg.Key = apiKey
      }
      cfg.BYOK = true
  }
  ```

- `InjectAuthHeader` (passthrough): Skip injection if BYOK — user's headers
  are already in the request:

  ```go
  func (p *Anthropic) InjectAuthHeader(headers *http.Header) {
      if headers.Get("X-Api-Key") != "" || headers.Get("Authorization") != "" {
          return
      }
      headers.Set(p.AuthHeader(), p.cfg.Key)
  }
  ```

**`provider/openai.go`:** Same pattern — check for existing `Authorization`
header in `InjectAuthHeader`, pass BYOK key/bearer through config.

### Step 10: Update interceptor SDK calls

**`intercept/messages/base.go` — `newMessagesService`:**

```go
if i.cfg.BYOKBearerToken != "" {
    opts = append(opts, option.WithAuthToken(i.cfg.BYOKBearerToken))
} else {
    opts = append(opts, option.WithAPIKey(i.cfg.Key))
}
```

The Anthropic SDK's `option.WithAuthToken()` sets `Authorization: Bearer`,
while `option.WithAPIKey()` sets `X-Api-Key`. This handles both BYOK
sub-modes.

**`intercept/chatcompletions/base.go` — `newCompletionsService`:**

For OpenAI, the SDK always uses `Authorization: Bearer <key>`, so the BYOK
personal key case works the same way. The subscription case (ChatGPT
Plus/Pro) would also use Bearer — needs validation as the RFC notes.

**`intercept/responses/base.go`:** Same as chatcompletions.

### Step 11: Update `bridge.go` — `newInterceptionProcessor`

Read key type from context and pass to `InterceptionRecord`:

```go
keyType := aibcontext.KeyTypeFromContext(ctx)
rec.RecordInterception(ctx, &recorder.InterceptionRecord{
    // ... existing fields ...
    KeyType: keyType,
})
```

Also add structured log field:

```go
log := logger.With(
    // ... existing fields ...
    slog.F("key_type", keyType),
)
```

## Phase 3: Wire it all together

### Step 12: `enterprise/aibridged/http.go` — set key type in context

After BYOK detection (Step 2), set key type in the context that gets passed
to the aibridge `RequestBridge`:

```go
keyType := "centralized"
if isBYOK {
    keyType = "byok"
}
r = r.WithContext(aibridge.WithKeyType(r.Context(), keyType))
```

This must happen before `handler.ServeHTTP(rw, r)` so the entire downstream
chain has access.

### Step 13: DRPC recorder bridge

The DRPC recorder implementation (which translates `InterceptionRecord` →
`RecordInterceptionRequest` proto) needs to pass through `KeyType`. Find the
recorder adapter that converts between `recorder.InterceptionRecord` and the
proto message, and add the `key_type` field mapping.

## Phase 4: Testing

- **Unit tests for `ExtractAuthToken`**: Add cases for BYOK header
  present/absent.
- **Unit tests for `IsBYOK`**: Trivial but important.
- **Integration test in `http_test.go`**: BYOK request with
  `X-Coder-AI-Governance-BYOK-Token` + `Authorization: Bearer <oauth-token>`
  — verify Coder token is extracted, BYOK header is stripped, Authorization
  is forwarded.
- **Integration test**: BYOK request with
  `X-Coder-AI-Governance-BYOK-Token` + `X-Api-Key: <api-key>` — same
  verification.
- **Integration test**: Centralized request (no BYOK header) — verify
  existing behavior unchanged.
- **Integration test**: BYOK request when `--allow-byok=false` — verify 403.
- **Test `InjectAuthHeader` skip** for passthrough routes in BYOK mode.
- **Test `key_type` in DB records** for both modes.

## Execution order

1. Steps 1–3 (coder/coder: header constant, http.go, flag) — can be done
   together.
2. Steps 4–6 (coder/coder: DB migration, proto, DRPC persistence) — can be
   done together.
3. Steps 7–11 (aibridge: context, config, providers, interceptors, bridge) —
   can be done together.
4. Steps 12–13 (wire together) — depends on 1–3 and 7–11.
5. Phase 4 (tests) — last.

Steps 1–3 and 7–11 are independent and can be worked on in parallel across
the two repos.
