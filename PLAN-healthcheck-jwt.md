# Implementation Plan: Healthcheck JWT Auth

## Problem

The background healthcheck runner (Prometheus metrics) calls `/api/v2/debug/ws` which sits behind `apiKeyMiddleware` + an RBAC check (`debug_info:read`). The current PR solves this with a `--health-check-api-key` flag, requiring operators to manually provision and configure an API key. This is awkward UX.

## Solution

Mint a JWT on startup using an in-memory signing key (following the `jwtutils` + tailnet resume token pattern). Add alternate middleware on `/debug/ws` that validates this JWT, bypassing the normal session auth + RBAC chain for internal callers.

## Architecture Context

The tailnet resume token system provides the pattern:

- `tailnet.GenerateResumeTokenSigningKey()` generates a 64-byte random key
- `jwtutils.StaticKey{ID, Key}` wraps it as a `SigningKeyManager`
- `jwtutils.Sign(ctx, key, claims)` mints JWTs with HS512
- `jwtutils.Verify(ctx, key, token, &claims)` validates them
- Claims use `jwtutils.RegisteredClaims` (wrapper around `go-jose/jwt.Claims`)

The `/debug` route group currently applies `apiKeyMiddleware` + owner-only RBAC at the group level, with `/debug/ws` nested inside.

## Steps

### 1. Remove `--health-check-api-key` config option

**Files:** `codersdk/deployment.go`, `codersdk/deployment_test.go`, `site/src/api/typesGenerated.ts`

- Remove `APIKey` field from `HealthcheckConfig` struct.
- Remove the `--health-check-api-key` / `CODER_HEALTH_CHECK_API_KEY` option from the deployment options slice.
- Remove the test entry in `TestDeploymentValues_HighlyConfigurable`.
- Run `make gen` to regenerate golden files, swagger docs, and TypeScript types.

### 2. Generate in-memory signing key + mint JWT at startup

**Files:** `coderd/coderd.go`

- Add two fields to `API` (not `Options`; these are derived state):

  ```go
  healthcheckKey   jwtutils.SigningKeyManager
  healthcheckToken string
  ```

- In `New()`, after options are set up:
  1. Generate a 64-byte random key via `crypto/rand`.
  2. Wrap as `jwtutils.StaticKey{ID: uuid.New().String(), Key: keyBytes}`.
  3. Sign a JWT using `jwtutils.Sign(ctx, key, claims)` with:
     - `Subject`: `"healthcheck"`
     - `Expiry`: 30 days (the token is only used internally; the process will restart before it expires).
  4. Store both on `api`.

### 3. Create `HealthcheckAuth` middleware

**Files:** `coderd/httpmw/healthcheckauth.go`, `coderd/httpmw/healthcheckauth_test.go`

New middleware function:

```go
func HealthcheckOrSessionAuth(
    verifyKey jwtutils.VerifyKeyProvider,
    sessionAuth func(http.Handler) http.Handler,
    rbacCheck func(http.Handler) http.Handler,
) func(http.Handler) http.Handler
```

Logic:

1. Extract token from request via `APITokenFromRequest(r)`.
2. Try `jwtutils.Verify(ctx, verifyKey, token, &claims)` with expected subject `"healthcheck"`.
3. If valid JWT: call `next.ServeHTTP(rw, r)` directly (skip session auth and RBAC; the echo endpoint has no sensitive data).
4. If not a valid JWT: delegate to `sessionAuth` then `rbacCheck` (the normal `/debug` middleware chain).

### 4. Restructure `/debug/ws` route

**Files:** `coderd/coderd.go`

Extract `/debug/ws` from the auth-protected debug group into its own sub-group with the new middleware:

```go
r.Route("/debug", func(r chi.Router) {
    // /debug/ws uses healthcheck JWT OR session auth.
    r.Group(func(r chi.Router) {
        r.Use(httpmw.HealthcheckOrSessionAuth(
            api.healthcheckKey,
            apiKeyMiddleware,
            debugRBACMiddleware,
        ))
        r.Get("/ws", (&healthcheck.WebsocketEchoServer{}).ServeHTTP)
    })

    // All other debug routes: session auth + RBAC.
    r.Group(func(r chi.Router) {
        r.Use(apiKeyMiddleware, debugRBACMiddleware)
        r.Get("/coordinator", api.debugCoordinator)
        // ... rest unchanged
    })
})
```

