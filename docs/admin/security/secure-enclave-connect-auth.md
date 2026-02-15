# Secure Enclave Connect Auth

## Objective

This feature adds a second layer of authentication for sensitive workspace
operations (SSH, port-forwarding) using the macOS Secure Enclave. When
enabled, users must pass a Touch ID biometric check before connecting to
workspaces, even if their session token has been compromised.

The primary threat this addresses is **stolen session tokens used from a
different machine**. A session token file (`~/.config/coderv2/session`)
stolen via phishing, malware, or backup exposure becomes useless for SSH
because the attacker does not have access to the victim's Secure Enclave
hardware.

## How It Works

### Key Generation (during `coder login`)

```
┌─────────────┐                    ┌──────────────────┐
│  coder CLI   │                    │  Coder Server     │
│  (macOS)     │                    │                    │
├─────────────┤                    ├──────────────────┤
│ 1. Generate ECDSA P-256 keypair  │                    │
│    in Secure Enclave             │                    │
│                                   │                    │
│ 2. Private key stays in hardware  │                    │
│    (never exportable)            │                    │
│                                   │                    │
│ 3. Save encrypted key reference   │                    │
│    to ~/.config/coderv2/          │                    │
│    connect-key (380 bytes)       │                    │
│                                   │                    │
│ 4. Upload 65-byte public key ────────────────────────►│
│    (0x04 || X || Y)              │  Stored on the     │
│                                   │  api_keys row      │
└─────────────┘                    └──────────────────┘
```

The `connect-key` file is an opaque encrypted blob (`dataRepresentation`)
that only the Secure Enclave on **this specific device** can use. It is
not the private key — it is a reference that the Secure Enclave decrypts
internally when asked to sign.

### Sensitive Action (during `coder ssh`)

```
┌─────────────┐                    ┌──────────────────┐
│  coder CLI   │                    │  Coder Server     │
├─────────────┤                    ├──────────────────┤
│ 1. Dial coordination WebSocket ──────────────────────►│
│                                   │ 2. Check: does    │
│                                   │    this API key   │
│                                   │    have a connect  │
│                                   │    public key?     │
│                                   │                    │
│                                   │ 3. Yes → return    │
│◄───────────────────────── 403 Forbidden ──────────────│
│    "connect-auth proof required"  │                    │
│                                   │                    │
│ 4. Touch ID prompt appears        │                    │
│    ┌─────────────────────┐        │                    │
│    │  🔒 Touch ID        │        │                    │
│    │                      │        │                    │
│    │  Coder needs your    │        │                    │
│    │  fingerprint for a   │        │                    │
│    │  secure workspace    │        │                    │
│    │  connection          │        │                    │
│    │                      │        │                    │
│    │  [Place finger]      │        │                    │
│    └─────────────────────┘        │                    │
│                                   │                    │
│ 5. User places finger             │                    │
│                                   │                    │
│ 6. Secure Enclave signs           │                    │
│    SHA-256(timestamp) with        │                    │
│    the private key                │                    │
│                                   │                    │
│ 7. Retry dial with header: ──────────────────────────►│
│    Coder-Connect-Proof:           │                    │
│    {timestamp, signature}         │ 8. Verify ECDSA   │
│                                   │    signature with  │
│                                   │    stored public   │
│                                   │    key             │
│                                   │                    │
│                                   │ 9. Check timestamp │
│                                   │    within ±30s     │
│                                   │                    │
│◄──────────────────── 101 Switching Protocols ─────────│
│    SSH connection established     │                    │
└─────────────┘                    └──────────────────┘
```

### Non-Sensitive Actions

Normal API calls (`coder list`, `coder users list`, `coder templates`,
etc.) are not affected. They use the session token as usual with no
Touch ID prompt.

## Server Configuration

### Deployment Flag

```yaml
# coder server config
connectAuthEndpoints:
  - ssh
```

Or via CLI:

```bash
coder server --connect-auth-endpoints ssh
```

Or via environment variable:

