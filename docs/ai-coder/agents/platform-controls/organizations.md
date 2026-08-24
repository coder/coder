# Organization scope

Coder Agents configuration is split between deployment-wide settings and organization-scoped settings.
Deployment-wide settings apply to every chat in the deployment.
Organization-scoped settings apply only to chats that belong to that organization.

This page describes which settings belong to each scope, what an upgrade does to existing settings, and which permissions control each resource.

Every deployment has at least the default organization, so the organization-scoped settings apply even when you never create a second organization.
Running more than 1 organization requires a [Premium license](../../../admin/users/organizations.md).

## What each scope owns

| Setting                         | Scope                  | Where to configure                                                         |
|---------------------------------|------------------------|----------------------------------------------------------------------------|
| AI providers and credentials    | Deployment             | **Admin settings** > **AI** > **Providers**                                |
| Chat models                     | Organization           | **Admin settings** > **AI** > **Models**                                   |
| Admin model overrides           | Organization           | **Admin settings** > **AI** > **Coder Agents** > **Organization settings** |
| Personal model overrides        | User, per organization | **Agents** > **Settings** > **Agents**                                     |
| Personal override toggle        | Deployment             | **Admin settings** > **AI** > **Coder Agents**                             |
| MCP servers                     | Organization           | **Admin settings** > **AI** > **Coder Agents** > **MCP servers**           |
| System prompt                   | Deployment             | **Admin settings** > **AI** > **Coder Agents** > **Instructions**          |
| Plan mode instructions          | Deployment             | **Admin settings** > **AI** > **Coder Agents** > **Instructions**          |
| Agent access to a template      | Template               | **Admin settings** > **AI** > **Coder Agents** > **Templates**             |
| Advisor runtime limits          | Deployment             | **Admin settings** > **AI** > **Coder Agents** > **Advisor**               |
| Autostop fallback and lifecycle | Deployment             | **Admin settings** > **AI** > **Coder Agents** > **Lifecycle**             |
| Data retention                  | Deployment             | **Admin settings** > **AI** > **Coder Agents** > **Lifecycle**             |
| Spend budgets                   | Deployment, per group  | **AI Governance**                                                          |

### AI providers stay deployment-wide

An AI provider holds the upstream endpoint and the credentials, so it remains a deployment-wide resource.
A chat model belongs to 1 organization and references 1 provider.
Organization admins select a provider for a model, but they cannot read the provider's key material, base URLs, or headers.

The organization model list reports provider availability alongside the models.
Each provider entry carries an `available` flag and, when the provider is unavailable, an `unavailable_reason` of `missing_api_key`, `fetch_failed`, or `user_api_key_required`.
The response also lists `unsupported_providers`, which are configured provider types that Coder Agents cannot use.

Refer to [Models](../models.md) for provider setup and model options.

## What an upgrade does

When a deployment upgrades to a release that scopes models and MCP servers to organizations, Coder migrates the existing configuration:

1. Every existing chat model moves to the default organization.
1. Every existing MCP server moves to the default organization, with its credentials intact.
1. Every migrated model and MCP server grants read access to the default organization's **Everyone** group, so current members keep access.
1. The single default model becomes the default model of the default organization.
   Each organization now has its own default model.
1. MCP server slugs become unique per organization instead of unique per deployment.
1. Existing admin and personal model overrides are deleted.
1. A deployment-wide template allowlist becomes the per-template **Allow Coder Agents to create workspaces using this template** setting.

Other organizations start with no models and no MCP servers.
Nothing is copied from the default organization.

> [!IMPORTANT]
> Model overrides are deleted rather than migrated.
> Admins must set the organization overrides again in the **Organization settings** section of **Admin settings** > **AI** > **Coder Agents**.
> Users must set their personal overrides again in **Agents** > **Settings** > **Agents**.

The template allowlist conversion follows these rules:

- A non-empty allowlist allows only the templates it lists, and blocks every other existing template.
- A missing, empty, or null allowlist leaves every template allowed.
- An unreadable allowlist value blocks every template, so review the template list after the upgrade.

The upgrade deletes the stored allowlist, so the original list cannot be recovered.
Templates created after the upgrade allow agents by default.

## New organizations start empty

A new organization has no chat models and no MCP servers.
An organization admin must add at least 1 model and mark it as the default before members can start a chat there.

Until a default model exists, chat requests in that organization fail with this response:

```json
{
  "message": "No chat model is available in this organization.",
  "detail": "Ask an organization administrator to configure and enable a chat model."
}
```

