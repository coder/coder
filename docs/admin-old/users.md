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

Similar to dormant users, suspended users do not count towards the total number
of licensed seats.
