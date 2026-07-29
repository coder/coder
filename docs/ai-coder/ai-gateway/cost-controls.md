# Cost Controls

> [!NOTE]
> AI Gateway is part of [AI Governance](../ai-governance.md), which is
> included with a Premium license.

Cost controls cap how much each user can spend on AI models through AI Gateway.
You set a budget for a group, and AI Gateway blocks a user's requests once their
estimated spend reaches that budget.

Cost controls are an estimate-and-block mechanism, not a billing system. Use
them to stop runaway usage and to see which teams and users drive spend. Do not
use them to reconcile your provider invoices.

Cost controls require:

- Coder v2.36 or later.
- AI Gateway [enabled and configured](./setup.md) with at least one provider.
- The `ai-gateway-cost-control`
  [experiment](../../install/releases/feature-stages.md#early-access-features),
  which enables spend views and user budget overrides:

  ```sh
  coder server --experiments=ai-gateway-cost-control
  # or
  CODER_EXPERIMENTS=ai-gateway-cost-control
  ```

## How cost controls work

| Term                 | What it means                                                                   | Where you set it                     |
|----------------------|---------------------------------------------------------------------------------|--------------------------------------|
| **Budget period**    | The window that spend accumulates in. Spend resets at the start of each period. | Deployment settings                  |
| **Group budget**     | A spend limit that applies to each member of a group, individually.             | **Groups** > group > **Settings**    |
| **User override**    | A spend limit for one user that replaces their group budget.                    | **Groups** > group > **Members** tab |
| **Effective budget** | The single limit that AI Gateway enforces for a user.                           | Resolved automatically               |
| **Budget group**     | The group that a user's spend is recorded against.                              | Resolved automatically               |
| **Estimated spend**  | The approximate cost of a user's token usage in the current period.             | Calculated by Coder                  |

### How the effective budget is resolved

For each request, AI Gateway resolves one budget for the user:

1. If the user has an override, Coder uses the override.
1. Otherwise, Coder uses the highest budget among the user's groups.
1. If none of the user's groups has a budget, the user is unlimited.

> [!NOTE]
> When two groups have the same budget, Coder picks the group the user joined
> the organization through first, and breaks any remaining tie by group ID.
> Users with no budget have their spend recorded against the **Everyone** group
> so that you can still track it.

Reassigning a user to a different budget does not move spend that Coder already
recorded, so a user's spend history can span more than one group.

## Configure deployment settings

Two deployment-wide settings control how budgets are resolved and when spend
resets. Both have a single supported value today.

| Setting       | Flag                 | Environment variable     | YAML key        | Values    | Default   |
|---------------|----------------------|--------------------------|-----------------|-----------|-----------|
| Budget policy | `--ai-budget-policy` | `CODER_AI_BUDGET_POLICY` | `budget_policy` | `highest` | `highest` |
| Budget period | `--ai-budget-period` | `CODER_AI_BUDGET_PERIOD` | `budget_period` | `month`   | `month`   |

- **Budget policy** decides which budget applies when a user belongs to several
  groups that have budgets. `highest` selects the largest budget.
- **Budget period** sets the reset window. `month` is the UTC calendar month, so
  spend resets at 00:00 UTC on the first day of each month.

You cannot set either value per organization or per group.

## Set a group budget

You must have the Owner, User Admin, Organization Admin, or Organization User
Admin [role](../../admin/users/groups-roles.md) to set a group budget.

A group budget is a limit for each member of the group. It is not a shared pool.
A budget of $200 in a group of ten members allows up to $200 of spend per
member.

1. Go to **Groups** and select a group.
1. Select **Settings**.
1. Under **AI budget**, set **Monthly limit per member**.
1. Select **Save**.

Keep the following in mind:

- Leave the field empty for an unlimited budget. The field shows `unlimited`
  when no budget is set.
- A budget of `$0` blocks all AI Gateway requests from members whose effective
  budget resolves to this group.
- The maximum budget is `$1,000,000` per member per period.

## Override a budget for a user

You must have the Owner or User Admin
[role](../../admin/users/groups-roles.md) to add or remove a user override.
Organization admins can view overrides but not change them.

Use an override when one person needs a different limit from the rest of their
group, such as a heavy user during a migration.

1. Go to **Groups**, select a group, then open the **Members** tab.
1. Open the member's action menu and select **Manage AI budget**.
1. Enable **Override group budget**.
1. Set **Custom monthly budget**, then choose the group in
   **Budget assigned to**.
1. Select **Update**.

Keep the following in mind:

- An override always wins over every group budget the user has.
- The user must belong to the group you select in **Budget assigned to**.
- The same limits apply: `$0` blocks all AI Gateway requests, and the maximum is
  `$1,000,000` per member per period.
- To remove an override, disable **Override group budget** and select
  **Update**. The user returns to their highest group budget.

## How spend is estimated

Coder estimates spend by multiplying each request's token usage by the model's
published price. Prices ship with each Coder release and come from a curated
[models.dev](https://models.dev) snapshot, so you do not configure them.

> [!WARNING]
> Estimated spend is not a bill.
>
> - It does not match your provider invoice. It ignores discounts,
>   committed-use pricing, and provider-side billing rules. Reconcile actual
>   costs with your provider.
> - Requests to models that are missing from the price table record token usage
>   but add nothing to a user's spend. A user who only calls unpriced models is
>   effectively unlimited.

To find unpriced usage, monitor
`coder_ai_gateway_cost_control_unpriced_token_usage_records_total`, which is
labeled by `provider` and `model`. A non-zero value means spend is
under-counted. Because prices ship with each release, a newly released model can
stay unpriced until you upgrade Coder.

Spend also starts accumulating only when you deploy v2.36. If you upgrade in the
middle of a month, the first period covers a partial month.

## How enforcement works

Before AI Gateway forwards a request, it compares the user's recorded spend for
the current period with their effective budget:

- If spend is below the budget, the request proceeds.
- If spend has reached or exceeded the budget, AI Gateway rejects the request
  with `403 Forbidden` and asks the user to contact an administrator.
- If Coder cannot check the budget, the request fails with a `500` error rather
  than bypassing the limit.

A blocked user regains access when the period resets or when you raise their
budget or add an override.

> [!NOTE]
> Enforcement is approximate. Coder only knows a request's cost after the
> provider responds, so requests that run at the same time can push a user
> slightly over their budget.

### Notifications

Coder sends notifications the first time a user's spend crosses a threshold in a
period:

| Threshold | The user receives              | Owners and user admins receive           |
|-----------|--------------------------------|------------------------------------------|
| 85%       | You're approaching your budget | `<username>` is approaching their budget |
| 100%      | You've reached your budget     | `<username>` has reached their budget    |

- A single expensive request can cross both thresholds at once.
- Budgets of `$0` and unpriced usage do not trigger notifications.
- Notifications are informational. Enforcement does not depend on them.

To configure delivery, see
[Notifications](../../admin/monitoring/notifications/index.md).

## Monitor spend

### In the dashboard

What each person sees depends on their role:

| Who                                                  | Can see                                                |
|------------------------------------------------------|--------------------------------------------------------|
| Every user                                           | Their own spend and budget in their avatar menu        |
| Members of a group                                   | The group's spend and budget, and their own member row |
| Owners, User Admins, and organization administrators | Spend and budgets for every group and every member     |

- **Groups** lists each group's spend against its budget. The AI budget column
  totals the limits of the members that the group is responsible for.
- A group's **Members** tab shows each member's spend, their effective budget,
  and which budget applies, labeled `Custom limit` for an override or
  `Group limit` for a group budget.
- Each user sees their own spend for the period under their avatar menu, shown
  as `$<spend> / $<budget> USD`.

Hover the info icons next to these values for details, such as when the period
resets or when another group manages a user's budget.

### As a CSV export

Organization admins can export spend for reporting or chargeback. The export is
available through the API only:

```sh
curl -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  "https://coder.example.com/api/v2/organizations/<organization>/ai/spend/export"
```

- The export defaults to the current period. To choose a range, pass both
  `period_start` and `period_end` as RFC 3339 timestamps.
- A custom range can span at most 31 days.
- Rows break spend down by user, group, model, and provider, and include token
  counts.

### With Prometheus

The following metrics report cost control activity. See
[Prometheus metrics](../../admin/integrations/prometheus.md) for the full list.

| Metric                                                             | Purpose                                         |
|--------------------------------------------------------------------|-------------------------------------------------|
| `coder_ai_gateway_cost_control_blocked_requests_total`             | Requests blocked because a budget was exceeded. |
| `coder_ai_gateway_cost_control_blocked_users`                      | Users currently over their budget.              |
| `coder_ai_gateway_cost_control_unpriced_token_usage_records_total` | Usage records with no known model price.        |
| `coder_ai_gateway_cost_control_enforcement_duration_seconds`       | Duration of budget checks.                      |

Alert on the unpriced metric. It is the only signal that spend is being
under-counted.

## Migrate from Coder Agents cost control

In v2.36, AI Governance cost controls fully replace Coder Agents cost control.

> [!WARNING]
> Spend limits configured under **Admin settings** > **AI** > **Spend** are no
> longer enforced. Until you set an AI budget, affected users have no limit.

Coder does not convert existing limits, so recreate them yourself:

1. Note the limits currently set under **Admin settings** > **AI** > **Spend**,
   including the default limit and any group or user overrides.
1. Recreate group limits as [group budgets](#set-a-group-budget).
1. Recreate per-user limits as [user overrides](#override-a-budget-for-a-user).

Plan for these differences:

- There is no deployment-wide default budget. Set a budget on each group that
  needs one.
- The only period is the UTC calendar month. Daily and weekly periods are gone.
- When a user belongs to several budgeted groups, the **highest** budget applies.
  Coder Agents cost control applied the lowest limit.
- Budgets cover all AI Gateway traffic, not only Coder Agents. Chat, IDE
  extensions, and CLI agents count against the same budget.
- Recorded spend does not carry over. Every user starts the first period at $0.
- Coder Agents users who exceed their budget see a usage limit error in chat.

## Next steps

- [Monitoring](./monitoring.md)
- [Auditing AI sessions](./audit.md)
- [AI Gateway API reference](../../reference/api/enterprise.md)
