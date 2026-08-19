# AI Agent MCP Gateway

Status: design specification. Nothing described here should be assumed to
exist. Every section is written as work to be done, in normative voice, so
that an implementer can build it from this document alone. Where the
document refers to behavior Coder already has independently of this
feature, it says so explicitly.

Companion documents:

- `AI_AGENT_IDENTITY_SPEC.md` specifies who an agent is and how its API key
  permissions are bounded by its sponsor.
- `AI_AGENT_SANDBOX_SPEC.md` specifies the guest credential and egress
  boundary.
- `AI_AGENT_SECURITY_ARCHITECTURE.md` relates both mechanisms to the wider
  AI security architecture.

Implementation references in this document are non-normative pointers to
one implementation. The requirements remain authoritative even if line
numbers move.

## Problem

A sandboxed AI agent needs to call third-party tools, but the two obvious
ways to enable that violate the sandbox boundary:

1. Giving the guest direct network access to every MCP server creates a new
   egress hole for each server and lets any process in the guest contact the
   third party outside Coder governance.
2. Giving the guest the sponsor's OAuth token, API key, or custom headers
   turns a short-lived tool call into a reusable credential that prompt
   injection can read and exfiltrate.

The sandbox already has one necessary outbound destination: the Coder
access URL used by its agent control channel. The MCP gateway must reuse
that destination. A sandbox must call an MCP endpoint under the same access
URL, and coderd or a standalone AI Gateway must make the upstream request.
No MCP server hostname needs to be added to the guest egress policy.

The resulting boundary is:

- the guest uses its scoped AI identity session token for Coder API calls,
  alongside the separate workspace-agent daemon credential;
- the gateway authenticates and authorizes the AI identity token;
- the gateway resolves the human sponsor's third-party credential;
- the gateway applies administrator tool policy; and
- the gateway injects the credential into the upstream request without ever
  returning it to the guest.

## Solution

Add an MCP data plane to AI Gateway at:

```text
<accessURL>/api/v2/ai-gateway/mcp/{server-slug}
```

An administrator registers an MCP server, selects its upstream transport
and authentication mode, and defines its tool policy. An AI agent sends
streamable HTTP MCP requests to the gateway with its Coder AI session token.
The gateway performs four decisions in order:

1. **Authentication.** Validate the full Coder API token and resolve its
   initiator.
2. **Authorization.** Build the canonical RBAC subject and require
   `mcp_gateway:use` for the addressed server configuration.
3. **Credential delegation.** Resolve and refresh the sponsor's credential
   through a purpose-built token broker that cannot otherwise read personal
   data.
4. **Tool governance.** Filter tool discovery and reject disallowed calls
   before they reach the upstream server.

The gateway must behave identically whether aibridged runs embedded in
coderd or as a standalone service. The dRPC protocol between aibridged and
coderd must carry authorization, registry, and credential-broker operations
so the standalone service never needs direct database access. Protocol
version 1.5 adds the governed server lookup
`GetMCPGatewayServerConfig(slug)`; see
`coderd/aibridged/proto/version.go:21-35`.

## Security properties

The design must preserve these properties:

1. **No new guest egress destination.** The gateway lives under the coderd
   access URL already required by the guest control channel.
2. **No sponsor credential in the guest.** OAuth access tokens, refresh
   tokens, configured API keys, and custom headers terminate at the gateway.
3. **Live sponsor ceiling.** An AI identity can use the gateway only while
   its sponsor's current roles, the key's scope set, and the key's resource
   allow-list all permit it.
4. **Server concealment.** Unknown, disabled, and unauthorized server slugs
   are indistinguishable to the caller.
5. **Policy before execution.** A denied `tools/call` never reaches the
   upstream MCP server.
6. **Credential scrubbing.** No Coder authentication header or cookie is
   forwarded upstream.
7. **Fail closed.** Missing identity, sponsor, provider, policy, or
   credential data denies the operation rather than falling back to an
   unauthenticated upstream call.

## Registry and administration

### Server configuration

`mcp_server_configs` must be the authoritative registry. Each entry must
include:

