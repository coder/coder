---
title: Spend management
---

Coder controls agent spend with AI Gateway budgets, and surfaces the resulting spend to both admins and users.

## Budgets

Coder Agents spend is controlled by AI Gateway budgets, which cap all AI Gateway usage (including Coder Agents chats) per user over the budget period.

- **Group budgets**:
  set a budget for a group from the group's settings page.
  The deployment budget policy resolves which group budget applies when a user belongs to multiple budgeted groups, which defaults to the group with the highest budget.
- **Per-user overrides**:
  set a custom budget for an individual user, attributed to one of their groups.
  Per-user overrides take priority over group budgets.

For budget settings, limits, and how Coder picks a user's budget group, refer to [AI Governance cost control](../../ai-governance/ai-gateway/cost-controls.md).

> [!IMPORTANT]
> Budget controls in the Coder UI, the group budget endpoints (`/api/v2/groups/{group}/ai/budget`), and the AI spend status and reporting endpoints all require the AI Gateway entitlement.
> No experiment is needed.
>
> Coder v2.37 removed native chat usage limits and native cost tracking.
> Coder does not move the old limits, per-model prices, or cost history to AI Gateway.
> After the upgrade, the **Spend** page, the per-model price fields, and the old cost endpoints are not available.
> To keep the old data, [export it before you upgrade](#export-native-cost-data-before-you-upgrade).

The API reference documents how to [get](../../reference/api/enterprise.md#get-group-ai-budget), [upsert](../../reference/api/enterprise.md#upsert-group-ai-budget), and [delete](../../reference/api/enterprise.md#delete-group-ai-budget) a group budget.

### Enforcement

- The AI Gateway checks the user's current spend before forwarding each request.
  When spend meets or exceeds the budget, the request is rejected and the chat shows a terminal error explaining that the budget was exceeded.
- Brief overage is possible when concurrent requests are in flight, because cost is recorded after the large language model (LLM) responds.

### User-facing status

The usage indicator on the Agents page and the summary in the user menu both show the signed-in user's current AI spend, their budget, and the period reset date.
Both appear only when the deployment has the AI Gateway entitlement.

### Export native cost data before you upgrade

Do these steps on your current server, before you upgrade to Coder v2.37 or later.

1. Record the per-model prices in **Admin settings** > **AI** > **Models**.
1. Choose a start date and an end date that cover all the history you need.
   Use RFC 3339 UTC timestamps.

   If you do not set the dates, the endpoints return only the last 30&nbsp;days.
1. Get the first page of users:

   `GET /api/experimental/chats/cost/users?start_date=<start>&end_date=<end>&limit=100&offset=0`

1. If `offset + users.length` is less than `count`, get the next page.
   For each page, increase `offset` by 100.
1. For each `users[].user_id`, get the summary with the same dates:

   `GET /api/experimental/chats/cost/{user_id}/summary?start_date=<start>&end_date=<end>`

   Each summary has the user's totals and the `by_model` and `by_chat` breakdowns.

After you upgrade, set up AI Gateway budgets.
AI Gateway reports do not include the old cost history.

## Spend details

Spend is shown where it is actionable:

<!-- TODO(AIGOV-685): Restore once the User spend page moves out of experimental.
- **Admin settings** > **AI** > **User spend**: total and per-user AI Gateway spend for the selected organization and reporting period.
  If you can also view AI sessions, you can filter it by provider, model, and client.
-->
- **Agents page and user menu**:
  the signed-in user's spend against their budget, as described previously.
- **Group settings**:
  each member's spend against the group's budget, for admins who can manage the group.
- **Chat summary panel**:
  the cost of one chat tree, on a chat's Summary tab.
  A subagent reports the total for its whole tree, including the chat that started it.

For organization spend exports, refer to [AI Gateway spend export](../../ai-governance/ai-gateway/cost-controls.md#spend-export).

The AI Gateway [sessions views](../../ai-governance/ai-gateway/audit.md#navigate-the-ui) show per-request token usage, which is the input to those costs rather than the costs themselves.

AI Gateway data is subject to its own [retention period](../../ai-governance/ai-gateway/monitoring.md#data-retention), 60&nbsp;days by default, which is configured independently of chat retention.
Spend for requests older than that period is no longer reported, so a chat for which gateway records have been pruned reports no cost.
