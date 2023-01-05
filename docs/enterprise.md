# Enterprise Features

Coder is free to use and includes some features that are only accessible with a paid license.
[Contact Sales](https://coder.com/contact) for pricing or [get a free
trial](https://coder.com/trial).

| Category        | Feature                                                                     |  Open Source   | Enterprise |
| --------------- | --------------------------------------------------------------------------- | :------------: | :--------: |
| User Management | [Groups](./admin/groups.md)                                                 |                |     X      |
| User Management | [OIDC](./admin/auth.md)                                                     | X<sup>\*</sup> |     X      |
| User Management | [SCIM](./admin/auth.md#scim)                                                |                |     X      |
| Governance      | [Audit Logging](./admin/audit-logs.md)                                      |                |     X      |
| Governance      | [Browser Only Connections](./networking.md#browser-only-connections)        |                |     X      |
| Governance      | [Template Access Control](./admin/rbac.md)                                  |                |     X      |
| Cost Control    | [Quotas](./admin/quotas.md)                                                 |                |     X      |
| Cost Control    | [Max Workspace Auto-Stop](./templates.md#configure-max-workspace-auto-stop) |                |     X      |
| Deployment      | [High Availability](./admin/high-availability.md)                           |                |     X      |
| Deployment      | [Service Banners](./admin/service-banners.md)                               |                |     X      |
| Deployment      | [Git Provider Integration](./admin/git-providers.md)                        | X<sup>\*</sup> |     X      |
| Deployment      | Isolated Terraform Runners                                                  |                |     X      |

<sup>\*</sup> Free for up to 20 users

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
