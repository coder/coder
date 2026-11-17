---
title: Cut off access to workspaces during a security incident
---

This guide is for Coder deployment administrators who need to stop users from reaching workspaces right away, for example after a data breach or a leaked credential.
It covers what each action blocks, the order to apply them in, and how to restore access afterward.
For outages and data loss rather than security incidents, refer to [Disaster recovery](../../install/operate/disaster-recovery.md).

> [!IMPORTANT]
> You need the Owner role to complete every step on this page.

## Situations that call for a cutoff

Most incidents that need an immediate cutoff start with a deployment that drifted from the [security best practices](./security-best-practices.md).
Common examples:

| Situation                                                                   | Practice that prevents it                                                                                                 |
|-----------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------|
| A long-lived API token leaks through a script, a CI log, or a public repo   | Set a short [`--max-token-lifetime`](../../reference/cli/server/index.md#--max-token-lifetime) and use per-purpose tokens |
| A user account is compromised and the attacker reaches the user's workspace | Require single sign-on and remove access in your identity provider (IdP) when people leave                                |
| Credentials are hard-coded in a template or a workspace image               | Deliver credentials with [secrets](./secrets.md) or external authentication instead                                       |
| A workspace port is shared publicly and exposes internal data               | Limit the [maximum port sharing level](../networking/port-forwarding.md#configure-maximum-port-sharing-level)             |
| A workspace has open outbound access to sensitive networks                  | Restrict [outbound network access](./security-best-practices.md#outbound-network-access) from workspaces                  |

## What each action blocks

Coder actions differ in whether they end sessions that are already open.
Pick the actions that match the scope of the incident.

| Action                        | Blocks new access              | Ends open sessions                                 |
|-------------------------------|--------------------------------|----------------------------------------------------|
| Suspend a user                | Yes, for that user             | No                                                 |
| Expire or delete an API token | Yes, for that token            | No                                                 |
| Stop workspaces               | Until a user starts them again | Yes, when the template removes the compute on stop |
| Stop the control plane        | Yes                            | No, for direct connections                         |
| Block traffic at the network  | Yes                            | Yes                                                |

Stopping the control plane alone isn't a reliable cutoff.
A direct connection runs on a WireGuard tunnel between the client and the workspace agent, and the tunnel doesn't pass through the control plane.
An active direct session, such as `coder ssh`, can keep working after the control plane stops.
Stopping the control plane also stops the dashboard and API that you use for the other steps.

## Cut off access

Work through these steps in order.
Skip a step when the incident doesn't need it.

### 1. Suspend the affected users

A suspended user can't sign in, and Coder rejects every request made with that user's API tokens.
The workspace agents in that user's workspaces also can't authenticate to the control plane.

1. Go to **Users**.
1. Select the vertical ellipsis next to the user, then select **Suspend**.
1. In the confirmation dialog, select **Suspend**.

From the CLI, run this command for each user:

```sh
coder users suspend <username>
```

Coder doesn't let you suspend yourself or a user with the Owner role.
To suspend an owner, remove the Owner role first.

If the attacker holds credentials for your IdP, also turn off the account in the IdP so the user can't sign in again after you reactivate them in Coder.

### 2. Revoke leaked tokens

When a token leaked but its user stays active, expire the token instead of suspending the user.

1. List every token in the deployment:

   ```sh
   coder tokens list --all
   ```

1. Expire each leaked token with the [expire API key endpoint](../../reference/api/users.md#expire-api-key), `PUT /api/v2/users/{user}/keys/{keyid}/expire`.

   Expiring keeps the token in the audit trail.
   `coder tokens remove` acts only on your own tokens.

### 3. Stop the workspaces

Stopping a workspace runs the template's stop transition.
In templates that follow the [resource persistence](../../templates/extending-templates/resource-persistence.md) pattern, stop removes the compute and keeps the persistent storage.
When the compute is gone, every open session to it ends, including direct ones.
A user who isn't suspended can start their workspace again, so suspend users first when the incident covers them.

To stop every running workspace from the dashboard:

1. Go to **Workspaces**.
1. Filter the list with `status:running`.
1. Select the checkbox in the table header to select every workspace on the page.
1. Select **Actions** > **Stop**, then confirm.
1. Repeat for each page of results.

To stop every running workspace from the CLI, run this command in a Bash or Zsh shell with [jq](https://jqlang.org/) installed:

```sh
coder list --search "status:running" --output json \
  | jq -r '.[] | "\(.owner_name)/\(.name)"' \
  | xargs -n 1 -P 10 coder stop --yes
```

A search query replaces the default `owner:me` filter, so an owner sees running workspaces for every user.
`coder stop` waits for its build to finish, so `-P 10` runs 10 stops at a time.

Check templates whose stop transition leaves compute running, such as templates that don't use `start_count`.
Remove that compute on its platform, or block its traffic in the next step.

### 4. Block traffic at the network

When you can't stop every workspace, or you need to cut off access before the stops finish, block traffic outside Coder.
Use the controls of your platform, such as cloud security groups or Kubernetes NetworkPolicies:

- Block inbound traffic to the control plane from users, at the load balancer or ingress.
- Block outbound traffic from workspaces to everything except what the investigation needs.

Blocking user traffic to the control plane stops sessions that use the built-in relay.
If you use custom relays, block user traffic to them too.
Blocking workspace outbound traffic stops direct sessions and data exfiltration from the workspaces.

## Investigate and restore access

1. Review the [audit logs](./audit-logs.md) for actions taken by the affected users and tokens.
1. Review the [connection logs](../monitoring/connection-logs.md) for workspace sessions during the incident.
1. Rotate any credential that the incident exposed, such as external authentication tokens, IdP client secrets, and template secrets.
1. If you use database encryption, [rotate the encryption keys](./database-encryption.md#rotate-keys) when the keys might be exposed.
1. Fix the practice that allowed the incident, from [Situations that call for a cutoff](#situations-that-call-for-a-cutoff).
1. Remove the network blocks, then [activate](../users/index.md#activate-a-suspended-user) the users that you suspended.

Audit logs and connection logs require a [Premium license](https://coder.com/pricing#compare-plans).

## Learn more

- [Security best practices](./security-best-practices.md)
- [Users](../users/index.md)
- [Disaster recovery](../../install/operate/disaster-recovery.md)