| Field                                  | Requirement                                                                     |
|----------------------------------------|---------------------------------------------------------------------------------|
| `slug`                                 | Stable path segment used by the gateway URL.                                    |
| `url`                                  | Upstream MCP endpoint.                                                          |
| `transport`                            | `streamable_http` or `sse`; only `streamable_http` is executable initially.     |
| `auth_type`                            | `none`, `external_auth`, `api_key`, `custom_headers`, `oauth2`, or `user_oidc`. |
| `external_auth_provider_id`            | Required when `auth_type = 'external_auth'`.                                    |
| `tool_rules`                           | JSON array of exact tool-name decisions.                                        |
| `tool_default`                         | `enabled` or `disabled` for tools without an explicit rule.                     |
| `tool_allow_list` and `tool_deny_list` | Existing exact-match compatibility policy.                                      |
| `enabled`                              | Disabled entries must not be reachable through the gateway.                     |

The schema requirements are represented in
`coderd/database/migrations/000572_mcp_server_external_auth_tool_rules_audit.up.sql:1-17`.

The external-auth provider ID must name a configured Coder external-auth
provider. The management API must reject a missing or unknown provider
rather than accepting a configuration that can never broker a credential.

### Management API

The canonical management API must be:

```text
/api/v2/ai-gateway/mcp-servers
/api/v2/ai-gateway/mcp-servers/{id}
```

It must support list, get, create, patch, and delete. The existing
experimental `/api/experimental/mcp/servers` path may remain as a
compatibility alias, but new clients must use the AI Gateway path. The
canonical SDK path is defined at `codersdk/mcp.go:12` and the route is
registered at `coderd/coderd.go:2155-2159`.

The administration surface must allow an administrator to select an
external-auth provider, configure exact per-tool rules and the server
default, and copy the resulting gateway URL. UI component names are not
part of this specification.

## Endpoint contract

### URL and methods

The public endpoint is:

```text
POST   <accessURL>/api/v2/ai-gateway/mcp/{server-slug}
GET    <accessURL>/api/v2/ai-gateway/mcp/{server-slug}
DELETE <accessURL>/api/v2/ai-gateway/mcp/{server-slug}
```

`POST` carries JSON-RPC requests. `GET` and `DELETE` support the streamable
HTTP session lifecycle. Any other method must return `405 Method Not
Allowed` with:

```text
Allow: GET, POST, DELETE
```

The client-facing transport is MCP streamable HTTP. The gateway must accept
single JSON-RPC request objects and non-empty JSON-RPC batches. It must
preserve JSON-RPC notifications and methods it does not govern.

An upstream configured with `transport = 'sse'` must return
`501 Not Implemented`. An unknown transport must also return `501` rather
than being guessed or forwarded. The transport checks are implemented at
`coderd/aibridged/mcp_gateway.go:290-297`.

### Authentication headers

The gateway handler must extract the full Coder token from these headers, in
this precedence order:

1. `X-Coder-AI-Governance-Token: <token>`
2. `Authorization: Bearer <token>`
3. `X-Api-Key: <token>`

The extraction contract is represented in
`coderd/aibridge/aibridge.go:63-80`. The outer AI Gateway middleware may
classify `X-Coder-AI-Governance-Token` as BYOK and reject it when BYOK is
disabled. Agents must therefore use the standard bearer form:

```text
Authorization: Bearer <AI identity session token>
```

Managed sandbox code supplies that token to guest processes as
`CODER_SESSION_TOKEN`. The create-script handoff calls the same value
`CODER_AI_SESSION_TOKEN` before the runtime places it in the guest. Cookies
are not an authentication mechanism for this endpoint. Missing or invalid
credentials must return `401 Unauthorized`.

Before any upstream request, the gateway must construct a new request and
copy only MCP transport headers:

- `Accept`
- `Content-Type`
- `Last-Event-ID`
- `Mcp-Session-Id`
- `MCP-Protocol-Version`
- `User-Agent`

