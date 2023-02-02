# Users

This article walks you through the user roles available in Coder and creating and managing users.

## Roles

Coder offers these user roles in the community edition:

|                                                       | Auditor | User Admin | Template Admin | Owner |
| ----------------------------------------------------- | ------- | ---------- | -------------- | ----- |
| Add and remove Users                                  |         | ✅         |                | ✅    |
| Manage groups (enterprise)                            |         | ✅         |                | ✅    |
| Change User roles                                     |         |            |                | ✅    |
| Manage **ALL** Templates                              |         |            | ✅             | ✅    |
| View, update and delete **ALL** Workspaces            |         |            | ✅             | ✅    |
| Execute and use **ALL** Workspaces                    |         |            |                | ✅    |
| View all user operation [Audit Logs](./audit-logs.md) | ✅      |            |                | ✅    |

A user may have one or more roles. All users have an implicit Member role
that may use personal workspaces.

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

```console
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

```console
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

```console
coder users activate <username|user_id>
```

Confirm the user activation by typing **yes** and pressing **enter**.

## Reset a password

To reset a user's via the web UI:

1. Go to **Users**.
2. Find the user whose password you want to reset, click the vertical ellipsis to the right,
   and select **Reset password**.
3. Coder displays a temporary password that you can send to the user; copy the
   password and click **Reset password**.

Coder will prompt the user to change their temporary password immediately after logging in.

You can also reset a password via the CLI:

```console
# run `coder reset-password <username> --help` for usage instructions
coder reset-password <username>
```
