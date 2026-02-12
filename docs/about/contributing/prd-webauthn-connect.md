# PRD: Server-Side WebAuthn for Workspace Connections

## Problem

Coder workspace connections (SSH, port forwarding) execute code on behalf of
the user. A stolen session token grants full access to all workspaces. There is
no mechanism to require physical presence (hardware security key) for sensitive
operations while allowing non-sensitive operations (listing, starting
workspaces) with a standard token.

The existing client-side FIDO2 prototype (branch `feat/fido2-hardware-key-auth`)
encrypts a connect token on disk with a FIDO2-derived key. This protects
against offline token theft but has fundamental limitations:

- The server has no awareness of the security key. The security boundary is the
  client, not the server.
- Token expiry requires re-login rather than transparent refresh.
- The connect token is visible in memory during creation.

## Solution

Server-side WebAuthn challenge-response for sensitive workspace operations.
Users register a FIDO2 security key with the Coder server. When performing
a sensitive operation (SSH, port forwarding), the server issues a WebAuthn
challenge. The client signs it with the YubiKey (touch) and the server
verifies the assertion before issuing a short-lived JWT scoped to the
requested operation.

### Two-token model

| Token | Purpose | Storage | Duration | FIDO2 required |
|-------|---------|---------|----------|----------------|
| Session token | Non-sensitive API calls (list, create, start, stop workspaces) | Keychain / session file | Configurable (existing `--session-duration`) | No |
| Connection JWT | Sensitive operations (SSH, port forward, app connect) | In-memory only, never written to disk | Configurable server-side (default: 5 min, 0 = single use) | Yes |

### Connection JWT duration

A new server-side flag `--fido2-token-duration` (env `CODER_FIDO2_TOKEN_DURATION`)
controls how long the connection JWT is valid after WebAuthn verification:

- **Default (5m)**: One touch grants SSH access for 5 minutes. Multiple
  connections within that window reuse the JWT without additional touches.
- **0**: Single-use. Every SSH connection requires a new WebAuthn challenge
  and touch. Most secure.
- **Longer durations (1h, 8h)**: Fewer touches during a work session. Less
  secure against session hijacking within the window.

### Authentication flow

```
1. coder ssh my-workspace
2. Client → Server:  POST /api/v2/users/me/webauthn/challenge
                     (with session token)
3. Server → Client:  { challenge, allowCredentials, rpId, timeout }
4. Client → YubiKey: CTAP2 GetAssertion (user touches key)
5. YubiKey → Client: signed assertion
6. Client → Server:  POST /api/v2/users/me/webauthn/verify
                     { assertion, clientDataJSON, authenticatorData, signature }
7. Server verifies assertion using stored public key
8. Server → Client:  { jwt: "<short-lived-token>" }
9. Client → Server:  WebSocket /api/v2/workspaceagents/{id}/coordinate
                     (Authorization: Bearer <jwt>)
```

### WebAuthn credential lifecycle

- **Register**: `POST /api/v2/users/me/webauthn/register/begin` +
  `POST /api/v2/users/me/webauthn/register/finish`
  Stores the credential public key in the database. Requires existing
  authenticated session.
- **List**: `GET /api/v2/users/me/webauthn/credentials`
- **Delete**: `DELETE /api/v2/users/me/webauthn/credentials/{id}`

### Require FIDO2 for all users (admin setting)

A server-side flag `--require-fido2-connect` (env `CODER_REQUIRE_FIDO2_CONNECT`)
enforces that all workspace connections must present a valid connection JWT.
When enabled:

- Users without a registered WebAuthn credential cannot SSH or port-forward.
- The coordination endpoint (`/api/v2/workspaceagents/{id}/coordinate`)
  rejects requests without a valid connection JWT.
- Non-sensitive operations (list, start, stop) continue to work with the
  regular session token.

## Architecture

### Server changes

**New database tables:**

```
webauthn_credentials:
  id              UUID PRIMARY KEY
  user_id         UUID REFERENCES users(id)
  credential_id   BYTEA        -- WebAuthn credential ID
  public_key      BYTEA        -- COSE public key
  attestation_type TEXT
  aaguid          BYTEA        -- authenticator AAGUID
  sign_count      BIGINT       -- replay protection
  name            TEXT         -- user-given name ("my yubikey")
  created_at      TIMESTAMPTZ
  last_used_at    TIMESTAMPTZ
```