```bash
export CODER_CONNECT_AUTH_ENDPOINTS=ssh
```

### Supported Endpoint Categories

| Category | What it protects |
|---|---|
| `ssh` | Workspace SSH connections (`coder ssh`) |
| `port-forward` | Port forwarding (`coder port-forward`) |

Both SSH and port-forward share the same coordination endpoint, so
enabling either category protects both connection types.

An empty list (the default) disables connect-auth enforcement entirely.

### Behavior When Enabled

- Users who have logged in from a macOS device with Secure Enclave will
  have a connect key automatically registered. SSH works with Touch ID.
- Users who logged in from a non-macOS device (Linux, Windows) or a Mac
  without Secure Enclave will **not** have a connect key. They will be
  **blocked** from SSH with a clear error message telling them to log in
  from a macOS device with Touch ID.
- Non-sensitive operations (API calls, template management, workspace
  creation) are never affected regardless of configuration.

## Building with Secure Enclave Support

The Secure Enclave integration uses Apple's CryptoKit framework via a
Swift-to-C-to-Go bridge. It requires:

- macOS with Xcode (for `swiftc`)
- CGO enabled
- Apple Silicon or Intel Mac with T2 chip (for Secure Enclave hardware)

### Development Build

```bash
make build-touchid
```

This compiles the Swift code into a static library, then builds the Go
binary with CGO linking against it. **No code signing is required** —
CryptoKit's Secure Enclave APIs work from unsigned binaries.

### Build Without Secure Enclave (default)

```bash
make build
# or
CGO_ENABLED=0 go build ./cmd/coder/
```

Builds with stub implementations. `touchid.IsAvailable()` returns false.
The binary works normally; connect-auth is simply not available.

### Build Tags

| Build | Secure Enclave | Touch ID |
|---|---|---|
| `CGO_ENABLED=1` on macOS with `libenclave.a` | Active | Works |
| `CGO_ENABLED=0` on macOS | Stub | Not available |
| Any build on Linux/Windows | Stub | Not available |

## External Dependencies

**No new Go module dependencies.** The implementation uses:

- `crypto/ecdsa`, `crypto/elliptic`, `crypto/sha256` (Go stdlib) — for
  server-side signature verification
- Apple CryptoKit (via Swift) — for Secure Enclave key operations
- Apple LocalAuthentication (via Swift) — for Touch ID biometric prompts
- Apple Security framework (via Swift) — for access control flags

The Swift code is compiled into a static library (`libenclave.a`) that is
linked into the Go binary via CGO.

## File Layout

```
cli/touchid/
  touchid.go             # Error types (ErrNotAvailable, ErrUserCancelled)
  enclave_darwin.go      # CGO bridge: Go → C → Swift (//go:build darwin && cgo)
  enclave_other.go       # Stub for non-darwin (//go:build !darwin || !cgo)
  enclave.h              # C function declarations
  enclave.swift          # CryptoKit Secure Enclave operations
  build_swift.sh         # Compiles enclave.swift → libenclave.a
  entitlements.plist     # Reference for production code signing

cli/connectauth.go       # SetupConnectAuth, ObtainConnectProof, TeardownConnectAuth
cli/config/file.go       # ConnectKey() config path

coderd/connectauth.go    # Server: PUT/DELETE endpoints, verifyConnectProof()
codersdk/connectauth.go  # SDK: types, client methods, constants

coderd/workspaceagents.go  # Enforcement in workspaceAgentClientCoordinate
codersdk/deployment.go     # --connect-auth-endpoints flag
codersdk/workspacesdk/
  dialer.go              # Retry logic with Coder-Connect-Proof header
  workspacesdk.go        # OnConnectAuthRequired callback
```

## Security Analysis

### What This Protects Against