An organization with no MCP servers produces no error.
Chats in that organization offer no MCP servers.

## Permissions

Two RBAC resources control this configuration.
Both are organization-scoped and support the same 5 actions.

| Resource            | Actions                                       |
|---------------------|-----------------------------------------------|
| `chat_model_config` | `create`, `read`, `update`, `delete`, `share` |
| `mcp_server_config` | `create`, `read`, `update`, `delete`, `share` |

| Role                 | `chat_model_config`    | `mcp_server_config`                          |
|----------------------|------------------------|----------------------------------------------|
| Owner                | All actions            | All actions                                  |
| Organization admin   | All actions            | All actions                                  |
| Auditor              | `read`                 | Full list view through audit log read access |
| Organization auditor | `read`                 | No direct grant                              |
| Organization member  | Access through the ACL | Access through the ACL                       |

The **MCP servers** settings page is part of deployment settings, so opening it in the dashboard also requires permission to edit deployment configuration.
Organization admins without that permission can manage MCP servers through the API.

API tokens carry scopes in addition to roles.
The public scopes are `chat_model_config:read` and `chat_model_config:share`.
The remaining `chat_model_config` scopes and all `mcp_server_config` scopes are internal.
Coder records create, update, and delete operations on both resources in the audit log, and redacts secrets.

## Share configuration with members and groups

Each chat model and each MCP server has a group access list and a user access list.
An entry grants the `read` role, which lets a member use the model or the server.
Coder adds the organization's **Everyone** group to the list when you create a model or a server, so all members have access by default.

To change an access list, you need the `share` action on that model or server.
The `share` action also controls who can read the access list.
You can only add members and groups from the organization that owns the model or the server.

Models have an access list editor in the dashboard under **Model actions** > **Share model**.
MCP server access lists are available through the API only:

- `GET /api/experimental/organizations/{organization}/mcp-servers/{mcpserverconfig}/acl`
- `PATCH /api/experimental/organizations/{organization}/mcp-servers/{mcpserverconfig}/acl`

## API routes

The organization-scoped routes are experimental and may change between releases.

| Purpose                | Route                                                                                                    |
|------------------------|----------------------------------------------------------------------------------------------------------|
| List or create models  | `GET`, `POST /api/experimental/organizations/{organization}/chats/models`                                |
| Read or change a model | `GET`, `PATCH`, `DELETE .../chats/models/{model}`                                                        |
| Model access list      | `GET`, `PATCH .../chats/models/{model}/acl`                                                              |
| Admin overrides        | `GET .../chats/model-overrides`, `PUT .../chats/model-overrides/{context}`                               |
| Personal overrides     | `GET .../members/{user}/chats/model-overrides`, `PUT .../members/{user}/chats/model-overrides/{context}` |
| List or create servers | `GET`, `POST /api/experimental/organizations/{organization}/mcp-servers`                                 |
| Read or change server  | `GET`, `PATCH`, `DELETE .../mcp-servers/{mcpserverconfig}`                                               |
| Server access list     | `GET`, `PATCH .../mcp-servers/{mcpserverconfig}/acl`                                                     |
| Connect OAuth2         | `GET .../mcp-servers/{mcpserverconfig}/oauth2/connect`                                                   |

Two MCP routes stay outside the organization path:

- `GET /api/experimental/mcp/servers/{mcpServer}/oauth2/callback` is frozen, because OAuth2 providers hold this URL.
- `DELETE /api/experimental/mcp/servers/{mcpServer}/oauth2/disconnect` stays available to the token owner, so a former organization member can still delete a stored token.

The deployment-wide settings keep their own routes, such as `/api/experimental/chats/config/personal-model-overrides` for the personal override toggle and `/api/v2/ai/providers` for AI providers.

## Configure many organizations and templates

Coder has no CLI command for chat models or MCP servers.
To configure several organizations, call the organization routes above once per organization with an API token that holds the required permission.

Template access for agents does have a CLI flag and a search filter:

- `coder templates create --agents-allowed=false` creates a template that agents cannot use.
- `coder templates edit --org <organization> --agents-allowed=true <template>` allows agents on an existing template.
  An unset flag keeps the stored value.
- `GET /api/v2/templates?q=agents-allowed:false` lists the templates that block agents.
  Add the organization path segment, `GET /api/v2/organizations/{organization}/templates?q=agents-allowed:false`, to scope the list to 1 organization.

Refer to [Template Optimization](./template-optimization.md) for the per-template setting and its effect on template selection.
