# MCP Servers

Administrators can register external MCP servers that provide additional tools
for agent chat sessions. Configured servers are injected into or offered to
users during chat depending on the availability policy.

This is an admin-only feature accessible at **Agents** > **Settings** >
**Manage Agents** > **MCP Servers**.

## Add an MCP server

1. Navigate to **Agents** > **Settings** > **Manage Agents** >
   **MCP Servers**.
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

| Field          | Required | Description                                                                                                                   |
|----------------|----------|-------------------------------------------------------------------------------------------------------------------------------|
| `enabled`      | No       | Master toggle. Disabled servers are hidden from non-admin users.                                                              |
| `availability` | Yes      | Controls how the server appears in chat sessions. See [Availability policies](#availability-policies).                        |
| `model_intent` | No       | When enabled, requires the model to describe each tool call's purpose in natural language, shown as a status label in the UI. |

#### Availability policies

| Policy        | Behavior                                               |
|---------------|--------------------------------------------------------|
| `force_on`    | Always injected into every chat. Users cannot opt out. |
| `default_on`  | Pre-selected in new chats. Users can opt out.          |
| `default_off` | Available in the server list but users must opt in.    |

## Authentication

Each MCP server uses one of five authentication modes. When you change the
auth type, fields from the previous type are automatically cleared.

Secrets are never returned in API responses — boolean flags indicate whether
a value is set.

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

| Field                  | Description                     |
|------------------------|---------------------------------|
| `oauth2_client_secret` | OAuth2 client secret.           |
| `oauth2_scopes`        | Space-separated list of scopes. |

**Auto-discovery** — leave `oauth2_client_id`, `oauth2_auth_url`, and
`oauth2_token_url` empty. The server attempts discovery in this order:

1. RFC 9728 — Protected Resource Metadata
1. RFC 8414 — Authorization Server Metadata
1. RFC 7591 — Dynamic Client Registration

Users connect through a popup that redirects through the OAuth2 provider.
Tokens are stored per-user and refreshed automatically. Users can disconnect
via the UI or API to remove stored tokens.

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

## Permissions

| Action                        | Required role             |
|-------------------------------|---------------------------|
| Create, update, or delete     | Admin (deployment config) |
| View enabled servers          | Any authenticated user    |
| OAuth2 connect and disconnect | Any authenticated user    |

Non-admin users only see enabled servers. Sensitive fields such as API keys
and client secrets are redacted in API responses.