| Threat | Protected | Mechanism |
|---|---|---|
| Session token stolen, used from a different machine | **Yes** | Secure Enclave key is device-bound. The `dataRepresentation` file is an encrypted reference that only this device's Secure Enclave can use. |
| `dataRepresentation` file stolen, used on another machine | **Yes** | Same as above — the encrypted reference is hardware-bound. |
| Attacker intercepts network traffic and replays proof | **Partially** | Proofs contain a timestamp verified within ±30 seconds. TLS is the primary defense. |
| Attacker with code execution on victim's machine | **Partially** | Touch ID prompt is required for each signing operation. The attacker would need to trick the user into touching the sensor. |

### Key Immutability

Once a connect public key is set on an API key, it **cannot be changed
or replaced** through the API. The server rejects any `PUT /connect-key`
request with HTTP 409 Conflict if a key is already enrolled.

This prevents an attacker who has stolen a session token from silently
replacing the victim's connect key with their own. Even with a valid
session token, the attacker cannot overwrite the existing key and must
instead create a new session (which requires completing the full login
flow including SSO + 2FA).

To enroll a new connect key (e.g., when changing laptops), the user
must run `coder login` which creates a **new API key** with a new ID.
The old API key and its connect key remain intact until the old session
expires or is revoked.

### What This Does NOT Protect Against

| Threat | Why not | Mitigation |
|---|---|---|
| Attacker completes a full login (SSO + 2FA) | A new login creates a new API key with a new connect key. The attacker has their own valid session. | This is by design — if someone can pass SSO + 2FA, they are considered authenticated. Detect via audit logs (multiple active sessions, unusual login locations). |
| Attacker with a stolen session token calls `PUT /connect-key` | **Blocked by key immutability.** The server rejects key updates on API keys that already have a connect key enrolled. | No mitigation needed — the server enforces this. |
| Attacker with a stolen session token calls `coder login --token` | This creates a **new** API key (different ID) and enrolls a connect key on it. The attacker can SSH with the new session. | **Require SSO + 2FA for login.** The `--token` flag creates a new session, so the full login flow (SSO + 2FA) should be the only way to obtain tokens. Disable token-based login if possible. |
| Attacker with code execution hooks the coder process | A sophisticated attacker could intercept the signing result after Touch ID approval. | Endpoint Detection and Response (EDR) software, code signing, binary integrity checks. |
| Non-macOS users | Secure Enclave is Apple-only. | Non-macOS users are blocked from sensitive endpoints when enforcement is enabled. Consider FIDO2 hardware keys as a cross-platform alternative. |

### Critical Requirement: SSO + 2FA

**Connect-auth enforcement should always be paired with SSO + 2FA for
login.** The security model assumes that `coder login` is a trusted
operation — whoever completes the login flow can enroll a Secure Enclave
key on a new API key. If login only requires a password (no 2FA), an
attacker with the password can log in, get a new session, and enroll
their own key.

With SSO + 2FA, enrollment is protected by the identity provider's
authentication strength, which typically includes:

- Something you know (password)
- Something you have (phone for push notification, TOTP, or hardware key)

This makes unauthorized key enrollment significantly harder.

Note: the key immutability check protects existing sessions — an
attacker with a stolen session token **cannot** replace the victim's
connect key. The SSO + 2FA requirement protects new sessions — an
attacker **cannot** create a new session without passing the full
authentication flow.

### Comparison with Alternatives

| Approach | Remote theft | Local attacker | Cross-platform | Requires hardware |
|---|---|---|---|---|
| **Secure Enclave (this PR)** | Protected | Touch ID required | macOS only | No (built into Mac) |
| **FIDO2 hardware key** | Protected | Physical touch required | Yes | Yes ($50+) |
| **Short-lived tokens** | Protected | Must re-auth | Yes | No |
| **No protection (status quo)** | Vulnerable | Vulnerable | N/A | N/A |

## Multi-Device Considerations

Currently, each `coder login` replaces the previous connect public key on
the API key. This means:

- Only the most recently logged-in device can SSH
- Logging in from a new laptop invalidates the old laptop's connect key
- There is no multi-device support (yet)

Supporting multiple devices would require storing multiple public keys per
user (a separate database table) rather than a single key on the API key
row. This is a potential future enhancement.
