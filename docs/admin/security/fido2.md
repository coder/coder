# FIDO2 Hardware Key Authentication

Coder supports FIDO2/WebAuthn hardware security keys (such as YubiKeys) to
protect sensitive workspace operations. When enabled, users must physically
touch their security key before establishing SSH, port-forwarding, or other
workspace connections.

This provides a physical presence guarantee that prevents stolen session tokens
from being used to access workspace contents.

## How it works

1. **Users register their security key** with `coder webauthn register`.
2. **When connecting**, the CLI automatically challenges the user to touch their
   key before the connection is established.
3. **The server verifies** the cryptographic proof and issues a short-lived
   connection JWT.
4. **The JWT is sent** with the connection request. The server rejects
   connections without a valid JWT when enforcement is enabled.

Non-connection operations (listing workspaces, creating them, managing
templates, etc.) are not affected and use the regular session token.

## Prerequisites

The `coder-fido2` helper binary must be installed on the user's machine and
available on `PATH`. This binary handles USB communication with the security
key. It is built separately because it requires CGo and the `libfido2` C
library.

### macOS

```shell
brew install libfido2
cd cmd/coder-fido2
CGO_ENABLED=1 go build -o /usr/local/bin/coder-fido2 .
```

### Linux

```shell
sudo apt-get install libfido2-dev
cd cmd/coder-fido2
CGO_ENABLED=1 go build -o /usr/local/bin/coder-fido2 .
```

## Server configuration

All FIDO2 settings are in the `FIDO2 / WebAuthn` flag group.

| Flag | Environment variable | Default | Description |
|------|---------------------|---------|-------------|
| `--require-fido2-connect` | `CODER_REQUIRE_FIDO2_CONNECT` | `false` | Require FIDO2 verification for all workspace connections. Users without a registered key are rejected with instructions to register one. |
| `--fido2-token-duration` | `CODER_FIDO2_TOKEN_DURATION` | `5m` | How long a connection token is valid after a key touch. Set to `0s` for single-use tokens (one connection per touch). |
| `--require-fido2-user-verification` | `CODER_REQUIRE_FIDO2_USER_VERIFICATION` | `false` | Require PIN or biometric in addition to physical key touch. |

### Example: enforce FIDO2 with single-use tokens

```shell
coder server \
  --require-fido2-connect \
  --fido2-token-duration 0s
```

### Example: FIDO2 with 10-minute window and PIN requirement

```shell
coder server \
  --require-fido2-connect \
  --fido2-token-duration 10m \
  --require-fido2-user-verification
```

### YAML configuration

```yaml
fido2:
  requireFido2Connect: true
  fido2TokenDuration: 5m0s
  requireFido2UserVerification: false
```

## User workflow

### Register a security key

```console
$ coder webauthn register --name "My YubiKey"
Starting WebAuthn registration...
Touch detected! Completing registration...
Security key "My YubiKey" registered (ID: a3497ce9-...)
```

### List registered keys

```console
$ coder webauthn list
a3497ce9-...  My YubiKey  created: 2026-02-12 11:59:23  last used: 2026-02-12 12:01:45
```

### Delete a key

```console
$ coder webauthn delete a3497ce9-14ef-45e2-b6e0-8c188914abe7
Credential a3497ce9-... deleted.
```

### SSH with FIDO2

When connecting to a workspace, the CLI automatically detects registered keys
and prompts for a touch:

```console
$ coder ssh my-workspace
Authenticating with FIDO2...
Touch detected! Verifying...
admin@my-workspace:~$
```

If the server requires FIDO2 and the user has no keys registered:

```console
$ coder ssh my-workspace
error: This server requires FIDO2 security key verification for workspace connections.
You have no security keys registered. Run 'coder webauthn register' to set up your key, then try again.
```

## Security properties

- **Physical presence**: Every workspace connection requires a physical key
  touch (when enforcement is enabled).
- **Replay protection**: Each connection JWT has a unique ID tracked by the
  server. A stolen JWT cannot be reused even within its validity window.
- **Single-use mode**: With `--fido2-token-duration 0s`, each JWT is valid for
  only one connection (10-second handshake window with JTI replay rejection).
- **User verification**: With `--require-fido2-user-verification`, the key's
  PIN or biometric must be provided in addition to touch.
- **Non-connection operations are unaffected**: Listing workspaces, creating
  templates, and other API calls work with the regular session token.
- **Workspace proxy auth is unaffected**: Server-side workspace proxies bypass
  FIDO2 enforcement since they are infrastructure, not end users.

## Limitations

- The web terminal in the Coder dashboard does not currently require FIDO2.
  Disable the web terminal if you need to close this gap.
- Workspace apps (code-server, JupyterLab, etc.) accessed through the dashboard
  proxy are not behind FIDO2 enforcement.
- The `coder-fido2` helper binary must be built and distributed separately from
  the main Coder binary due to its CGo dependency.
