---
title: Agent Hours allotments
---

Agent Hours allotments let you plan how your deployment's licensed Agent Hours are divided between organizations and the groups inside them.

> [!IMPORTANT]
> Coder doesn't enforce allotments yet.
> Allotments are configuration only: they don't change how Coder measures or reports Agent Time, when license warnings appear, or when agents can run.
> Every agent keeps drawing from the deployment's shared pool, as described in [Licensing & usage](../licensing-usage.md).

Allotments require a Premium license that includes Agent Hours.

## How allotments work

Each allotment is a percentage of a pool, with up to two decimal places.
Percentages keep their meaning when a license is renewed or resized.
When the license grants a fixed number of Agent Hours, Coder also shows the hours each percentage represents.

Allotments come in two tiers, and each tier divides its own pool:

| Tier         | Pool                                  | Sum limit                             |
|--------------|---------------------------------------|---------------------------------------|
| Organization | The deployment's licensed Agent Hours | 100% across all organizations         |
| Group        | The group's organization's share      | 100% across the organization's groups |

The tiers are counted separately.
Changing or removing an organization's allotment doesn't change the allotments of its groups.

Agent Hours that aren't allotted stay in a shared pool.
An organization without an allotment draws from that shared pool, and its groups can still have allotments.
The **Everyone** group can't have an allotment, because members without an allotted group already draw from their organization's unallotted share.
A deployment with a single organization can allot to groups without giving the organization an allotment.
Coder shows hour figures for groups only when their organization has an allotment and the license grants a fixed number of Agent Hours.

Coder rejects any change that would push a tier above 100%, including concurrent changes from several administrators.
The API returns `409 Conflict` with the share that is still unallotted and the largest value `allotment_bps` can take.

## How usage is counted

Coder tracks how many Agent Hours each organization, group, and user used in the current license usage period.
This breakdown stays in your deployment: Coder still reports only the hourly deployment total, as described in [Licensing & usage](../licensing-usage.md#agent-time-usage-reporting).

- Agent Time counts toward the chat's organization and the user who owns the chat.
  Subagents count toward the chat that started them.
- In each organization, a user's Agent Time counts toward one group: the user's group in that organization with the largest allotment.
  When several of the user's groups share the largest allotment, Coder picks the one with the lowest group ID, so the choice stays stable.
- Agent Time of users who aren't in an allotted group counts toward the **Everyone** group, which stands for the organization's unallotted share.
- Coder attributes each UTC hour shortly after it ends, using the group memberships and allotments at that time.
  Later changes to memberships or allotments apply only to later hours.

Usage updates once an hour, so the current hour isn't included yet.
An hour counts toward the license usage period that contains its start, as it does for the license total.

When you upgrade to a Coder version that tracks usage per organization, Coder attributes the Agent Time already used in each hour to the Everyone group of the chat's organization.
Agent Time from chats that were deleted before the upgrade can't be attributed, so organization usage can add up to less than the license total.

## Manage allotments

1. Go to **Admin settings** > **AI** > **Coder Agents** > **Agent Hours**.
1. To allot to an organization, select **Add allotment** under **Organization allotments**, choose the organization, and enter a percentage.
1. To allot to a group, choose the organization under **Group allotments**, select **Add allotment**, choose the group, and enter a percentage.

The gauge above each table shows how much of the pool is allotted.
Use the edit button in a row to change an allotment, or the remove button to delete it after you confirm.
Coder records every change in the [audit log](../../../admin/security/audit-logs.md).

## Permissions

| Action                                       | Who can perform it                                                                                     |
|----------------------------------------------|--------------------------------------------------------------------------------------------------------|
| Set or remove organization allotments        | Owners                                                                                                 |
| List all organization allotments             | Owners and auditors                                                                                    |
| Set or remove group allotments               | Users who can update the group: owners, user admins, organization admins, and organization user admins |
| List the group allotments of an organization | Users who can read groups in the organization                                                          |

Organization admins can't change their own organization's allotment.
They can read it, because it's the base of their group allotments.

## License changes

- **Unlimited Agent Hours**: you can still set allotments, and each tier is still capped at 100%.
  Coder shows percentages only.
- **No Agent Hours**: when the license doesn't include Agent Hours, the **Agent Hours** page is hidden from navigation.
  If you could manage allotments under a license with Agent Hours, opening the page directly shows a notice that the license doesn't include Agent Hours.
  Other users see a permission error instead.
  The allotment API returns `403 Forbidden`.
- **Renewal or resizing**: allotments are stored as percentages and stay in place.
  Hour figures update to match the new license.

## API

The [API reference](../../../reference/api/enterprise.md#get-agent-hours-organization-allotments) documents the allotment endpoints.
Allotments use basis points in the `allotment_bps` field, where `10000` equals 100%.

The [usage endpoints](../../../reference/api/enterprise.md#get-agent-hours-usage) report usage in milliseconds, for the deployment's organizations, an organization's groups, and a group's members.
They use the same permissions as the matching allotment endpoints: listing usage for all organizations requires the access needed to list all organization allotments, and an organization's group usage requires permission to read its groups.
Group member usage returns only the members you can read.
