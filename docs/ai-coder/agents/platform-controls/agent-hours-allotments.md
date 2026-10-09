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
A deployment with a single organization can allot to groups without giving the organization an allotment.
Coder shows hour figures for groups only when their organization has an allotment and the license grants a fixed number of Agent Hours.

Coder rejects any change that would push a tier above 100%, including concurrent changes from several administrators.
The API returns `409 Conflict` and names the share that is still unallotted.

## Manage allotments

1. Go to **Admin settings** > **AI** > **Coder Agents** > **Agent Hours**.
1. To allot to an organization, select **Add allotment** under **Organization allotments**, choose the organization, and enter a percentage.
1. To allot to a group, choose the organization under **Group allotments**, select **Add allotment**, choose the group, and enter a percentage.

The gauge above each table shows how much of the pool is allotted.
Select **Edit** to change an allotment or **Remove** to delete it.
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
- **No Agent Hours**: when the license doesn't include Agent Hours, the **Agent Hours** page is hidden and the allotment API returns `403 Forbidden`.
- **Renewal or resizing**: allotments are stored as percentages and stay in place.
  Hour figures update to match the new license.

## API

The [API reference](../../../reference/api/enterprise.md#get-agent-hours-organization-allotments) documents the allotment endpoints.
Allotments use basis points in the `allotment_bps` field, where `10000` equals 100%.
