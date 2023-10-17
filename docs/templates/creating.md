# Working with templates

You create and edit Coder templates as [Terraform](./tour.md) configuration
files (`.tf`) and any supporting files, like a README or configuration files for
other services.

## Who creates templates?

The [Template Admin](../admin/users.md) role (and above) can create templates.
End users, like developers, create workspaces from them.

Templates can also be [managed with git](./change-management.md), allowing any
developer to propose changes to a template.

You can give different users and groups access to templates with
[role-based access control](../admin/rbac.md).

## Starter templates

We provide starter templates for common cloud providers, like AWS, and
orchestrators, like Kubernetes. From there, you can modify them to use your own
images, VPC, cloud credentials, and so on. Coder supports all Terraform
resources and properties, so fear not if your favorite cloud provider isn't
here!

![Starter templates](../images/templates/starter-templates.png)

If you prefer to use Coder on the [command line](../cli.md), use
`coder templates init`.

> Coder starter templates are also available on our
> [GitHub repo](https://github.com/coder/coder/tree/main/examples/templates).

## Community Templates

As well as Coder's starter templates, you can see a list of community templates
by our users
[here](https://github.com/coder/coder/blob/main/examples/templates/community-templates.md).

## Editing templates

Our starter templates are meant to be modified for your use cases. You can edit
any template's files directly in the Coder dashboard.

![Editing a template](../images/templates/choosing-edit-template.gif)

If you'd prefer to use the CLI, use `coder templates pull`, edit the template
files, then `coder templates push`.

> Even if you are a Terraform expert, we suggest reading our
> [guided tour](./tour.md).

## Updating templates

Coder tracks a template's versions, keeping all developer workspaces up-to-date.
When you publish a new version, developers are notified to get the latest
infrastructure, software, or security patches. Learn more about
[change management](./change-management.md).

![Updating a template](../images/templates/update.png)

## Delete templates

You can delete a template using both the coder CLI and UI. Only
[template admins and owners](../admin/users.md) can delete a template, and the
template must not have any running workspaces associated to it.

In the UI, navigate to the template you want to delete, and select the dropdown
in the right-hand corner of the page to delete the template.

![delete-template](../images/delete-template.png)

Using the CLI, login to Coder and run the following command to delete a
template:

```shell
coder templates delete <template-name>
```

### Delete workspaces

When a workspace is deleted, the Coder server essentially runs a
[terraform destroy](https://www.terraform.io/cli/commands/destroy) to remove all
resources associated with the workspace.

> Terraform's
> [prevent-destroy](https://www.terraform.io/language/meta-arguments/lifecycle#prevent_destroy)
> and
> [ignore-changes](https://www.terraform.io/language/meta-arguments/lifecycle#ignore_changes)
> meta-arguments can be used to prevent accidental data loss.

## Next steps

- [Your first template](./tutorial.md)
