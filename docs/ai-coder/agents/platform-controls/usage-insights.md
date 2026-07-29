# Spend Management

Coder controls agent spend with AI Gateway budgets, and surfaces the
resulting spend to both admins and users.

## Budgets

Coder Agents spend is controlled by AI Gateway budgets, which cap all AI
Gateway usage (including Coder Agents chats) per user over the budget
period.

- **Group budgets**: set a budget for a group from the group's settings
  page. The deployment budget policy resolves which group budget applies
  when a user belongs to multiple budgeted groups.
- **Per-user overrides**: set a custom budget for an individual user,
  attributed to one of their groups. Takes priority over group budgets.

The deployment flags `--ai-budget-policy` and `--ai-budget-period` currently
support only `highest` and `month`. `highest` selects the group with the
largest spend limit, and `month` resets spend at the start of each UTC
calendar month. A user who belongs to no budgeted group falls back to the
Everyone group, which has no budget unless one is set for it. There is no
deployment-wide budget amount, and a configured spend limit cannot exceed
$1,000,000 per member per period.

> [!IMPORTANT]
> Budget controls in the Coder UI, the group budget endpoints
> (`/api/v2/groups/{group}/ai/budget`), and the AI spend status and reporting
> endpoints all require the AI Gateway entitlement. No experiment is needed.
>
> Native chat usage limits are removed from the application. Existing native
> limit values are not migrated to AI Gateway budgets and are no longer
> enforced. Configure AI Gateway budgets separately.

### Enforcement

- The AI Gateway checks the user's current spend before forwarding each
  request. When spend meets or exceeds the budget, the request is rejected
  and the chat shows a terminal error explaining that the budget was
  exceeded.
- Brief overage is possible when concurrent requests are in flight, because
  cost is recorded after the LLM responds.

### User-facing status

The usage indicator on the Agents page and the summary in the user menu both
show the signed-in user's current AI spend, their budget, and the period
reset date. Both appear only when the deployment has the AI Gateway
entitlement.

## Spend visibility

Spend is shown where it is actionable:

- **Agents page and user menu**: the signed-in user's spend against their
  budget, as described above.
- **Group settings**: each member's spend against the group's budget, for
  admins who can manage the group.
- **Chat summary panel**: the cost of one chat tree, on a chat's Summary tab.
  A subagent reports the total for its whole tree, including the chat that
  started it.
- **Agents** > **Settings** > **Manage Agents** > **Spend**: deployment-wide
  chat cost per user, with per-user drill-down.

> [!NOTE]
> Per-chat cost comes from AI Gateway records, which are pruned according to
> `--ai-gateway-retention` (60 days by default). A chat whose gateway records
> have been pruned reports no cost.
