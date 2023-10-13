# Users

This article walks you through the user roles available in Coder and creating
and managing users.

## Roles

Coder offers these user roles in the community edition:

|                                                       | Auditor | User Admin | Template Admin | Owner |
| ----------------------------------------------------- | ------- | ---------- | -------------- | ----- |
| Add and remove Users                                  |         | ✅         |                | ✅    |
| Manage groups (enterprise)                            |         | ✅         |                | ✅    |
| Change User roles                                     |         |            |                | ✅    |
| Manage **ALL** Templates                              |         |            | ✅             | ✅    |
| View **ALL** Workspaces                               |         |            | ✅             | ✅    |
| Update and delete **ALL** Workspaces                  |         |            |                | ✅    |
| Run [external provisioners](./provisioners.md)        |         |            | ✅             | ✅    |
| Execute and use **ALL** Workspaces                    |         |            |                | ✅    |
| View all user operation [Audit Logs](./audit-logs.md) | ✅      |            |                | ✅    |

A user may have one or more roles. All users have an implicit Member role that
may use personal workspaces.

## Security notes

A malicious Template Admin could write a template that executes commands on the
host (or `coder server` container), which potentially escalates their privileges
or shuts down the Coder server. To avoid this, run
[external provisioners](./provisioners.md).

In low-trust environments, we do not recommend giving users direct access to
edit templates. Instead, use
[CI/CD pipelines to update templates](../templates/change-management.md) with
proper security scans and code reviews in place.

## User status

Coder user accounts can have different status types: active, dormant, and
suspended.

### Active user

An _active_ user account in Coder is the default and desired state for all
users. When a user's account is marked as _active_, they have complete access to
the Coder platform and can utilize all of its features and functionalities
without any limitations. Active users can access workspaces, templates, and
interact with Coder using CLI.

### Dormant user

A user account is set to _dormant_ status when they have not yet logged in, or
have not logged into the Coder platform for the past 90 days. Once the user logs
in to the platform, the account status will switch to _active_.

Dormant accounts do not count towards the total number of licensed seats in a
Coder subscription, allowing organizations to optimize their license usage.

### Suspended user

When a user's account is marked as _suspended_ in Coder, it means that the
account has been temporarily deactivated, and the user is unable to access the
platform.

Only user administrators or owners have the necessary permissions to manage
suspended accounts and decide whether to lift the suspension and allow the user
back into the Coder environment. This level of control ensures that
administrators can enforce security measures and handle any compliance-related
issues promptly.

## Create a user

To create a user with the web UI:

1. Log in as a user admin.
2. Go to **Users** > **New user**.
3. In the window that opens, provide the **username**, **email**, and
   **password** for the user (they can opt to change their password after their
   initial login).
4. Click **Submit** to create the user.

The new user will appear in the **Users** list. Use the toggle to change their
**Roles** if desired.

To create a user via the Coder CLI, run:

```shell
coder users create
```

When prompted, provide the **username** and **email** for the new user.

You'll receive a response that includes the following; share the instructions
with the user so that they can log into Coder:

```console
Download the Coder command line for your operating system:
https://github.com/coder/coder/releases/latest

Run  coder login https://<accessURL>.coder.app  to authenticate.

Your email is:  email@exampleCo.com
Your password is:  <redacted>

Create a workspace   coder create !
```

## Suspend a user

User admins can suspend a user, removing the user's access to Coder.

To suspend a user via the web UI:

1. Go to **Users**.
2. Find the user you want to suspend, click the vertical ellipsis to the right,
   and click **Suspend**.
3. In the confirmation dialog, click **Suspend**.

To suspend a user via the CLI, run:

```shell
coder users suspend <username|user_id>
```

Confirm the user suspension by typing **yes** and pressing **enter**.

## Activate a suspended user

User admins can activate a suspended user, restoring their access to Coder.

To activate a user via the web UI:

1. Go to **Users**.
2. Find the user you want to activate, click the vertical ellipsis to the right,
   and click **Activate**.
3. In the confirmation dialog, click **Activate**.

To activate a user via the CLI, run:

```shell
coder users activate <username|user_id>
```

Confirm the user activation by typing **yes** and pressing **enter**.

## Reset a password

To reset a user's via the web UI:

1. Go to **Users**.
2. Find the user whose password you want to reset, click the vertical ellipsis
   to the right, and select **Reset password**.
3. Coder displays a temporary password that you can send to the user; copy the
   password and click **Reset password**.

Coder will prompt the user to change their temporary password immediately after
logging in.

You can also reset a password via the CLI:

```shell
# run `coder reset-password <username> --help` for usage instructions
coder reset-password <username>
```

> Resetting a user's password, e.g., the initial `owner` role-based user, only
> works when run on the host running the Coder control plane.

### Resetting a password on Kubernetes

```shell
kubectl exec -it deployment/coder /bin/bash -n coder

coder reset-password <username>
```

## User filtering

In the Coder UI, you can filter your users using pre-defined filters or by
utilizing the Coder's filter query. The examples provided below demonstrate how
to use the Coder's filter query:

- To find active users, use the filter `status:active`.
- To find admin users, use the filter `role:admin`.
- To find users have not been active since July 2023:
  `status:active last_seen_before:"2023-07-01T00:00:00Z"`

The following filters are supported:

- `status` - Indicates the status of the user. It can be either `active`,
  `dormant` or `suspended`.
- `role` - Represents the role of the user. You can refer to the
  [TemplateRole documentation](https://pkg.go.dev/github.com/coder/coder/v2/codersdk#TemplateRole)
  for a list of supported user roles.
- `last_seen_before` and `last_seen_after` - The last time a used has used the
  platform (e.g. logging in, any API requests, connecting to workspaces). Uses
  the RFC3339Nano format.
