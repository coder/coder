# MCP Servers

Organization admins can register external MCP servers that provide additional
tools for agent chat sessions. Each organization has its own set of MCP
servers, and chats only offer servers from the chat's organization. Configured
servers are injected into or offered to users during chat depending on the
availability policy.

This is an admin-only feature accessible at **AI Settings** > **Coder Agents** > **MCP servers**
(`/ai/settings/mcp-servers`). In multi-organization deployments, use the
organization picker to choose which organization's servers to manage. The
server list shows the picker when you can access more than one organization's
servers. The add and update views always show the target organization, as a
read-only field when only one organization is available.

## Add an MCP server

1. Navigate to **AI Settings** > **Coder Agents** > **MCP servers**.
1. Click **Add**.
1. Fill in the configuration fields described below.
1. Click **Save**.

### Identity

| Field          | Required | Description                                                   |
|----------------|----------|---------------------------------------------------------------|
| `display_name` | Yes      | Human-readable name shown to users in chat.                   |
| `slug`         | Yes      | URL-safe unique identifier, auto-generated from display name. |
| `description`  | No       | Brief summary of what the server provides.                    |
| `icon_url`     | No       | Emoji or image URL displayed alongside the server name.       |

### Connection

| Field       | Required | Description                                     |
|-------------|----------|-------------------------------------------------|
| `url`       | Yes      | The MCP server endpoint URL.                    |
| `transport` | Yes      | Transport protocol. `streamable_http` or `sse`. |

### Availability

