# AI Governance Cost Control

> [!NOTE]
> AI Gateway is part of [AI Governance](../ai-governance.md), which is
> included with a Premium license.

AI Governance Cost Control governs AI spend in two complementary ways:

- **Enforcement** stops a user's AI Gateway requests once their spend reaches
  their budget.
- **Reporting** shows what each user and group has spent in the current budget
  period.

AI Governance Cost Control requires:

- Coder v2.36 or later.
- AI Gateway [enabled and configured](./setup.md) with at least one provider.

> [!NOTE]
> AI Governance Cost Control reports estimated spend rather than billed cost.
> Estimates will not match your provider invoices exactly. For details, see
> [How spend is estimated](#how-spend-is-estimated).

These terms appear throughout this page and in the dashboard:

| Term                | What it means                                                                     | Where it is set                      |
|---------------------|-----------------------------------------------------------------------------------|--------------------------------------|
| **Budget period**   | The window spend accumulates in before it resets. Defaults to the calendar month. | Deployment settings                  |
| **Budget policy**   | The rule that picks a user's effective group. Defaults to the highest budget.     | Deployment settings                  |
| **Group budget**    | A spend limit granted to each member of a group.                                  | **Groups** > group > **Settings**    |
| **User override**   | A spend limit for one user that supersedes their group budget.                    | **Groups** > group > **Members** tab |
| **Effective group** | The group that supplies a user's budget and records their spend.                  | Resolved automatically               |
| **Estimated spend** | A user's approximate spend in the current budget period.                          | Calculated by Coder                  |

## Deployment settings

Two deployment-wide settings govern budget resolution and the reset window. Each
accepts a single value today.

| Setting       | Flag                 | Environment variable     | Supported values | Default   |
|---------------|----------------------|--------------------------|------------------|-----------|
| Budget policy | `--ai-budget-policy` | `CODER_AI_BUDGET_POLICY` | `highest`        | `highest` |
| Budget period | `--ai-budget-period` | `CODER_AI_BUDGET_PERIOD` | `month`          | `month`   |

- **Budget policy** determines which budget wins when a user belongs to more
  than one budgeted group. `highest` selects the largest.
- **Budget period** sets the reset window. `month` is the UTC calendar month, so
  spend resets at 00:00 UTC on the first day of each month.

Neither setting can be scoped to an organization or a group.

## Budgets

**Every user's spend is recorded against a group.** A user only has a budget
when one is set on a group they belong to, or when they are given a personal
override.

> [!NOTE]
> No budgets exist when AI Governance Cost Control is first deployed. Until an
> administrator sets them, users have **unlimited spend** and their spend is
> recorded against the **Everyone** group.

### Group budget

Setting a group budget requires the Owner, User Admin, Organization Admin, or
Organization User Admin [role](../../admin/users/groups-roles.md).

A group budget applies to each member individually rather than to the group as a
whole. A budget of $200 gives every member $200 to spend.

1. Go to **Groups** and select a group.
1. Select **Settings**.
1. Under **AI budget**, set **Monthly limit per member**.
1. Select **Save**.

Budget values behave as follows:

- An empty field means no limit. The field displays `unlimited`.
- `$0` blocks every AI Gateway request from members whose effective group is
  this group.
- The maximum is `$1,000,000` per member per budget period.

### User override

Adding or removing an override requires the Owner or User Admin
[role](../../admin/users/groups-roles.md). Organization administrators can view
overrides but cannot change them.

Override a group budget when one person needs a different limit from the rest of
their group.

1. Go to **Groups**, select a group, then open the **Members** tab.
1. Open the member's action menu and select **Manage AI budget**.
1. Enable **Override group budget**.
1. Set **Custom monthly budget**, then choose the group in
   **Budget assigned to**.
1. Select **Update**.

Overrides behave as follows:

- An override supersedes every group budget the user belongs to.
- The user must belong to the group selected in **Budget assigned to**.
- `$0` and the `$1,000,000` maximum apply to overrides as well.
- Disabling **Override group budget** removes the override and returns the user
  to the budget of their [effective group](#effective-group-resolution).

### Effective group resolution

A user can belong to several groups that have budgets, and can also hold an
override. AI Gateway resolves a single effective group for each request, in this
order:

1. The user's [override](#user-override) takes precedence over every group
   budget.
1. Otherwise, the [budget policy](#deployment-settings) selects one of the
   groups the user belongs to. The default policy, `highest`, selects the group
   with the largest budget.
1. If none of those groups has a budget, the user has unlimited spend and their
   spend is recorded against the **Everyone** group.

> [!NOTE]
> Groups with identical budgets are ranked by the organization membership the
> user joined first, then by group ID.

Recorded spend is immutable. Changing which budget applies to a user affects
future requests only: spend that Coder has already recorded stays with the group
it was attributed to, so a user's history can span several groups.

## How spend is estimated

Coder multiplies the token usage of each request by the published price of the
model that served it. Prices come from a curated [models.dev](https://models.dev)
snapshot that ships with every Coder release, so no configuration is required.

To see which models a release prices, consult the
[price book](https://github.com/coder/coder/blob/main/coderd/aibridge/prices/data/prices.json)
in the Coder repository.

Estimates exclude negotiated discounts, committed-use pricing, and provider
billing adjustments, so they can differ from the amounts your providers report.

> [!IMPORTANT]
> Requests to models that are missing from the price table record token usage
> but add nothing to a user's spend. A user who only calls unpriced models is
> effectively unlimited.

Monitor `coder_ai_gateway_cost_control_unpriced_token_usage_records_total`,
labeled by `provider` and `model`, to detect unpriced usage. Any non-zero value
means spend is under-counted. Because the price book ships with the release, a
newly launched model can remain unpriced until you upgrade Coder.

Spend accumulates only from the moment v2.36 is deployed. Upgrading mid-month
therefore produces a partial first period.

## How enforcement works

AI Gateway checks each request before forwarding it, comparing the user's
recorded spend for the current budget period against their budget:

- Spend below the budget: the request proceeds.
- Spend at or above the budget: AI Gateway returns `403 Forbidden` and directs
  the user to an administrator.
- Budget check fails: the request returns a `500` error rather than bypassing
  the limit.

Access resumes when the budget period resets, when an administrator raises the
budget, or when an override is added.

> [!NOTE]
> Enforcement is approximate. A request's cost is known only after the provider
> responds, so requests in flight at the same time can carry a user slightly
> past their budget.

### Notifications

The first time a user's spend crosses a threshold within a period, Coder
notifies the user and the administrators responsible for them:

| Threshold | The user receives              | Owners and user admins receive           |
|-----------|--------------------------------|------------------------------------------|
| 85%       | You're approaching your budget | `<username>` is approaching their budget |
| 100%      | You've reached your budget     | `<username>` has reached their budget    |

- A single expensive request can cross both thresholds at once.
- Budgets of `$0` and unpriced usage cross no thresholds.
- Notifications are informational. Enforcement does not depend on them.

For delivery methods, see
[Notifications](../../admin/monitoring/notifications/index.md).

## Monitor spend

Spend reporting is available in the Coder dashboard, as a CSV export, and
through Prometheus.

### In the Coder dashboard

Visibility follows the viewer's role:

| Who                                                  | Sees                                                   |
|------------------------------------------------------|--------------------------------------------------------|
| Every user                                           | Their own spend and budget in their avatar menu        |
| Members of a group                                   | The group's spend and budget, and their own member row |
| Owners, User Admins, and organization administrators | Spend and budgets for every group and every member     |

- The **Groups** page compares each group's spend with the combined limits of
  the members it covers.
- The **Members** tab of a group reports each member's spend, their budget, and
  its source, labeled `Custom limit` for an override or `Group limit` for a
  group budget.
- The avatar menu reports the signed-in user's own spend for the budget period
  as `$<spend> / $<budget> USD`.

The info icons beside these figures explain the detail behind them, such as when
the period resets or which group manages a user's budget.

### As a CSV export

Organization administrators can export spend for reporting and chargeback. The
export is available through the API only.

```sh
curl -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  "https://coder.example.com/api/v2/organizations/<organization>/ai/spend/export"
```

- Without parameters, the export covers the current budget period.
- To select a range, pass `period_start` and `period_end` together as RFC 3339
  timestamps. A range can span at most 31 days.
- Each row breaks spend down by user, group, model, and provider, with the
  underlying token counts.

### With Prometheus

These metrics report AI Governance Cost Control activity. For the full list, see
[Prometheus metrics](../../admin/integrations/prometheus.md).

| Metric                                                             | Purpose                                         |
|--------------------------------------------------------------------|-------------------------------------------------|
| `coder_ai_gateway_cost_control_blocked_requests_total`             | Requests blocked because a budget was exceeded. |
| `coder_ai_gateway_cost_control_blocked_users`                      | Users currently over their budget.              |
| `coder_ai_gateway_cost_control_unpriced_token_usage_records_total` | Usage records with no known model price.        |
| `coder_ai_gateway_cost_control_enforcement_duration_seconds`       | Duration of budget checks.                      |

Alert on the unpriced metric. It is the only signal that spend is being
under-counted.

## Migrate from Coder Agents Cost Control

AI Governance Cost Control replaces Coder Agents Cost Control in v2.36.

> [!WARNING]
> Spend limits configured under **Admin settings** > **AI** > **Spend** are no
> longer enforced. Affected users have no limit until an AI budget is set.

Existing limits are not converted. Recreate them:

1. Record the limits currently set under **Admin settings** > **AI** >
   **Spend**, including the default limit and any group or user overrides.
1. Recreate group limits as [group budgets](#group-budget).
1. Recreate per-user limits as [user overrides](#user-override).

Expect the following differences:

- No deployment-wide default exists. Each group that needs a limit requires its
  own budget.
- The UTC calendar month is the only period. Daily and weekly periods are gone.
- Users in several budgeted groups receive the highest budget. Coder Agents Cost
  Control applied the lowest.
- Budgets cover all AI Gateway traffic. Chat, IDE extensions, and CLI agents
  draw on the same budget.
- Recorded spend does not carry over. Every user starts the first period at $0.
- Coder Agents users who exceed their budget see a usage limit error in chat.

## Next steps

- [Monitoring](./monitoring.md)
- [Auditing AI sessions](./audit.md)
- [AI Gateway API reference](../../reference/api/enterprise.md)
