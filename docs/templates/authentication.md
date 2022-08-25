# Provider Authentication

<blockquote class="danger">
  <p>
  Do not store secrets in templates. Assume every user has cleartext access
  to every template.
  </p>
</blockquote>

Coder's provisioner process needs to authenticate with cloud provider APIs to provision
workspaces. You can either pass credentials to the provisioner as parameters or execute Coder
in an environment that is authenticated with the cloud provider.

We encourage the latter where supported. This approach simplifies the template, keeps cloud
provider credentials out of Coder's database (making it a less valuable target for attackers),
and is compatible with agent-based authentication schemes (that handle credential rotation
and/or ensure the credentials are not written to disk).

Cloud providers for which the Terraform provider supports authenticated environments include

- [Google Cloud](https://registry.terraform.io/providers/hashicorp/google/latest/docs)
- [Amazon Web Services](https://registry.terraform.io/providers/hashicorp/aws/latest/docs)
- [Microsoft Azure](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs)
- [Kubernetes](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs)

Additional providers may be supported; check the
[documentation of the Terraform provider](https://registry.terraform.io/browse/providers) for
details.

The way these generally work is via the credentials being available to Coder either in some
well-known location on disk (e.g. `~/.aws/credentials` for AWS on posix systems), or via
environment variables. It is usually sufficient to authenticate using the CLI or SDK for the
cloud provider before running Coder for this to work, but check the Terraform provider
documentation for details.