It must not copy `Authorization`, `X-Api-Key`,
`X-Coder-AI-Governance-Token`, cookies, or arbitrary caller headers.
Configured upstream credentials are added only after this scrub. The
allow-list is implemented at `coderd/aibridged/mcp_gateway.go:550-584`.

### HTTP status codes

| Status                      | Meaning                                                                                          |
|-----------------------------|--------------------------------------------------------------------------------------------------|
| `200 OK`                    | JSON-RPC success or a gateway-generated JSON-RPC error.                                          |
| `204 No Content`            | A notification or locally handled request produces no response object.                           |
| `401 Unauthorized`          | The Coder token is missing, invalid, expired, revoked, or belongs to an invalid identity.        |
| `403 Forbidden`             | A gateway-wide control such as the AI budget or BYOK policy rejects the authenticated initiator. |
| `404 Not Found`             | The slug is unknown, disabled, or not authorized for the canonical subject.                      |
| `405 Method Not Allowed`    | The HTTP method is not `GET`, `POST`, or `DELETE`.                                               |
| `500 Internal Server Error` | Registry or authorization infrastructure failed.                                                 |
| `501 Not Implemented`       | The configured upstream transport is `sse` or otherwise unsupported.                             |
| `502 Bad Gateway`           | The upstream request could not be completed.                                                     |

A second upstream `401` after the one permitted credential refresh attempt
may be forwarded as the upstream response. Policy-generated denials and
re-authentication instructions are JSON-RPC errors over HTTP `200`, not
HTTP authentication failures.

### JSON-RPC errors

Malformed JSON, an empty body, and an empty batch must return JSON-RPC parse
error `-32700` with `id: null`.

A malformed `tools/call` request must return `-32602`:

```json
{
  "jsonrpc": "2.0",
  "id": 7,
  "error": {
    "code": -32602,
    "message": "invalid tools/call parameters"
  }
}
```

A denied tool call must return `-32603`, include the exact tool name in both
the message and structured data, and must not contact the upstream:

```json
{
  "jsonrpc": "2.0",
  "id": 8,
  "error": {
    "code": -32603,
    "message": "tool \"delete_repository\" denied by MCP gateway policy",
    "data": {
      "tool": "delete_repository"
    }
  }
}
```

The denial shape is defined at
`coderd/aibridged/mcp_gateway.go:193-217`.

When the sponsor must authenticate again, the gateway must return `-32603`
without contacting the upstream:

```json
{
  "jsonrpc": "2.0",
  "id": 9,
  "error": {
    "code": -32603,
    "message": "Authentication is required for MCP provider \"github\". Ask the user to authenticate using the provided URL.",
    "data": {
      "reauth_url": "https://coder.example.com/external-auth/github",
      "provider_id": "github"
    }
  }
}
```

The payload fields are constructed at
`coderd/aibridged/mcp_gateway.go:488-502`. The client must surface the URL
to the human sponsor. The AI agent must not attempt to complete the browser
flow itself.

For a batch, the gateway must remove denied calls from the upstream batch,
forward the remaining items, then merge the local denial responses into the
JSON-RPC response array.

## Authorization model

### Resource and scope

RBAC must define resource `mcp_gateway` with action `use`, producing the
low-level scope names:

```text
mcp_gateway:*
mcp_gateway:use
```

The resource action is defined at
`coderd/rbac/policy/policy.go:412-416`, and generated scope constants include
`mcp_gateway:use` at `coderd/rbac/scopes_constants_gen.go:76`.

These low-level scope names must be internal-only. They must not appear in
`coderd/rbac/scopes_catalog.go`'s `externalLowLevel` map, whose contract is
that unlisted low-level scopes are not user-requestable
(`coderd/rbac/scopes_catalog.go:8-11`). This prevents ordinary token-minting
surfaces from asking for a narrowly named MCP gateway privilege. Internal
AI identity profile minting may assign it after validating the complete
profile.

Broad compatibility scopes such as legacy `all` retain their existing
meaning. Internal-only naming is not a claim that every non-AI session is
categorically unable to reach the gateway. The real decision remains the
intersection of live roles and the token's effective scope.

