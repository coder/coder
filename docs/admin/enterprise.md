# Enterprise Features

Coder is free to use and includes some features that are only accessible with a paid license.
Contact sales@coder.com to obtain a license.

### User Management
- [Groups](./groups.md)
- [Template RBAC](./rbac.md)
- [SCIM](./auth.md#scim)

### Networking & Deployment
- [High Availability](./high-availability.md)
- [Browser Only Connections](../networking.md#browser-only-connections)

### Other
- [Audit Logging](./audit-logs.md)
- [Quotas](./quotas.md)

### Coming soon

- Multiple Git Provider Authentication
- Max Workspace Auto-Stop

## Adding your license key

### Requirements

- Your license key (contact sales@coder.com if you don't have yours)
- Coder CLI installed

### Instructions

1. Save your license key to disk and make note of the path
2. Open a terminal
3. Ensure you are logged into your Coder deployment

   `coder login <access url>`

4. Run

   `coder licenses add -f <path to your license key>`

## Up Next

- [Learn how to contribute to Coder](../CONTRIBUTING.md).
