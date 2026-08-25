# Organization scope

Coder Agents configuration is split between deployment-wide settings and organization-scoped settings.
Deployment-wide settings apply to every chat in the deployment.
Organization-scoped settings apply only to chats that belong to that organization.

This page describes which settings belong to each scope, what an upgrade changes, and who can configure each setting.

Every deployment has at least the default organization, so the organization-scoped settings apply even when you never create a second organization.
Running more than 1 organization requires a [Premium license](../../../admin/users/organizations.md).

## What each scope owns

| Setting                         | Scope                  | Where to configure                                                         |
|---------------------------------|------------------------|----------------------------------------------------------------------------|
| AI providers and credentials    | Deployment             | **Admin settings** > **AI** > **Providers**                                |
| Chat models                     | Organization           | **Admin settings** > **AI** > **Models**                                   |
| Admin model overrides           | Organization           | **Admin settings** > **AI** > **Coder Agents** > **Organization settings** |
| Personal model overrides        | User, per organization | **Agents** > **Settings** > **Agents**                                     |
| Personal override toggle        | Deployment             | **Admin settings** > **AI** > **Coder Agents** > **Deployment settings**   |
| MCP servers                     | Organization           | **Admin settings** > **AI** > **Coder Agents** > **MCP servers**           |
| System prompt                   | Deployment             | **Admin settings** > **AI** > **Coder Agents** > **Instructions**          |
| Plan mode instructions          | Deployment             | **Admin settings** > **AI** > **Coder Agents** > **Instructions**          |
| Agent access to a template      | Template               | **Admin settings** > **AI** > **Coder Agents** > **Templates**             |
| Advisor runtime limits          | Deployment             | **Admin settings** > **AI** > **Coder Agents** > **Deployment settings**   |
| Virtual desktop provider        | Deployment             | **Admin settings** > **AI** > **Coder Agents** > **Deployment settings**   |
| Autostop fallback and lifecycle | Deployment             | **Admin settings** > **AI** > **Coder Agents** > **Lifecycle**             |
| Data retention                  | Deployment             | **Admin settings** > **AI** > **Coder Agents** > **Lifecycle**             |

The **Deployment settings** section holds the Advisor card and the Virtual desktop card.
Both cards appear on the **Coder Agents** page.

### AI providers stay deployment-wide

An AI provider holds the upstream endpoint and the credentials, so it stays a deployment-wide resource.
A chat model belongs to 1 organization and references 1 provider.
You select a provider when you add a model, but the dashboard never shows the provider's credentials.

Refer to [Models](../models.md) for provider setup and model options.

## What an upgrade does

When a deployment upgrades to a release that scopes models and MCP servers to organizations, Coder moves the existing configuration:

- Every existing chat model moves to the default organization.
- Every existing MCP server moves to the default organization, with its credentials intact.
- Every moved model and MCP server stays available to the default organization's **Everyone** group, so current members keep access.
- The previous default model becomes the default model of the default organization.
- Coder removes the existing admin and personal model overrides.
- Coder ignores the previous Advisor model override, so you must set it again.
- The deployment-wide template allowlist becomes the per-template **Allow Coder Agents to create workspaces using this template** setting.

Other organizations start with no models and no MCP servers.
Coder copies nothing from the default organization.

> [!IMPORTANT]
> Coder does not migrate the model overrides.
> Administrators must set the organization overrides again in the **Organization settings** section of **Admin settings** > **AI** > **Coder Agents**.
> Users must set their personal overrides again in **Agents** > **Settings** > **Agents**.

The template allowlist conversion follows these rules:

- A non-empty allowlist allows only the templates it lists, and blocks every other existing template.
- A missing or empty allowlist leaves every template allowed.
- An unreadable allowlist blocks every template, so review the template list after the upgrade.

The upgrade removes the stored allowlist, so you cannot recover the original list.
Templates created after the upgrade allow agents by default.

## Organizations without models or MCP servers

A new organization has no chat models and no MCP servers.
Add at least 1 model before members start a chat in that organization.

When an organization has no model that the user can use, Coder does not start the chat.
The user sees a message that asks an administrator to configure a chat model.

An organization with no MCP servers produces no error.
Chats in that organization offer no MCP servers.

## Who configures each scope

- Deployment administrators configure the providers and every setting in the **Deployment settings** section.
- Users with edit access to an organization's models configure that organization's models and admin model overrides.
- Users with read access to an organization's models can open the **Models** page and the **Coder Agents** page, and Coder shows the fields as read-only.
- Users with access to an organization's MCP servers can open the **MCP servers** page.
- Auditors and custom roles can hold read access without edit access.

The dashboard shows the organization picker when you can access more than 1 organization.
Your access applies to the selected organization, so your controls can differ between organizations.

## Manage model and MCP server access

Each chat model and each MCP server has an access list of groups and users.
An entry lets a member use the model or the server.
Coder adds the organization's **Everyone** group when you create a model or a server, so all members have access by default.
You can add only the members and the groups of the organization that owns the model or the server.

To change a model's access list, open **Model actions** > **Manage permissions** on the **Models** page.
Refer to [Manage model permissions](../models.md#manage-model-permissions) for the steps.

The **MCP servers** page has no access list editor.
Change an MCP server access list through the API instead:

- `GET /api/experimental/organizations/{organization}/mcp-servers/{mcpserverconfig}/acl`
- `PATCH /api/experimental/organizations/{organization}/mcp-servers/{mcpserverconfig}/acl`

## Related pages

- [Models](../models.md) for providers, models, and model overrides.
- [MCP Servers](./mcp-servers.md) for MCP server configuration.
- [Template Optimization](./template-optimization.md) for the per-template agent setting.