Both chat and workspace or sandbox AI profiles must include
`mcp_gateway:use` and a wildcard `mcp_gateway` resource allow-list entry.
The profile assignments are represented at
`coderd/aiagentidentity/profile.go:24-42` and
`coderd/aiagentidentity/profile.go:65-89`.

### Canonical subject

The data plane must not authorize the API key's stored user row in
isolation. It must use the same canonical subject construction as normal
API authentication:

- for a human key, use that human's live roles;
- for an AI key, resolve `ai_agents.owner_user_id` and use the sponsor's live
  roles;
- intersect those roles with the key's scope set;
- intersect the scope allow-list with the key's persisted resource
  allow-list; and
- preserve the AI user as the acting identity for attribution.

`APIKeyRBACSubject` implements the sponsor role selection at
`coderd/httpmw/apikey.go:508-563`. `APIKey.ScopeSet()` carries the persisted
allow-list into scope expansion at
`coderd/database/modelmethods.go:368-383`.

Authorization must call:

```text
Authorize(subject, use, mcp_gateway.WithID(mcp_server_config_id))
```

An unauthorized result must become a concealed `404`, not a distinguishable
`403`. Infrastructure errors remain `500`. The dRPC check is represented at
`coderd/aibridgedserver/aibridgedserver.go:1011-1052`.

This is the first scope-enforcing authorization check on the AI Gateway data
plane. The model-provider routes still call `IsAuthorized`, which validates
key liveness but has a documented authorization TODO at
`coderd/aibridged/proto/aibridged.proto:38-45` and
`coderd/aibridgedserver/aibridgedserver.go:894-916`. Implementers must not
copy that weaker check into new governed routes.

## Token delegation boundary

### Why AI scopes cannot read sponsor credentials

An AI key must never receive user personal read or update permissions merely
because it needs a third-party token. Workspace and sandbox profiles must
contain no user-data scope. Profile validation must reject API key
management, user secrets, user skills, user mutation, and personal-data
permissions; see `coderd/aiagentidentity/profile.go:146-203`.

The gateway therefore needs an explicit server-side delegation boundary
that is narrower than system access.

### Broker actor

`dbauthz.AsMCPGatewayTokenBroker` must install a site-wide internal actor
with exactly these user-resource actions:

```text
read_personal
update_personal
```

`read_personal` permits reading the sponsor's external-auth link.
`update_personal` permits persisting a refreshed token. The actor must have
no general user read, create, update, or delete permission, and no read
permission on workspaces, templates, API keys, or organizations. The role is
defined at `coderd/database/dbauthz/dbauthz.go:870-886` and its negative
permission tests are at
`coderd/database/dbauthz/dbauthz_test.go:7724-7768`.

No gateway handler may use `AsSystemRestricted` for the external-auth link
read or refresh. System access may be used only for the preliminary AI
identity lookup needed before the broker actor exists.

### Sponsor resolution

Credential ownership must be resolved as follows:

| API key owner | Credential owner           |
|---------------|----------------------------|
| Human user    | The key owner.             |
| AI agent user | `ai_agents.owner_user_id`. |

For an AI key, the broker must never query an external-auth link under the
AI user's ID. The authoritative sponsor resolution is represented at
`coderd/aibridgedserver/aibridgedserver.go:840-860`.

Authentication must first verify that the key, AI user, AI agent row, and
human sponsor are all live. A deleted, suspended, malformed, or non-human
sponsor identity must fail closed.

### Refresh and re-authentication

For `auth_type = 'external_auth'`, the broker must:

1. find the configured external-auth provider;
2. read the sponsor's link through the token-broker actor;
3. call the provider's `externalauth.Config.RefreshToken` path;
4. persist refreshed token state through the same actor; and
5. return only the access token to aibridged.

A missing link or an invalid token must not be returned as a dRPC error.
Instead, the broker must return:

```text
reauth_required = true
reauth_url      = <accessURL>/external-auth/{provider-id}
provider_id     = {provider-id}
```