**New API endpoints (6):**

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/v2/users/me/webauthn/register/begin` | Start WebAuthn registration |
| POST | `/api/v2/users/me/webauthn/register/finish` | Complete registration, store credential |
| GET | `/api/v2/users/me/webauthn/credentials` | List registered credentials |
| DELETE | `/api/v2/users/me/webauthn/credentials/{id}` | Delete a credential |
| POST | `/api/v2/users/me/webauthn/challenge` | Generate assertion challenge |
| POST | `/api/v2/users/me/webauthn/verify` | Verify assertion, return JWT |

**JWT issuance:**

The connection JWT is a standard RS256/ES256 JWT containing:

```json
{
  "sub": "<user_id>",
  "aud": "coder-connect",
  "exp": <now + fido2_token_duration>,
  "iat": <now>,
  "jti": "<unique_id>",
  "scope": "ssh,port-forward,app-connect"
}
```

Signed with a server-side key (can reuse Coder's existing signing key or a
dedicated FIDO2 signing key). Verified statelessly at the coordination
endpoint.

**Coordination endpoint changes:**

The `/api/v2/workspaceagents/{id}/coordinate` endpoint gains an additional
auth path:

1. Check for `Authorization: Bearer <jwt>` header
2. If present, verify JWT signature, expiry, audience
3. If valid, allow the connection (bypass scope check for ActionSSH)
4. If absent or invalid, fall back to existing session token auth
5. If `--require-fido2-connect` is enabled and no valid JWT, reject with 401

### Client changes

**Helper binary (`coder-fido2`):**

Separate Go module (`cmd/coder-fido2/`), built with CGo + `libfido2`.
Does not affect the main coder module's `go.mod`. Two subcommands:

- `coder-fido2 register --config-dir <dir>`
  Called during WebAuthn registration. Server sends creation options,
  client passes them to the helper which calls CTAP2 MakeCredential
  (user touches key). Prints the attestation response as JSON to stdout.
  Client sends it back to the server's finish endpoint.
- `coder-fido2 assert --config-dir <dir>`
  Called during WebAuthn authentication. Server sends assertion options
  (challenge + allowCredentials), client passes them to the helper which
  calls CTAP2 GetAssertion (user touches key). Prints the assertion
  response (authenticatorData, signature, clientDataJSON) as JSON to
  stdout. Client sends it back to the server's verify endpoint.

Both subcommands:
- Read options from stdin as JSON (challenge, rpId, credential IDs, etc.)
- Read optional PIN from a second line on stdin
- Exit code 2 for touch timeout (retryable)
- Exit code 3 for PIN required (prompt and retry)
- Device discovery reused from the client-side branch

**CLI changes:**

- `coder login` stays the same (session token for non-sensitive ops).
- `coder webauthn register` — registers a security key with the server.
  Calls begin/finish endpoints, shells out to helper for MakeCredential.
- SSH/port-forward path in `InitClient`: before opening the WebSocket,
  checks if the server requires FIDO2 (or if the user has a registered key),
  runs the challenge-verify flow, caches the JWT in memory for its duration.

**Reuse from client-side branch (`feat/fido2-hardware-key-auth`):**

- `cli/fido2/helper.go` — shell-out wrapper (`runHelper`), timeout/PIN
  detection (`ErrTouchTimeout`, `ErrPinRequired`), exit code handling.
  Modified: `RunRegister` and new `RunAssert` replace `RunDeriveKey`.
- `DialFailureHandler` interface in `codersdk/credentials.go` — used to
  trigger JWT refresh on 401 during WebSocket dial.
- `cmd/coder-fido2/` — device discovery, error handling, exit codes.
  Modified: `cmdRegister` updated for WebAuthn attestation format,
  new `cmdAssert` for WebAuthn assertion signing. HMAC-secret logic
  removed.

**Drop from client-side branch:**

- `cli/fido2/store.go` — no AES encryption, no tokens on disk.
- `codersdk/credentials_fido2.go` — `FIDO2SessionTokenProvider` replaced
  by `WebAuthnSessionTokenProvider` that does challenge-verify-JWT.
- `coderd/rbac/scopes.go` change — JWT auth bypasses scopes.

### Dependencies

**Server:** `github.com/go-webauthn/webauthn` — WebAuthn Relying Party
library. Handles credential storage format, challenge generation, assertion
verification. Pure Go, no CGo.

**Client helper:** `github.com/keys-pub/go-libfido2` — CTAP2 over USB HID.
CGo + libfido2. Isolated in separate Go module (`cmd/coder-fido2/`). Does
not affect main module's `go.mod`.

## Implementation plan

### Phase 1: Server-side WebAuthn (core)

1. Add `github.com/go-webauthn/webauthn` to server dependencies
2. Database migration: `webauthn_credentials` table + queries
3. `coderd/webauthn.go`: WebAuthn registration endpoints (begin/finish)
4. `coderd/webauthn.go`: Challenge/verify endpoints + JWT issuance
5. Credential list/delete endpoints
6. Connection JWT verification at coordination endpoint
   (`coderd/workspaceagents.go`)
7. Route registration in `coderd/coderd.go`
8. `--fido2-token-duration` server flag (default 5m)

### Phase 2: Client integration

9. Update `cmd/coder-fido2/`: `register` (attestation) + `assert`
   (assertion signing) subcommands. Input via JSON on stdin, output
   JSON to stdout. Reuse device discovery, timeout/PIN error handling.
10. `cli/fido2/helper.go`: `RunRegister` + `RunAssert` wrappers
11. `coder webauthn register` CLI command (begin → helper → finish)
12. `WebAuthnSessionTokenProvider` in `codersdk/`: challenge → helper
    assert → verify → cache JWT in memory
13. Wire provider in `InitClient` when server reports WebAuthn available

### Phase 3: Enforcement and polish

14. `--require-fido2-connect` server flag
15. Audit log entries for WebAuthn registration/authentication
16. Update `enterprise/audit/table.go` for new table
17. Documentation page
18. Dashboard UI for managing WebAuthn credentials (future, separate PR)

## Security properties

| Property | Client-side (old) | Server-side (new) |
|----------|-------------------|-------------------|
| Token theft protection | Encrypted on disk | No token on disk |
| Server awareness | None | Full verification |
| Replay protection | None | Sign count + JTI |
| Single-use mode | No | Yes (duration=0) |
| Admin enforcement | No | Yes (require flag) |
| Challenge freshness | N/A | Server-generated nonce |

## Open questions

1. **Should registration require admin approval?** Or can any user self-serve
   register a key?
2. **Multiple keys per user?** The schema supports it. Useful for backup keys.
3. **Recovery flow**: If a user loses their only key and `--require-fido2-connect`
   is enabled, how do they regain access? Admin override endpoint?
4. **Browser-based WebAuthn**: Should the dashboard support WebAuthn for
   browser-based terminal sessions? The server-side infrastructure supports
   it — the browser has native WebAuthn APIs — but it's additional UI work.
