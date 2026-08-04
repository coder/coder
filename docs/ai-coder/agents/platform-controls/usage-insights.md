# Spend Management

Coder provides usage reporting for Coder Agents and enforces spend through AI
Gateway budgets on release 2.36.

## Native chat usage limits

As of release 2.36, native chat usage limits are no longer configurable or
enforced. Coder does not check previously configured values when processing
chat messages. Values configured before upgrading remain stored, have no
effect, and are not migrated to AI Gateway budgets.

To limit spend, configure AI Gateway budgets. They are the only enforcement
mechanism.

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

> [!NOTE]
> Automatic title generation uses lightweight models. Its token usage is not
> included in usage reporting.