Unexpected provider, database, or refresh failures remain internal errors.
The broker behavior is represented at
`coderd/aibridgedserver/aibridgedserver.go:820-891`.

Aibridged may cache an external-auth access token for 60 seconds, keyed by
`(initiator_id, mcp_server_config_id)`. It must evict that entry and perform
one fresh broker call when the upstream returns `401 Unauthorized`. It must
retry the upstream request at most once. The cache key and TTL are defined
at `coderd/aibridged/mcp_gateway.go:42-53`; eviction and retry are at
`coderd/aibridged/mcp_gateway.go:390-408` and
`coderd/aibridged/mcp_gateway.go:520-532`.

### Other authentication modes

The gateway must handle configured server credentials as follows:

| Auth type        | Gateway behavior                                               |
|------------------|----------------------------------------------------------------|
| `none`           | Add no credential.                                             |
| `external_auth`  | Inject `Authorization: Bearer <sponsor-access-token>`.         |
| `api_key`        | Inject the configured value under the configured header name.  |
| `custom_headers` | Decode and inject the configured header map.                   |
| `oauth2`         | Reject with JSON-RPC `-32603`; do not forward unauthenticated. |
| `user_oidc`      | Reject with JSON-RPC `-32603`; do not forward unauthenticated. |

The rejection of unsupported authentication modes is represented at
`coderd/aibridged/mcp_gateway.go:438-517`.

## Tool governance

### Policy evaluation

Tool policy must be deterministic and based on the exact upstream tool
name. It has three layers, all of which must pass:

1. **Compatibility exact lists.** If `tool_allow_list` is non-empty, the
   tool must be an exact member. Otherwise an exact member of
   `tool_deny_list` is denied.
2. **Explicit rule and server default.** The first exact `tool_rules` entry
   for the tool decides it. If no rule matches, `tool_default = 'disabled'`
   denies and any other valid default enables. Registry validation must
   reject empty names and duplicate exact names.
3. **External-auth compatibility regexes.** A matching deny regex vetoes the
   tool. If an allow regex exists, the tool must match it. These regexes are
   combined with the registry policy and cannot re-enable a tool denied by
   an earlier layer.

The exact-list, rule, and default semantics are implemented at
`coderd/mcptools/policy.go:9-34`. The additional regex gates are implemented
at `coderd/aibridged/mcp_gateway.go:135-175`. Rule normalization rejects
empty and duplicate names at `coderd/mcp.go:1751-1765`.

### Discovery and invocation

For `tools/list`, the gateway must forward the request, inspect the matching
JSON-RPC response by ID, and remove every tool whose exact name is not
allowed. It must preserve other result fields such as pagination cursors.
The filter is represented at
`coderd/aibridged/mcp_gateway.go:587-657`.

For `tools/call`, the gateway must parse `params.name` before forwarding. A
disallowed call must receive the local `-32603` error described above. It
must not reach the upstream even when it appears in a mixed batch.

Filtering discovery is not sufficient by itself. Clients may remember a
tool from an earlier policy revision or construct a call directly, so the
invocation check is the security boundary.

## Audit and attribution

### Configuration audit

Create, update, and delete operations on the server registry must write
standard `audit_logs` entries with resource type `mcp_server_config` and
actions create, write, and delete. The handlers initialize those actions at
`coderd/mcp.go:259-270`, `coderd/mcp.go:708-719`, and
`coderd/mcp.go:1104-1115`.

Audit diffs may include public configuration and tool policy. They must
classify stored credentials as secrets. At minimum, OAuth client secrets,
API key header and value fields, custom headers, and encryption key IDs must
not appear in plaintext audit diffs. The audit table classification is at
`enterprise/audit/table.go:430-465`.

### Tool-call audit

Every accepted or denied `tools/call` should eventually produce a dedicated
per-call record with:

- AI initiator ID;
- human sponsor ID;
- API key ID;
- MCP server configuration ID and slug;
- exact tool name;
- allow or deny result;
- upstream status or invocation error; and
- timestamps and request correlation identifiers.