| Field                   | Required | Description                                                                                                                         |
|-------------------------|----------|-------------------------------------------------------------------------------------------------------------------------------------|
| `enabled`               | No       | Master toggle. Disabled servers are hidden from non-admin users.                                                                    |
| `availability`          | Yes      | Controls how the server appears in chat sessions. See [Availability policies](#availability-policies).                              |
| `model_intent`          | No       | When enabled, requires the model to describe each tool call's purpose in natural language, shown as a status label in the UI.       |
| `forward_coder_headers` | No       | When enabled, forwards Coder identity headers on every outgoing MCP request. See [Coder identity headers](#coder-identity-headers). |

#### Availability policies

| Policy        | Behavior                                                                          |
|---------------|-----------------------------------------------------------------------------------|
| `force_on`    | Injected into every chat whose owner has ACL access to the server. No opting out. |
| `default_on`  | Pre-selected in new chats. Users can opt out.                                     |
| `default_off` | Available in the server list but users must opt in.                               |

## Authentication

Each MCP server uses one of five authentication modes. When you change the
auth type, fields from the previous type are automatically cleared.

OAuth2 client secrets, API keys, and custom headers are never returned in API responses.
Boolean flags indicate whether each value is set.

### None

No credentials are sent. Use this for servers that do not require
authentication.

### OAuth2

Per-user authorization. The administrator configures the OAuth2 provider, and
each user independently completes the authorization flow.

**Manual configuration** — provide all three fields together:

| Field              | Description                 |
|--------------------|-----------------------------|
| `oauth2_client_id` | OAuth2 client ID.           |
| `oauth2_auth_url`  | Authorization endpoint URL. |
| `oauth2_token_url` | Token endpoint URL.         |

Optional fields:

| Field                   | Description                               |
|-------------------------|-------------------------------------------|
| `oauth2_client_secret`  | OAuth2 client secret.                     |
| `oauth2_scopes`         | Space-separated list of scopes.           |
| `oauth2_revocation_url` | Token revocation endpoint URL (RFC 7009). |

The revocation endpoint must use HTTPS.
Loopback URLs may use HTTP for local development and tests.

**Auto-discovery** — leave `oauth2_client_id`, `oauth2_auth_url`, and
`oauth2_token_url` empty. The server attempts discovery in this order:

1. RFC 9728 — Protected Resource Metadata
1. RFC 8414 — Authorization Server Metadata
1. RFC 7591 — Dynamic Client Registration

Auto-discovery also records the provider's `revocation_endpoint` from the
RFC 8414 metadata when advertised. An explicit `oauth2_revocation_url` in
the request takes precedence over the discovered value.

Users connect through a popup that redirects through the OAuth2 provider.
Tokens are stored per-user and refreshed automatically. Users can disconnect
via the UI or API to remove stored tokens. When a revocation endpoint is
configured, disconnecting also asks the provider to revoke the token
(RFC 7009). Provider revocation is best-effort: the stored token is always
deleted from Coder, and the disconnect response reports whether provider
revocation succeeded via `token_revoked` and `token_revocation_error`.

### API key

A static key sent as a header on every request.

| Field            | Required | Description                          |
|------------------|----------|--------------------------------------|
| `api_key_header` | Yes      | Header name (e.g., `Authorization`). |
| `api_key_value`  | Yes      | Secret value sent in the header.     |

### Custom headers

Arbitrary key-value header pairs sent on every request. At least one header
is required when this mode is selected.

### User OIDC Identity

Forwards the calling user's OIDC access token (stored in
`user_links.oauth_access_token`) to the MCP server as an
`Authorization: Bearer <token>` header. The token is refreshed
transparently before each request if it has expired or is close to
expiring.

No admin-configurable fields. No per-user connect step.

**Limitation**: this auth mode only works for users who authenticated to
Coder via OIDC. Users who logged in with password or GitHub will see
requests sent without an authorization header, and the upstream MCP
server is expected to respond with 401.

## Tool governance

Control which tools from a server are available in chat:

| Field             | Description                                                                           |
|-------------------|---------------------------------------------------------------------------------------|
| `tool_allow_list` | If non-empty, only the listed tool names are exposed. An empty list allows all tools. |
| `tool_deny_list`  | Listed tool names are always blocked, even if they appear in the allow list.          |

## Coder identity headers

MCP servers configured with `forward_coder_headers = true` receive Coder identity headers on every outgoing request.
When the server config has a signing secret, Coder also signs the request body and the effective identity header values.

| Header                        | Description                                                                               |
|-------------------------------|-------------------------------------------------------------------------------------------|
| `X-Coder-Owner-Id`            | Coder user who owns the chat that issued the tool call.                                   |
| `X-Coder-Chat-Id`             | Top-level parent chat ID. For root chats, this is the chat's own ID.                      |
| `X-Coder-Subchat-Id`          | Subchat ID. This header is absent for root chats.                                         |
| `X-Coder-Workspace-Id`        | Workspace associated with the chat. This header is absent when the chat has no workspace. |
| `X-Coder-Signature-Timestamp` | Unix timestamp in seconds used to limit replay.                                           |
| `X-Coder-Signature`           | Request signature in the form `v1=<lowercase hexadecimal HMAC-SHA256>`.                   |

Coder sends the same identity headers to LLM providers, so a first-party MCP server can correlate a tool call with the originating chat.

### Manage the signing secret

Coder generates a 32-byte random signing secret when you create or update a server with **Forward Coder identity headers** enabled and the server has no secret.
The mutation response returns the hexadecimal `signing_secret` once.
Later list and get responses omit `signing_secret` and return only `has_signing_secret`.

> [!WARNING]
> Copy the signing secret before you close the response.
> Coder cannot display the same secret again, so losing it requires regeneration and receiver reconfiguration.

To replace the secret in the UI, open the server's actions menu and select **Regenerate signing secret**.
You can also send `POST /api/experimental/organizations/{organization}/mcp-servers/{id}/regenerate-signing-secret`.
The response returns the new `signing_secret` once, and the old secret stops verifying requests immediately.

### Signature format

Coder builds this canonical string from the outgoing request.
The lines use `\n` separators with no trailing newline:

```txt
v1
<timestamp from X-Coder-Signature-Timestamp>
<HTTP method, uppercase>
<request path including query, for example /api/mcp?x=1>
<lowercase hexadecimal SHA-256 of the exact request body bytes>
owner=<value of X-Coder-Owner-Id>
chat=<value of X-Coder-Chat-Id>
subchat=<value of X-Coder-Subchat-Id>
workspace=<value of X-Coder-Workspace-Id>
```

An absent identity header contributes an empty value after the equals sign.
A request without a body uses the SHA-256 hash of the empty byte string.
Coder sets `X-Coder-Signature` to `v1=` followed by the lowercase hexadecimal HMAC-SHA256 of the canonical string, keyed with the server's signing secret.
The `v1=` prefix identifies the signing algorithm version.

If an auth header for the configured `auth_type` collides with an identity header, the auth header wins.
Coder signs the effective header value that the request sends.

### Verify signatures

The receiver must hash the raw request body before JSON parsing or other transformations.
The following TypeScript example verifies the signature and enforces a 300-second timestamp window:

```ts
import { createHash, createHmac, timingSafeEqual } from "node:crypto";

type VerifyCoderRequest = {
  method: string;
  pathWithQuery: string;
  rawBody: Uint8Array;
  headers: Headers;
  signingSecret: string;
  nowSeconds?: number;
};

export function verifyCoderRequest({
  method,
  pathWithQuery,
  rawBody,
  headers,
  signingSecret,
  nowSeconds = Math.floor(Date.now() / 1000),
}: VerifyCoderRequest): boolean {
  const timestamp = headers.get("x-coder-signature-timestamp") ?? "";
  const received = headers.get("x-coder-signature") ?? "";
  if (!/^\d+$/.test(timestamp) || !received.startsWith("v1=")) {
    return false;
  }

  const timestampSeconds = Number(timestamp);
  if (
    !Number.isSafeInteger(timestampSeconds) ||
    Math.abs(nowSeconds - timestampSeconds) > 300
  ) {
    return false;
  }

  const bodyHash = createHash("sha256").update(rawBody).digest("hex");
  const canonical = [
    "v1",
    timestamp,
    method.toUpperCase(),
    pathWithQuery,
    bodyHash,
    `owner=${headers.get("x-coder-owner-id") ?? ""}`,
    `chat=${headers.get("x-coder-chat-id") ?? ""}`,
    `subchat=${headers.get("x-coder-subchat-id") ?? ""}`,
    `workspace=${headers.get("x-coder-workspace-id") ?? ""}`,
  ].join("\n");
  const expected = `v1=${createHmac("sha256", signingSecret)
    .update(canonical)
    .digest("hex")}`;
  const receivedBytes = Buffer.from(received, "utf8");
  const expectedBytes = Buffer.from(expected, "utf8");

  return (
    receivedBytes.length === expectedBytes.length &&
    timingSafeEqual(receivedBytes, expectedBytes)
  );
}
```

Use the raw request target for `pathWithQuery`, including its leading slash and query string.
Receivers MUST use constant-time comparison for the signature.
Receivers MUST treat the identity headers as trustworthy only after signature verification succeeds.
Reject requests when the timestamp differs from the receiver's current time by more than 300 seconds.
This timestamp window is the `v1` replay bound because `v1` has no nonce or replay cache.

Because the identity headers disclose chat identity, **Forward Coder identity headers** is off by default.
Enable it only for first-party or trusted internal MCP servers.

## Permissions

| Action                                                   | Required role              |
|----------------------------------------------------------|----------------------------|
| Create, update, delete, or regenerate the signing secret | Organization admin         |
| View enabled servers                                     | Member granted through ACL |
| OAuth2 connect                                           | Member granted through ACL |
| OAuth2 disconnect                                        | Token owner                |
| Manage ACLs                                              | Organization admin         |

Disconnect only needs a valid session: users removed from the ACL or the
organization can still delete their stored token and revoke the provider
grant.

Members only see enabled servers in their own organizations. Sensitive fields
such as API keys and client secrets are redacted in API responses.

The **MCP servers** settings page is part of deployment settings, so opening it in the dashboard also requires permission to edit deployment configuration.
Organization admins without that permission can manage servers through the API.
Creating or updating a server with `auth_type` set to `user_oidc` also requires the `deployment_config:update` permission.

### Access control

Each server has a group and user ACL that controls which members can see and
use it. New servers grant read access to the organization's **Everyone** group,
so all members have access by default. Admins can remove the Everyone entry and
grant specific groups or users instead through the API
(`GET`/`PATCH /api/experimental/organizations/{organization}/mcp-servers/{id}/acl`); there is no ACL editor
in the settings page. ACL management is available in all editions and does not
require an enterprise entitlement. ACL changes are recorded in the audit log.

Revoking access stops a member from newly selecting the server in any chat,
but chats that already have the server selected keep using it, the same way
existing workspaces keep running after template access is revoked. To cut
off existing chats as well, disable or delete the server.
