---
title: Respond to a security incident
---

This guide is for Coder deployment administrators who respond to a security incident, such as a leaked credential, a compromised account, or exposed data.
It covers how to contain the incident with the smallest action that stops it, how to cut off access to every workspace when the scope is unclear, and how to restore access afterward.

> [!IMPORTANT]
> You need the Owner role to complete every step on this page.

## Choose a response

Contain the incident with the narrowest action that stops it.
A narrow response keeps unaffected users working and preserves the state that the investigation needs.
Cut off access to every workspace only when you can't tell what the attacker can reach.

| Incident                                                      | Contain it                                                                                                                             |
|---------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------|
| An API token leaks through a script, a CI log, or a repo      | [Expire the token](#expire-leaked-tokens)                                                                                              |
| A user account is compromised                                 | [Suspend the user](#suspend-a-compromised-user), then turn off the account in your identity provider (IdP)                             |
| An account with the Owner role is compromised                 | [Contain a compromised owner](#contain-a-compromised-owner)                                                                            |
| Credentials are hard-coded in a template or a workspace image | Rotate the credentials, then push a template version without them                                                                      |
| A shared workspace port exposes internal data                 | Stop the workspace, then lower the [maximum port sharing level](../networking/port-forwarding.md#configure-maximum-port-sharing-level) |
| The scope is unknown, or many accounts are affected           | [Cut off access to every workspace](#cut-off-access-to-every-workspace)                                                                |

## Know what each action blocks

Coder actions differ in whether they end sessions that are already open.

| Action                       | Blocks new access              | Ends open sessions                                 |
|------------------------------|--------------------------------|----------------------------------------------------|
| Suspend a user               | Yes, for that user             | No                                                 |
| Expire an API token          | Yes, for that token            | No                                                 |
| Stop workspaces              | Until a user starts them again | Yes, when the template removes the compute on stop |
| Stop the control plane       | Yes                            | No, for direct connections                         |
| Block traffic at the network | Yes                            | Yes                                                |

Stopping the control plane alone isn't a reliable cutoff.
A direct connection runs on a WireGuard tunnel between the client and the workspace agent, and the tunnel doesn't pass through the control plane.
An active direct session, such as `coder ssh`, can keep working after the control plane stops.
Stopping the control plane also stops the dashboard and API that you use for the other steps.

## Contain the incident

### Expire leaked tokens

When a token leaked but its user stays trusted, expire the token instead of suspending the user.

1. List every token in the deployment:

   ```sh
   coder tokens list --all
   ```

1. Expire each leaked token with the [expire API key endpoint](../../reference/api/users.md#expire-api-key), `PUT /api/v2/users/{user}/keys/{keyid}/expire`.

   Expiring keeps the token in the audit trail.
   `coder tokens remove` acts only on your own tokens.

### Suspend a compromised user

A suspended user can't sign in, and Coder rejects every request made with that user's API tokens.
The workspace agents in that user's workspaces also can't authenticate to the control plane.

1. Go to **Users**.
1. Select the vertical ellipsis next to the user, then select **Suspend**.
1. In the confirmation dialog, select **Suspend**.

From the CLI, run:

```sh
coder users suspend <username>
```

Suspension doesn't end the user's open sessions.
To end them, also [stop the user's workspaces](#3-stop-the-workspaces).

If the attacker holds the user's IdP credentials, also turn off the account in the IdP.
Otherwise, the attacker can sign in again after you reactivate the user in Coder.

### Contain a compromised owner

Coder doesn't let you suspend a user with the Owner role, and you can't change your own roles.
Another owner removes the Owner role first, then suspends the account:

1. Remove the Owner role.
   Run `coder users edit-roles <username>` and clear **owner** in the role picker.
1. Suspend the user:

   ```sh
   coder users suspend <username>
   ```

If your deployment syncs site roles from OIDC, Coder rejects role changes for OIDC users.
Remove the Owner role in your IdP instead.

#### Break-glass owner accounts

A break-glass account is an owner that signs in with a password instead of single sign-on.
When its password leaks, reset the password from another owner account before you remove the role.
A reset through the dashboard or the API deletes every API key that the account holds, which ends the attacker's API access at once.

1. Go to **Users**.
1. Select the vertical ellipsis next to the account, then select **Reset password**.
1. Store the temporary password in your secrets manager, then select **Reset password**.

`coder reset-password` writes the new password straight to the database and doesn't delete the account's API keys.
After a `coder reset-password`, expire the account's tokens separately.

If no owner account is trustworthy, create a new owner with database access, then contain the compromised owners from it:

```sh
coder server create-admin-user --postgres-url "<connection-url>"
```

Refer to [`coder server create-admin-user`](../../reference/cli/server/create-admin-user.md).

## Cut off access to every workspace

Cut off access to every workspace when you can't tell which accounts or workspaces the attacker reached.
Work through these steps in order.

### 1. Suspend the affected users

Suspend every user that the incident might cover, as in [Suspend a compromised user](#suspend-a-compromised-user).
A user who isn't suspended can start their workspace again after you stop it.

### 2. Expire leaked tokens

Expire every token that the incident might cover, as in [Expire leaked tokens](#expire-leaked-tokens).

### 3. Stop the workspaces

Stopping a workspace runs the template's stop transition.
In templates that follow the [resource persistence](../../templates/extending-templates/resource-persistence.md) pattern, stop removes the compute and keeps the persistent storage.
When the compute is gone, every open session to it ends, including direct ones.

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
If you use [custom relays](../networking/index.md#custom-relays), block user traffic to them too.
Blocking workspace outbound traffic stops direct sessions and data exfiltration from the workspaces.

## Investigate and restore access

1. Review the [audit logs](./audit-logs.md) for actions taken by the affected users and tokens.
1. Review the [connection logs](../monitoring/connection-logs.md) for workspace sessions during the incident.
1. Rotate any credential that the incident exposed, such as external authentication tokens, IdP client secrets, and template secrets.
1. If you use database encryption, [rotate the encryption keys](./database-encryption.md#rotate-keys) when the keys might be exposed.
1. Remove the network blocks, then [activate](../users/index.md#activate-a-suspended-user) the users that you suspended.

Audit logs and connection logs require a [Premium license](https://coder.com/pricing#compare-plans).

## Prevent common incidents

Most incidents start with a deployment that drifted from the [security best practices](./security-best-practices.md):

- Set a short [`--max-token-lifetime`](../../reference/cli/server/index.md#--max-token-lifetime) and use one token per purpose.
- Require single sign-on, and keep break-glass owner accounts few, with passwords in a secrets manager.
- Deliver credentials with [secrets](./secrets.md) or external authentication, not in templates or images.
- Limit the [maximum port sharing level](../networking/port-forwarding.md#configure-maximum-port-sharing-level).
- Restrict [outbound network access](./security-best-practices.md#outbound-network-access) from workspaces.

## Learn more

- [Security best practices](./security-best-practices.md)
- [Users](../users/index.md)
