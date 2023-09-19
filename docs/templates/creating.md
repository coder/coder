#  Creating, editing, and updating templates

You create and edit Coder templates as [Terraform](./concepts.md)
configuration files (`.tf`) and any supporting files, like a README or
configuration files for other services.

## Who creates templates?

The [Template Admin](../admin/users.md) role (and above) can create
templates. End users, like developers, create workspaces from them.

Templates can also be [managed with git](./change-management.md),
allowing any developer to propose changes to a template.

You can give different users and groups access to templates with
[role-based access control](../admin/rbac.md).

## Starter templates

We provide starter templates for common cloud providers, like AWS, and
orchestrators, like Kubernetes. From there, you can modify them to use
your own images, VPC, cloud credentials, and so on. Coder supports all
Terraform resources and properties, so fear not if your favorite cloud
provider isn't here!

![Starter templates](../images/templates/starter-templates.png)

If you prefer to use Coder on the [command line](../cli.md), use
`coder templates init`.

> Coder starter templates are also available on our [GitHub
> repo](https://github.com/coder/coder/tree/main/examples/templates).

## Editing templates

Our starter templates are meant to be modified work for your use
cases. You can edit any template's files directly in the Coder
dashboard.

![Editing a template](../images/templates/choosing-edit-template.gif)

If you'd prefer to use the CLI, use `coder templates pull`, edit the
template files, then `coder templates push`.

> Even if you are a Terraform expert, we suggest reading our [guided
> tour](./tour.md).

## Updating templates

Coder tracks templates versions, keeping all developer workspaces
up-to-date. When you publish a new version, developers are notified to
get the latest infrastructure, software, or security patches. Learn
more about [change management](./change-management.md).

![Updating a template](../images/templates/update.png)

## Next step

- [Your first template](./tutorial.md)
