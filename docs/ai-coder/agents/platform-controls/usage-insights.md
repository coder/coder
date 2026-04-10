# Spend Management

Coder provides admin-only controls for monitoring and controlling agent
spend: usage limits and cost tracking.

## Usage limits

Navigate to **Agents** > **Settings** > **Spend**.

Usage limits cap how much each user can spend on LLM usage within a rolling
time period. When enabled, the system checks the user's current spend before
processing each chat message.

### Configuration

- **Enable/disable toggle** — master on/off for the entire limit system.
- **Period** — `day`, `week`, or `month`. Periods are UTC-aligned: midnight
  UTC for daily, Monday start for weekly, first of the month for monthly.
- **Default limit** — deployment-wide default in dollars. Applies to all
  users who do not have a more specific override. Leave unset for no limit.
- **Per-user overrides** — set a custom dollar limit for an individual user.
  Takes highest priority.
- **Per-group overrides** — set a limit for a group. When a user belongs to
  multiple groups, the lowest group limit applies.

### Priority hierarchy

The system resolves a user's effective limit in this order:

1. Individual user override (highest priority)
1. Minimum group limit across all of the user's groups
1. Global default limit
1. No limit (if limits are disabled or no value is configured)

### Enforcement

- Checked before each chat message is processed.
- When current spend meets or exceeds the limit, the chat returns a
  **409 Conflict** response and the message is blocked.
- Fail-open: if the limit query itself fails, the message is allowed
  through.
- Brief overage is possible when concurrent messages are in flight, because
  cost is determined only after the LLM returns.

### User-facing status

Users can view their own spend status, including whether a limit is active,
their effective limit, current spend, and when the current period resets.

> [!NOTE]
> The admin configuration page shows the count of models without pricing
> data. Models missing pricing cannot be tracked accurately against limits.

## Cost tracking

Navigate to **Agents** > **Settings** > **Spend**.

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

- **Summary cards** — total cost, token breakdowns, and message counts.
- **Usage limit progress** — if a limit is active, a color-coded progress
  bar shows current spend relative to the limit.
- **Per-model breakdown** — table of costs and token usage by model.
- **Per-chat breakdown** — table of costs and token usage by chat session.

> [!NOTE]
> Automatic title generation uses lightweight models, such as Claude Haiku or GPT-4o
> Mini. Its token usage is not counted towards usage limits or shown in usage
> summaries.
