---
title: Request organization
---

> [!NOTE]
> AI Gateway is part of [AI Governance](../index.md), which is included with a Premium license.

This page is for administrators who manage [AI Governance cost control](./cost-controls.md) in a deployment with more than one organization.
AI Gateway assigns every request to exactly one organization.
That organization decides which groups and budgets apply to the request, and which group its spend is recorded against.
For a user who belongs to one organization, every request takes that organization.
For a user who belongs to several, the budget that applies depends on how the request authenticates, such as which workspace's session token it uses.

Request organizations require Coder v2.40 or later on `coderd`, the process that runs the control plane, and on every [standalone AI Gateway](./standalone.md) replica.

## How AI Gateway chooses the organization

AI Gateway uses the first source that applies to the request:

| Order | Request comes from                                                   | Organization                                                                                                                      |
|-------|----------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------|
| 1     | A client that authenticates with a workspace session token           | The workspace's organization                                                                                                      |
| 2     | A [Coder Agents](../../coder-agents/index.md) chat                   | The chat's organization                                                                                                           |
| 3     | Any other client, such as one that uses a token you created yourself | The organization of the user's [effective group](./cost-controls.md#effective-group-resolution) across all of their organizations |

A workspace session token is the token a template injects with `data.coder_workspace_owner.me.session_token`, as the [client setup examples](./clients/index.md) do.
A token that you create with `coder tokens create` or in **Account settings** > **Tokens** isn't a workspace session token.
Neither is the token that `coder login token` prints.
Requests that use these tokens take the organization of the user's effective group, even when the client runs inside a workspace.

A chat request takes the chat's organization, even when the chat uses a workspace.
A Coder Agents chat can only use a workspace in its own organization, so the chat and its workspace always agree.

### Reliable sources and the fallback

Workspace session tokens and Coder Agents chats are reliable sources.
A workspace or chat belongs to one organization for its whole life, so every request it sends lands in the same organization.

The effective group is a fallback.
It always finds an organization, but that organization can change between two requests in the same session.
The organization changes when you change the user's group budgets, user override, or group memberships, because a different group can then become their effective group.

When no budget applies to the user in any organization, the fallback picks the `Everyone` group of the default organization.
If the user isn't a member of the default organization, it picks the organization they joined first.

To keep a user's AI Gateway spend in a predictable organization, have their clients authenticate with a workspace session token.

## Rejected requests

AI Gateway rejects a request with `403 Forbidden` and the message `unauthorized`, before sending it upstream, when:

- The request uses the session token of a workspace that was deleted.
- The user is no longer a member of the workspace's or chat's organization, or that organization was deleted.
- The user is no longer a member of any organization, so the fallback finds none.

## What the organization controls

### Effective group and budget

AI Gateway resolves the user's [effective group](./cost-controls.md#effective-group-resolution) inside the request's organization:

- Only the user's groups in that organization count, and the [budget policy](./cost-controls.md#deployment-settings) picks between them.
- A [user override](./cost-controls.md#user-override) applies only when its assigned group is in that organization.
  An override assigned to a group in another organization is ignored.
- When no budget applies in that organization, the user has unlimited spend there, and their spend is recorded against the organization's `Everyone` group.

For a request that uses the effective group fallback, AI Gateway resolves the effective group across all of the user's organizations first, and the request then takes that group's organization.

### Spend

Spend is recorded against the effective group, and a budget only counts spend recorded against its own group.
As a result, spend in one organization doesn't use up a budget in another.
Budget notifications follow the same rule: Coder sends them for the budget in the request's organization.

### Model prices

The request's organization doesn't change the price, because the custom prices that you set with [`coder exp ai-model-prices`](./cost-controls.md#configure-model-prices) apply in every organization.

## Users in several organizations

Budgets follow the request's organization, not every organization the user belongs to.
Take a user who belongs to two organizations:

- Organization A gives them a group budget of $100&nbsp;United States dollars (USD).
- Organization B gives them a group budget of $10&nbsp;USD.

| Request comes from                           | Request organization    | Budget that applies |
|----------------------------------------------|-------------------------|---------------------|
| A workspace in organization A                | A                       | $100&nbsp;USD       |
| A workspace in organization B                | B                       | $10&nbsp;USD        |
| A Coder Agents chat in organization B        | B                       | $10&nbsp;USD        |
| A token they created, used from their laptop | A, through the fallback | $100&nbsp;USD       |

The workspace rows assume the client authenticates with the workspace session token.
The fallback picks organization A because the default budget policy, `highest`, selects the larger budget.

If organization B had no budget at all, requests from B would have unlimited spend, even though the user has a budget in A.
To limit a user everywhere, give them a budget in every organization they send requests from.

When a user reaches their budget in one organization, requests from another organization where they're under budget, or have no budget, still go through.

## Where spend reports can differ

Some reports resolve the user's effective group across all of their organizations, rather than per organization:

- The avatar menu
- The usage indicator on the Coder Agents page
- The [Get user AI spend](../../reference/api/enterprise.md#get-user-ai-spend) API endpoint
- The budget and its source on a group's **Members** tab
- The combined limit on the **Groups** page
- The `coder_ai_gateway_cost_control_blocked_users` metric

For a user in several organizations, these reports can differ from the budget that applied to a request that used a workspace session token or came from a chat.

Spend amounts on the **Groups** page and the **Members** tab, and in the [spend export](./cost-controls.md#spend-export), are the spend recorded against each group, so they match where each request was charged.
To find where a request was charged, use the spend export, whose rows name the group that each request's spend was recorded against.

## Standalone gateways older than v2.40

A standalone AI Gateway replica older than Coder v2.40 can still connect to a newer `coderd`, as [Version compatibility](./standalone.md#version-compatibility) describes.
Until you upgrade it, requests through that replica behave as follows:

- The budget check runs across all of the user's organizations, as it did before v2.40.
- The request's organization isn't recorded, so its spend is attributed to the effective group resolved across all of the user's organizations.

The [rejected requests](#rejected-requests) still apply, because `coderd` chooses the organization before the replica checks the budget.

To apply budgets per organization, upgrade `coderd` first, then upgrade every standalone replica.

## Learn more

- [AI Governance cost control](./cost-controls.md)
- [Standalone AI Gateway](./standalone.md)
- [Organizations](../../admin/users/organizations.md)
