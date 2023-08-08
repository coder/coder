# Enterprise Features

Coder is free to use and includes some features that are only accessible with a paid license.
[Contact Sales](https://coder.com/contact) for pricing or [get a free
trial](https://coder.com/trial).

| Category        | Feature                                                                              | Open Source | Enterprise |
| --------------- | ------------------------------------------------------------------------------------ | :---------: | :--------: |
| User Management | [Groups](./admin/groups.md)                                                          |     ❌      |     ✅     |
| User Management | [Group & role sync](./admin/auth.md#group-sync-enterprise)                           |     ❌      |     ✅     |
| User Management | [SCIM](./admin/auth.md#scim)                                                         |     ❌      |     ✅     |
| Governance      | [Audit Logging](./admin/audit-logs.md)                                               |     ❌      |     ✅     |
| Governance      | [Browser Only Connections](./networking/#browser-only-connections-enterprise)        |     ❌      |     ✅     |
| Governance      | [Template Access Control](./admin/rbac.md)                                           |     ❌      |     ✅     |
| Cost Control    | [Quotas](./admin/quotas.md)                                                          |     ❌      |     ✅     |
| Cost Control    | [Max Workspace Autostop](./templates/#configure-max-workspace-autostop)              |     ❌      |     ✅     |
| Deployment      | [High Availability](./admin/high-availability.md)                                    |     ❌      |     ✅     |
| Deployment      | [Multiple Git Providers](./admin/git-providers.md#multiple-git-providers-enterprise) |     ❌      |     ✅     |
| Deployment      | [Appearance](./admin/appearance.md)                                                  |     ❌      |     ✅     |
| Deployment      | [Isolated Terraform Runners](./admin/provisioners.md)                                |     ❌      |     ✅     |
| Deployment      | [Workspace Proxies](./admin/workspace-proxies.md)                                    |     ❌      |     ✅     |

> Previous plans to restrict OIDC and Git Auth features in OSS have been removed
> as of 2023-01-11

## Adding your license key

### Requirements

- Your license key
- Coder CLI installed

### Instructions

1. Save your license key to disk and make note of the path
2. Open a terminal
3. Ensure you are logged into your Coder deployment

   `coder login <access url>`

4. Run

   `coder licenses add -f <path to your license key>`

## Up Next

- [Learn how to contribute to Coder](./CONTRIBUTING.md).
