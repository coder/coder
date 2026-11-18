---
title: Respond to a security incident
---

This guide is for Coder deployment administrators who respond to a security incident, such as a leaked credential, a compromised account, or exposed data.
It covers how to contain the incident with the smallest action that stops it, how to respond when a component of the deployment is compromised, how to cut off access to every workspace when the scope is unclear, and how to restore access afterward.

> [!IMPORTANT]
> You need the Owner role to complete every step on this page.

The representational state transfer (REST) API examples use `curl` and [jq](https://jqlang.org/) in a Bash or Zsh shell, and read these environment variables:

```sh
export CODER_URL="https://coder.example.com"
export CODER_SESSION_TOKEN="$(coder login token)"
```

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
| A deployment component is compromised                         | [Respond to a compromised component](#respond-to-a-compromised-component)                                                              |
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
Expiring keeps the token in the audit trail.

<div class="tabs">

#### CLI

List every token in the deployment:

```sh
coder tokens list --all
```

`coder tokens remove` acts only on your own tokens.
To expire another user's token, use the REST API.

#### REST API

List every token in the deployment, with its ID and owner:

```sh
curl -fsS -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  "$CODER_URL/api/v2/users/me/keys/tokens?include_all=true" \
  | jq -r '.[] | [.id, .username, .token_name] | @tsv'
```

Expire a token with its ID and the owner's username:

```sh
curl -fsS -X PUT -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  "$CODER_URL/api/v2/users/<username>/keys/<token-id>/expire"
```

Refer to [Expire API key](../../reference/api/users.md#expire-api-key).

</div>

### Suspend a compromised user

A suspended user can't sign in, and Coder rejects every request made with that user's API tokens.
The workspace agents in that user's workspaces also can't authenticate to the control plane.
Suspension doesn't end the user's open sessions.
To end them, also [stop the user's workspaces](#3-stop-the-workspaces).

<div class="tabs">

#### Dashboard

1. Go to **Users**.
1. Select the vertical ellipsis next to the user, then select **Suspend**.
1. In the confirmation dialog, select **Suspend**.

#### CLI

```sh
coder users suspend <username> --yes
```

#### REST API

```sh
curl -fsS -X PUT -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  "$CODER_URL/api/v2/users/<username>/status/suspend"
```

Refer to [Suspend user account](../../reference/api/users.md#suspend-user-account).

</div>

If the attacker holds the user's IdP credentials, also turn off the account in the IdP.
Otherwise, the attacker can sign in again after you reactivate the user in Coder.

### Contain a compromised owner

> [!NOTE]
> If your deployment syncs site roles from OIDC with [`--oidc-user-role-field`](../../reference/cli/server/index.md#--oidc-user-role-field), Coder rejects role changes for OIDC users.
> A role removed in your IdP reaches Coder only when the user signs in again, so it doesn't contain an attacker who is already signed in.

Coder doesn't let you suspend a user with the Owner role, and you can't change your own roles.
From another owner account, remove the account's site roles, then suspend it.
When site roles sync from OIDC, first unset `--oidc-user-role-field` and restart the control plane.
After you suspend the account, turn it off in your IdP, then restore the setting.

<div class="tabs">

#### Dashboard

1. Go to **Users**.
1. Select the vertical ellipsis next to the account, then select **Edit roles**.
1. Clear **Owner** and every other role, then select **Confirm**.
1. Select the vertical ellipsis again, then select **Suspend**.
1. In the confirmation dialog, select **Suspend**.

#### CLI

The CLI keeps the account's other site roles, because `coder users edit-roles --yes` needs at least one role in `--roles`.
To clear every role, use the dashboard or the REST API.

```sh
user="<username>"
keep="$(coder users show "$user" --output json \
  | jq -r '[.roles[].name | select(. != "owner")] | join(",")')"
coder users edit-roles "$user" --roles "$keep" --yes
coder users suspend "$user" --yes
```

When Owner is the account's only site role, `edit-roles` fails with `--roles is required when using --yes`.

#### REST API

This script clears every site role on the account, then suspends it:

```sh
user="<username>"
curl -fsS -X PUT -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" -d '{"roles": []}' \
  "$CODER_URL/api/v2/users/$user/roles"
curl -fsS -X PUT -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  "$CODER_URL/api/v2/users/$user/status/suspend"
```

Refer to [Assign role to user](../../reference/api/users.md#assign-role-to-user).

</div>

#### Break-glass owner accounts

A break-glass account is an owner that signs in with a password instead of single sign-on.
When its password leaks, reset the password from another owner account before you remove the role.
A reset through the dashboard or the REST API deletes every API key that the account holds, which ends the attacker's API access at once.

<div class="tabs">

##### Dashboard

1. Go to **Users**.
1. Select the vertical ellipsis next to the account, then select **Reset password**.
1. Store the temporary password in your secrets manager, then select **Reset password**.

##### REST API

```sh
curl -fsS -X PUT -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" -d '{"password": "<new-password>"}' \
  "$CODER_URL/api/v2/users/<username>/password"
```

</div>

`coder reset-password` writes the new password straight to the database and doesn't delete the account's API keys.
After a `coder reset-password`, expire the account's tokens separately.

If no owner account is trustworthy, create a new owner with direct database access, then contain the compromised owners from it:

```sh
coder server create-admin-user --postgres-url "<connection-url>"
```

This command has no REST API equivalent, because every API call that grants the Owner role needs an owner's credentials.
Refer to [`coder server create-admin-user`](../../reference/cli/server/create-admin-user.md).

## Respond to a compromised component

When an attacker reaches a component of the deployment, rotate the secrets that the component holds.
Each component holds different secrets.

### PostgreSQL database

The database holds the state of the whole deployment.
Coder stores API keys and provisioner keys as hashes, but stores OAuth tokens, user and workspace secrets, and signing keys in plaintext unless you turn on [database encryption](./database-encryption.md).

1. Rotate the database credentials, then update [`--postgres-url`](../../reference/cli/server/index.md#--postgres-url) on every control plane replica and restart it.
1. Treat stored external authentication tokens as exposed.
   Revoke them in each external provider.
1. If the attacker could write to the database, restore it from a backup taken before the incident.

### Control plane

A control plane host holds the database connection URL and the secrets in the `coder server` configuration.
Rotate each one that the deployment uses:

- The database credentials in [`--postgres-url`](../../reference/cli/server/index.md#--postgres-url).
- The OIDC client secret in [`--oidc-client-secret`](../../reference/cli/server/index.md#--oidc-client-secret).
- Each external authentication client secret, such as `CODER_EXTERNAL_AUTH_0_CLIENT_SECRET`.
- The [external token encryption keys](./database-encryption.md#rotate-keys).
- The TLS key in [`--tls-key-file`](../../reference/cli/server/index.md#--tls-key-file).
- The provisioner pre-shared key in [`--provisioner-daemon-psk`](../../reference/cli/server/index.md#--provisioner-daemon-psk).
- The cloud credentials that the built-in provisioners use to build workspaces.

Redeploy the control plane from a trusted image, then follow the [PostgreSQL database](#postgresql-database) steps, because the attacker held the database credentials.

### External provisioner

An external provisioner holds its provisioner key and the cloud credentials that it uses to build workspaces.

1. Delete its key, then create a new one for the replacement provisioner:

   ```sh
   coder provisioner keys delete <key-name>
   ```

1. Rotate the cloud credentials that the provisioner used.
1. Review the template versions that the provisioner built during the incident.

Refer to [External provisioners](../../install/operate/provisioners/index.md).

### Workspace proxy

Regenerate the proxy's token, which invalidates the old one:

```sh
coder workspace-proxy regenerate-token <proxy-name>
```

Then redeploy the proxy from a trusted image with the new token.

### Workspace

[Suspend the workspace owner](#suspend-a-compromised-user), then stop the workspace.
Stopping usually removes the compute, so take a snapshot of the disk first if the investigation needs it.

## Cut off access to every workspace

Cut off access to every workspace when you can't tell which accounts or workspaces the attacker reached.
Work through these steps in order.

### 1. Suspend the affected users

Suspend every user that the incident might cover, as in [Suspend a compromised user](#suspend-a-compromised-user).
A user who isn't suspended can start their workspace again after you stop it.

When you can't tell which users are affected, and you trust the owner accounts, suspend every user that doesn't have the Owner role.
This includes service accounts, so automation that uses them stops too.

<div class="tabs">

#### CLI

```sh
coder users list --output json \
  | jq -r '.[] | select(.status != "suspended")
      | select([.roles[].name] | index("owner") | not) | .username' \
  | xargs -n 1 -P 10 coder users suspend --yes
```

#### REST API

```sh
curl -fsS -H "Coder-Session-Token: $CODER_SESSION_TOKEN" "$CODER_URL/api/v2/users" \
  | jq -r '.users[] | select(.status != "suspended")
      | select([.roles[].name] | index("owner") | not) | .username' \
  | xargs -n 1 -P 10 -I {} curl -fsS -X PUT -o /dev/null \
      -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
      "$CODER_URL/api/v2/users/{}/status/suspend"
```

</div>

If you don't trust every owner account, first [contain the compromised owners](#contain-a-compromised-owner).

### 2. Expire leaked tokens

Expire every token that the incident might cover, as in [Expire leaked tokens](#expire-leaked-tokens).

### 3. Stop the workspaces

Stopping a workspace runs the template's stop transition.
In templates that follow the [resource persistence](../../templates/extending-templates/resource-persistence.md) pattern, stop removes the compute and keeps the persistent storage.
When the compute is gone, every open session to it ends, including direct ones.

<div class="tabs">

#### Dashboard

1. Go to **Workspaces**.
1. Filter the list with `status:running`.
1. Select the checkbox in the table header to select every workspace on the page.
1. Select **Actions** > **Stop**, then confirm.
1. Repeat for each page of results.

#### CLI

```sh
coder list --search "status:running" --output json \
  | jq -r '.[] | "\(.owner_name)/\(.name)"' \
  | xargs -n 1 -P 10 coder stop --yes
```

A search query replaces the default `owner:me` filter, so an owner sees running workspaces for every user.
`coder stop` waits for its build to finish, so `-P 10` runs 10 stops at a time.

</div>

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

> [!IMPORTANT]
> Audit logs and connection logs require a [Premium license](https://coder.com/pricing#compare-plans).
> Without them, you rely on `coderd` logs and the logs of your platform for the investigation.

1. Review the [audit logs](./audit-logs.md) for actions taken by the affected users and tokens.
1. Review the [connection logs](../monitoring/connection-logs.md) for workspace sessions during the incident.
1. Rotate any credential that the incident exposed, such as external authentication tokens, IdP client secrets, and template secrets.
1. If you use database encryption, [rotate the encryption keys](./database-encryption.md#rotate-keys) when the keys might be exposed.
1. Remove the network blocks, then [activate](../users/index.md#activate-a-suspended-user) the users that you suspended.

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
