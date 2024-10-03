# Groups

Groups can be used with [template RBAC](../templates/template-permissions.md) to
give groups of users access to specific templates. They can be defined via the
Coder web UI,
[synced from your identity provider](./oidc-auth.md#group-sync-enterprise-premium)
or
[managed via Terraform](https://registry.terraform.io/providers/coder/coderd/latest/docs/resources/template).

![Groups](../../images/groups.png)

## Enabling this feature

This feature is only available with a
[Premium or Enterprise license](https://coder.com/pricing).
