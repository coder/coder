# Spend Management

Coder provides usage reporting for Coder Agents and two independent ways to
limit spend on release 2.36: existing native chat usage limits and AI Gateway
budgets.

## Native chat usage limits

The native usage-limit configuration UI has been removed from release 2.36.
Values configured before upgrading remain stored and enforced. Coder checks the
user's current spend before processing each chat message and returns a **409
Conflict** response when the applicable limit is reached.

Existing native values are not migrated to AI Gateway budgets. To change spend
controls after upgrading, configure new AI Gateway budgets. Native limits remain
in effect until they are changed through the existing experimental API or
removed in a later release.

## AI Gateway budgets

AI Gateway budgets cap each user's AI Gateway spend, including Coder Agents
chats, over a monthly period.

- Set a group budget from the group's settings page.
- When a user belongs to several budgeted groups, the deployment budget policy
  selects the applicable group. The default policy selects the highest budget.
- A per-user override takes priority over group budgets.

Budget controls require a license that includes AI Gateway. Existing native
usage-limit values and recorded spend are not migrated when you configure a
budget.

The API reference documents how to
[get](../../../reference/api/enterprise.md#get-group-ai-budget),
[upsert](../../../reference/api/enterprise.md#upsert-group-ai-budget), and
[delete](../../../reference/api/enterprise.md#delete-group-ai-budget) a group
budget.

The Agents page shows the signed-in user's current AI Gateway spend and budget
when AI Gateway is available.

## Spend visibility

Navigate to **Agents** > **Settings** > **Manage Agents** > **Spend** to view
usage-only reporting for deployment-wide Coder Agents chat costs.

The top-level table includes total cost, message and chat counts, and token usage
for each user. It supports date range filtering, search, and pagination.

Select a user to view summary cards and per-model and per-chat breakdowns. The
Spend page does not configure native usage limits or AI Gateway budgets.
