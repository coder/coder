# Template

Templates are written in [Terraform](https://developer.hashicorp.com/terraform/intro) and define the underlying infrastructure that all Coder workspaces run on.

![Starter templates](../../images/admin/templates/starter-templates.png)

<small>The "Starter Templates" page within the Coder dashboard.</small>

## Learn the concepts

While templates are written in standard Terraform, it's important to learn the Coder-specific concepts behind templates. The best way to learn the concepts is by
[creating a basic template from scratch](../../tutorials/template-from-scratch.md).

<!-- TODO; Consider linking to Terraform help docs -->

## Starter templates

After learning the basics, use starter templates to import a template with sensible defaults for popular platforms (e.g. AWS, Kubernetes, Docker, etc). Docs: [Create a template from a starter template](./creating-templates.md#from-a-starter-template).

## Extending templates

It's often necessary to extend the template to make it generally useful to end users. Common modifications are:

- Your image(s) (e.g. a Docker image with languages and tools installed)
- Additional parameters (e.g. disk size, instance type, or region)
- Additional IDEs (e.g. JetBrains) or features (e.g. dotfiles, RDP)

Learn more about the various ways you can [extend your templates](./extending-templates.md).

## Best Practices

We recommend starting with a universal template that can be used for basic tasks. As your Coder deployment grows, you can create more templates to meet the needs of different teams.

- [Image management](../../tutorials/image-management.md): Learn how to create and publish images for use within Coder workspaces & templates.
- [Dev Container support](#): Enable dev containers to allow teams to bring their own tools into Coder workspaces.
- [Template hardening](./): Configure your template to prevent certain resources from being destroyed (e.g. user disks).
- [Manage templates with Ci/Cd pipelines](#): Learn how to source control your templates and use GitOps to ensure template changes are reviewed and tested.

## Template permissions & policies (enterprise)

TODO
