# Spend Management

Coder provides admin-only controls for monitoring and controlling agent
spend: AI Gateway budgets and cost tracking.

## Budgets

Coder Agents spend is controlled by AI Gateway budgets, which cap all AI
Gateway usage (including Coder Agents chats) per user over the budget
period.

- **Group budgets**: set a budget for a group from the group's settings
  page. The deployment budget policy resolves which group budget applies
  when a user belongs to multiple budgeted groups.
- **Per-user overrides**: set a custom budget for an individual user,
  attributed to one of their groups. Takes priority over group budgets.

Budgets always reset monthly. A user who belongs to no budgeted group falls
back to the Everyone group, which has no budget unless one is set for it.
There is no deployment-wide budget amount.

> [!IMPORTANT]
> Budgets require the `ai-gateway-cost-control` experiment and a license
> that includes AI Gateway. Without the experiment the group budget controls
> are hidden and the budget endpoints respond that the experiment must be
> enabled, so no budget can be configured, and no spend is capped.

### Enforcement

- The AI Gateway checks the user's current spend before forwarding each
  request. When spend meets or exceeds the budget, the request is rejected
  and the chat shows a terminal error explaining that the budget was
  exceeded.
- Brief overage is possible when concurrent requests are in flight, because
  cost is recorded after the LLM responds.

### User-facing status

When a budget applies, users see their current AI spend, the budget, and
the period reset date in the usage indicator on the Agents page.

## Cost tracking

Navigate to **Agents** > **Settings** > **Manage Agents** > **Spend**.

This view shows deployment-wide LLM chat costs with per-user drill-down.

### Top-level view

A per-user rollup table with the following columns:

| Column             | Description                         |
|--------------------|-------------------------------------|
| Total cost         | Aggregate dollar spend for the user |
| Messages           | Number of chat messages sent        |
| Chats              | Number of distinct chat sessions    |
| Input tokens       | Total input tokens consumed         |
| Output tokens      | Total output tokens consumed        |
| Cache read tokens  | Tokens served from cache            |
| Cache write tokens | Tokens written to cache             |

The table supports date range filtering (default: last 30 days), search by
name or username, and pagination.

### Per-user detail view

Select a user to see:

- **Summary cards**: total cost, token breakdowns, and message counts.
- **Per-model breakdown**: table of costs and token usage by model.
- **Per-chat breakdown**: table of costs and token usage by chat session.

> [!NOTE]
> Automatic title generation uses lightweight models, such as Claude Haiku or
> GPT-4o Mini. Its token usage is not shown in usage summaries.