These records should use the existing `aibridge_tool_usages` family only if
its attribution model can represent an MCP gateway call without fabricating
an LLM interception. Otherwise a purpose-built record is required.

Per-call gateway recording is not implemented in the referenced data plane.
Existing `RecordToolUsage` calls describe model interception tool output and
are not evidence that gateway requests are recorded.

## Invariants

Drive tests from these:

1. **Same-host reachability.** A sandbox can reach every configured MCP
   server through the coderd access URL without an egress exception for the
   upstream host.
2. **Credential non-materialization.** No sponsor OAuth token, refresh token,
   API key, or configured custom header appears in the guest, gateway
   response, log, or audit diff.
3. **Canonical authorization.** Gateway use is the intersection of sponsor
   roles, key scopes, and the key resource allow-list.
4. **Sponsor resolution.** An AI key always resolves credentials through
   `ai_agents.owner_user_id`, never through the AI user row.
5. **Broker least privilege.** Sponsor token reads and refresh writes use
   only the MCP token-broker actor.
6. **Concealment.** Unknown, disabled, and unauthorized slugs all return
   `404`.
7. **Tool consistency.** The same policy function governs `tools/list` and
   `tools/call`.
8. **No denied forwarding.** A denied tool call produces no upstream network
   request.
9. **One-shot refresh.** An upstream `401` can trigger at most one credential
   eviction, one broker refresh, and one retry.
10. **Coder credential scrubbing.** No caller Coder token or cookie is copied
    into an upstream request.
11. **Unsupported auth fails closed.** `oauth2`, `user_oidc`, unknown auth
    types, and missing required fields never cause an unauthenticated
    upstream request.
12. **Standalone parity.** Embedded and standalone gateway deployments use
    the same HTTP handler and dRPC decisions.

## Implementation order

1. Add the `mcp_gateway` RBAC resource, internal scope constants, and AI
   profile grants with matching resource allow-list entries.
2. Extend the registry schema with external-auth binding and explicit tool
   policy, then add canonical management routes and CRUD audit.
3. Add `AuthorizeMCPGateway` using the canonical API key subject.
4. Add the least-privilege token-broker actor and
   `GetMCPUpstreamCredential` sponsor resolution.
5. Add `GetMCPGatewayServerConfig` to the dRPC protocol and implement the
   streamable HTTP proxy, credential scrub, cache, refresh, and tool policy.
6. Add the administration UI for provider binding, gateway URL display, and
   exact tool-rule configuration.
7. Add per-call gateway recording with sponsor attribution.
8. Wire sandbox templates and agent clients to use their scoped AI session
   token against the same coderd access URL.

Each step must be independently testable. No data-plane step may depend on
secrets being delivered into a workspace.

## Known gaps and not yet implemented

- **SSE upstream transport.** Registry entries may name `sse`, but the
  gateway returns `501` and does not proxy them.
- **Bespoke OAuth auth types.** `oauth2` and `user_oidc` registry modes are
  rejected by the gateway. They must not be forwarded without credentials.
- **Older MCP OAuth subsystem.** The existing `mcp_server_user_tokens`
  storage, OAuth connect and callback routes, and refresh machinery remain
  in place for older MCP clients. The gateway does not use them for sponsor
  external-auth delegation, and this specification does not require their
  removal.
- **Model-route scope enforcement.** Non-MCP model routes still rely on
  `IsAuthorized`, whose scope authorization TODO remains open.

The following were gaps in earlier drafts and are now implemented:

- **Per-call gateway recording.** Gateway `tools/call` requests, allowed and
  denied alike, create an `mcp-gateway` interception with denormalized
  sponsor attribution and per-call rows in `aibridge_tool_usages`.
- **Reserved provider name.** AI provider creation rejects the name `mcp`
  after trimming and case folding.
- **Token delivery to declared templates.** The
  `data.coder_workspace_ai_agent` data source opts a template into AI
  identity minting at import time and exposes the scoped session token for
  the author to inject; the `coder_ai_agent` rich-parameter opt-in remains
  as a deprecated fallback.
