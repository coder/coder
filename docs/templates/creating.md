#  Creating, editing, and updating templates

You create and edit Coder templates as [Terraform](./concepts.md) configuration files (`.tf`).

## Who creates templates?

The [Template Admin](../admin/users.md) role (and above) can create templates. End users (developers) create workspaces from them.

Templates can also be [managed witg git](./change-management.md), allowing any developer to propose changes to a template.

> [Template RBAC](../admin/rbac.md) allows you to give different users & groups access to templates.

## Starter templates

We provide starter templates for common cloud providers (e.g. AWS) and orchestrators (e.g. Kubernetes). From there, you can modify them with [Terraform](https://terraform.io) to use your own images, VPC, cloud credentials, etc. All Terraform resources and properties are supported, so fear not if your favorite cloud isn't here!

![Starter templates](https://user-images.githubusercontent.com/22407953/256705348-e6fb2963-27f5-414f-9f5c-345cd3b7ee28.png)

If you'd prefer to use the CLI, use `coder templates init`.

> The Terraform code for our starter templates are avalible on our [GitHub](https://github.com/coder/coder/tree/main/examples/templates).

## Editing templates

Our starter templates are meant to be modified work for your use cases! You can edit the Terraform code for a template directly in the UI.

![Editing a template](https://user-images.githubusercontent.com/22407953/256706060-71fb48f4-9a1b-42ad-9380-0ecc02db3218.gif)

If you'd prefer to use the CLI, use `coder templates pull` and `coder templates push`.

> Even if you are a Terraform expert, we suggest reading our full guide on [writing Coder templates](./managing.md).

## Updating templates

Templates are versioned, keeping all developer workspaces up-to-date. When a new version is published, developers are notified to get the latest infrastructure, software, or security patches.

![Template update screen](https://user-images.githubusercontent.com/22407953/256712740-96121f81-a3c8-4be0-90dc-c1c4cabed634.png)

## Next step

- [Your first templates](./tutorial.md)