Factor the existing inline RBAC middleware into a named `debugRBACMiddleware` variable to avoid duplication.

### 5. Wire JWT to healthcheck runner

**Files:** `coderd/coderd.go`, `coderd/debug.go`, `coderd/prometheusmetrics/prometheusmetrics.go`, `cli/server.go`

- Change `HealthcheckFunc` signature: rename `apiKey` to `token` for clarity, but keep the `string` type. The websocket check already sends it via `Coder-Session-Token` header, which works for both session tokens and JWTs.
- Add `HealthcheckToken() string` accessor on `*API` (similar to `HealthCheckCache()`).
- In `cli/server.go`, pass `coderAPI.HealthcheckToken()` to `prometheusmetrics.Healthcheck()` instead of `vals.Healthcheck.APIKey.String()`.
- In `debug.go` (`debugDeploymentHealth`), continue passing `httpmw.APITokenFromRequest(r)` for user-initiated healthchecks (their session token still works via normal auth).
- Revert the `*http.Client` parameter addition to `HealthcheckFunc` (keep the dedicated transport in `prometheusmetrics.Healthcheck`, pass client internally).

### 6. Clean up `HealthcheckFunc` signature

**Files:** `coderd/coderd.go`, `coderd/coderdtest/coderdtest.go`, `coderd/debug.go`, `coderd/debug_test.go`, `cli/support_test.go`, `coderd/prometheusmetrics/prometheusmetrics.go`, `coderd/prometheusmetrics/prometheusmetrics_test.go`

Revert the `*http.Client` parameter addition from the `HealthcheckFunc` signature. Instead:

- Keep `func(ctx, token string, progress) *HealthcheckReport` (original arity, just with the `token` name).
- In `prometheusmetrics.Healthcheck()`, construct the `*http.Client` locally and pass it into `healthcheck.Run()` via the options (the PR already does this; just decouple it from the `HealthcheckFunc` interface).

### 7. Tests

**Files:** `coderd/httpmw/healthcheckauth_test.go`, `coderd/prometheusmetrics/prometheusmetrics_test.go`

1. **Middleware unit tests** (`healthcheckauth_test.go`):
   - Valid healthcheck JWT grants access.
   - Expired healthcheck JWT falls through to session auth.
   - Invalid/garbage token falls through to session auth.
   - Valid session token + RBAC still works (existing behavior preserved).

2. **Existing tests**: Update `prometheusmetrics_test.go` and `debug_test.go` to remove `*http.Client` param from mock `HealthcheckFunc`.

## Cross-Replica Consideration

The in-memory key means the JWT is only verifiable by the replica that minted it. If the access URL load-balances to a different replica, the websocket check gets a 401 and reports an error. This is acceptable:

- Single-replica (most common) works perfectly.
- Multi-replica: the websocket check may degrade, which accurately reflects that the internal healthcheck token cannot cross replica boundaries.
- A future upgrade path exists: replace `jwtutils.StaticKey` with a DB-backed `cryptokeys.NewSigningCache` using a new `CryptoKeyFeature`, requiring a migration. This is unnecessary complexity for now.

## Files Changed Summary

| File                                                 | Change                                              |
|------------------------------------------------------|-----------------------------------------------------|
| `codersdk/deployment.go`                             | Remove `APIKey` from `HealthcheckConfig` + option   |
| `codersdk/deployment_test.go`                        | Remove test entry                                   |
| `coderd/coderd.go`                                   | Add key/token fields, restructure `/debug/ws` route |
| `coderd/httpmw/healthcheckauth.go`                   | New middleware                                      |
| `coderd/httpmw/healthcheckauth_test.go`              | Middleware tests                                    |
| `coderd/debug.go`                                    | Revert `*http.Client` param                         |
| `coderd/debug_test.go`                               | Update mock signatures                              |
| `coderd/prometheusmetrics/prometheusmetrics.go`      | Use JWT token, internalize client                   |
| `coderd/prometheusmetrics/prometheusmetrics_test.go` | Update mock signatures                              |
| `cli/server.go`                                      | Pass `coderAPI.healthcheckToken`                    |
| `cli/support_test.go`                                | Update mock signature                               |
| `coderd/coderdtest/coderdtest.go`                    | Revert `*http.Client` param                         |
| Golden files, swagger, docs                          | Regenerated via `make gen`                          |
