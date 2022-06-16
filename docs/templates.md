# Templates

Templates define the infrastructure underlying workspaces. Each Coder deployment
can have multiple templates for different workloads, such as ones for front-end
development, Windows development, and so on.

Coder manages templates, including sharing them and rolling out updates
to everybody. Users can also manually update their workspaces.

## Manage templates

Coder provides production-ready [sample templates](https://github.com/coder/coder/tree/main/examples/templates),
but you can modify the templates with Terraform.

```sh
# start from an example
coder templates init

# optional: modify the template
vim <template-name>/main.tf

# add the template to Coder deployment
coder templates <create/update> <template-name>
```

## Parameters

Templates often contain *parameters*. In Coder, there are two types of parameters:

- **Admin parameters** are set when a template is created/updated. These values
  are often cloud secrets, such as a `ServiceAccount` token, and are annotated
  with `sensitive =  true` in the template code.

- **User parameters** are set when a user creates a workspace. They are unique
  to each workspace, often personalization settings such as "preferred region"
  or "workspace image".

## Best Practices

### Template Changes

We recommend source controlling your templates.

### Authenticating with Cloud Providers

Coder's provisioner process needs to authenticate with cloud provider APIs to provision
workspaces. We strongly advise against including credentials directly in your templates. You
can either pass credentials to the provisioner as parameters, or execute Coder
in an environment that is authenticated with the cloud provider.

We encourage the latter where supported.  This approach simplifies the template, keeps cloud
provider credentials out of Coder's database (making it a less valuable target for attackers),
and is compatible with agent-based authentication schemes (that handle credential rotation
and/or ensure the credentials are not written to disk).

Cloud providers for which the Terraform provider supports authenticated environments include:

- [Google Cloud](https://registry.terraform.io/providers/hashicorp/google/latest/docs)
- [Amazon Web Services](https://registry.terraform.io/providers/hashicorp/aws/latest/docs)
- [Microsoft Azure](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs)
- [Kubernetes](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs)

Additional providers may be supported; check the
[documentation of the Terraform provider](https://registry.terraform.io/browse/providers) for
details.

The way these generally work is via the credentials being available to Coder either in some
well-known location on disk (e.g. `~/.aws/credentials` for AWS on posix systems), or via
environment variables.  It is usually sufficient to authenticate using the CLI or SDK for the
cloud provider before running Coder for this to work, but check the Terraform provider
documentation for details.

---

Next: [Workspaces](./workspaces.md)
