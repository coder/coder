# Enterprise Features

Coder is free to use and includes some features that are only accessible with a paid license.
Contact sales@coder.com to obtain a license.

These features are available in the enterprise edition:

- [Audit Logging](./audit-logs.md)
- [Browser Only Connections](../networking.md#browser-only-connections)
- [Quotas](./quotas.md)
- [SCIM](./auth.md#scim)

And we're releasing these imminently:

- High Availability
- Template RBAC
- Multiple Git Provider Authentication

## Adding your license key

### You will need:

- Your license key (contact sales@coder.com if you don't have yours)
- Coder CLI installed

### Steps:

1. Save your license key to disk and make note of the path
2. Open a terminal
3. Ensure you are logged into your Coder deployment

   `coder login <access url>`

4. Run

   `coder licenses add -f <path to your license key>`

## Up Next

- [Learn how to contribute to Coder](../CONTRIBUTING.md).
