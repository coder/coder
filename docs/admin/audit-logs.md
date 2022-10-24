# Audit Logs

Audit Logs allows **Auditors** to monitor user operations in
their deployment.

## Tracked Events

We track **create, update and delete** events for the following resources:

- GitSSHKey
- Template
- TemplateVersion
- Workspace
- User

## Filtering logs

In the Coder UI you can filter your audit logs using the pre-defined filter or by using the Coder's filter query like the examples below:

- `resource_type:workspace action:delete` to find deleted workspaces
- `resource_type:template action:create` to find created templates

The supported filters are:

- `resource_type` - The type of the resource. It can be a workspace, template, user, etc. You can [find here](https://pkg.go.dev/github.com/coder/coder@main/codersdk#ResourceType) all the resource types that are supported.
- `resource_id` - The ID of the resource.
- `resource_target` - The name of the resource. Can be used instead of `resource_id`.
- `action`- The action applied to a resource. You can [find here](https://pkg.go.dev/github.com/coder/coder@main/codersdk#AuditAction) all the actions that are supported.
- `username` - The username of the user who triggered the action.
- `email` - The email of the user who triggered the action.

## Enabling this feature

This feature is only available with an enterprise license. [Learn more](../enterprise.md)
