# Authentication

> [!NOTE]
> AI Gateway requires the [AI Governance Add-On](../ai-governance.md).

AI Gateway uses standard Coder API tokens to authenticate clients.
The same token a user uses to interact with the Coder API grants access to AI Gateway.
No additional credentials or separate login flow is required.

## Token format

Coder tokens are opaque strings with the format `<ID>-<secret>`:

- **ID**: 10 characters, stored in plaintext and used to look up the key record.
- **Secret**: 22 characters, stored only as a bcrypt hash; never persisted in plaintext.

Tokens are not JWTs and carry no embedded claims.
All validation happens server-side by looking up the key record in the database.

Generate a token from the Coder UI (**Account settings > Tokens**) or via the CLI:

```sh
coder tokens create --lifetime 30d my-ai-token
```

## How AI Gateway validates a token

When AI Gateway receives a request, `aibridged` runs the following checks in order.
Any failure short-circuits the sequence and returns an error.

| Step | Check                                                        | Error on failure |
|------|--------------------------------------------------------------|------------------|
| 1    | Token matches `<ID>-<secret>` format                         | `ErrInvalidKey`  |
| 2    | Key ID exists in the database                                | `ErrUnknownKey`  |
| 3    | Key has not passed its `expires_at` timestamp                | `ErrExpired`     |
| 4    | bcrypt hash of the secret matches the stored `hashed_secret` | `ErrInvalidKey`  |
| 5    | Owner user record exists                                     | `ErrUnknownUser` |
| 6    | Owner has not been deleted                                   | `ErrDeletedUser` |
| 7    | Owner is not a system user                                   | `ErrSystemUser`  |

On success, `aibridged` returns the owner's user ID, API key ID, and username
for attribution in session records and audit logs.

> [!NOTE]
> AI Gateway does not currently update the `last_used` timestamp on a token
> when it is used to authenticate.
> This is a known limitation and does not affect the validity check.

## Authentication paths

There are two ways a client authenticates to AI Gateway,
depending on whether it uses the direct API or AI Gateway Proxy.

### Direct API

Clients that support a configurable base URL send requests directly to `coderd`
using standard Coder token authentication.

**How it works:**

1. Client sets its base URL to `https://<deployment-url>/api/v2/aibridge/<NAME>/`.
1. Client presents the Coder token in one of two headers:
   - `Authorization: Bearer <token>`
   - `X-Api-Key: <token>`
1. `coderd` validates the token via standard API key middleware.
1. `coderd` forwards the request to the configured upstream provider,
   injecting the deployment's upstream provider key.

This path is available to any client that supports a custom base URL,
such as Claude Code, Codex, Cline, and most OpenAI-compatible tools.
See [Client Configuration](./clients/index.md) for per-client setup instructions.

### Proxy mode (AI Gateway Proxy)

For clients that do not support a configurable base URL,
AI Gateway Proxy intercepts outbound HTTPS traffic using a MITM TLS proxy.
The Coder token travels in the HTTP proxy credentials during the `CONNECT` handshake.

**How it works:**

1. Client's environment is configured to route traffic through the proxy:

   ```sh
   HTTPS_PROXY=http://ignored:<coder-token>@<proxy-host>:8888
   ```

1. The proxy authenticates the token from the
   `Proxy-Authorization: Basic base64(ignored:<token>)` header.
1. The proxy intercepts TLS connections to allowlisted AI provider domains
   (`api.anthropic.com`, `api.openai.com`, `api.individual.githubcopilot.com`)
   using a MITM CA certificate.
1. All other HTTPS traffic passes through unmodified.

The MITM CA certificate must be installed on client machines.
Download it from:

```text
GET https://<deployment-url>/api/v2/aibridge/proxy/ca-cert.pem
```

See [AI Gateway Proxy](./ai-gateway-proxy/index.md) for full setup instructions,
including CA certificate installation and per-client configuration.

## Credential modes

AI Gateway supports two credential modes
that control whose upstream provider key is used when forwarding requests to the LLM provider.

### Centralized mode

The deployment's upstream provider key, configured by the admin in `CODER_AIBRIDGE_<PROVIDER>_KEY`,
is used for all requests.
Clients authenticate with their Coder token only.
No LLM credentials need to be distributed to users.

This is the default mode and the recommended approach for most deployments.

### BYOK mode (Bring Your Own Key)

Users supply their own LLM credentials in the `Authorization` or `X-Api-Key` header
alongside their Coder token.
AI Gateway forwards the user's key to the upstream provider
while adding a `X-Coder-AI-Governance-Token` header carrying the Coder token for attribution.

BYOK is useful in deployments where users already have individual provider accounts,
or where per-user billing is required.

## Token lifecycle

| Property          | Details                                                                                                                             |
|-------------------|-------------------------------------------------------------------------------------------------------------------------------------|
| **Expiry**        | Set at creation time via `--lifetime`. Expired tokens are rejected at validation step 3 with `ErrExpired`.                          |
| **Revocation**    | Tokens can be deleted from the UI or CLI (`coder tokens rm <name>`). Deleted tokens fail at validation step 2 with `ErrUnknownKey`. |
| **User deletion** | When a user is deleted, all their tokens fail at validation step 6 with `ErrDeletedUser`.                                           |
| **Rotation**      | Create a new token and update client configuration, then delete the old token.                                                      |

Keep tokens short-lived for non-interactive use cases (CI, automation).
Use the `--lifetime` flag to match the expected duration of the task.

## Next steps

- [Setup](./setup.md): Enable AI Gateway and configure providers
- [Client Configuration](./clients/index.md): Configure individual AI coding tools
- [Auditing AI Sessions](./audit.md): Review session records and attribution
