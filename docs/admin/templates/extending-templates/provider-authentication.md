# Provider Authentication

<blockquote class="danger">
  <p>
  Do not store secrets in templates. Assume every user has cleartext access
  to every template.
  </p>
</blockquote>

The Coder server's
[provisioner](https://registry.terraform.io/providers/coder/coder/latest/docs/data-sources/provisioner)
process needs to authenticate with other provider APIs to provision workspaces.
There are two approaches to do this:

- Pass credentials to the provisioner as parameters.
- Preferred: Execute the Coder server in an environment that is authenticated
  with the provider.

We encourage the latter approach where supported:

- Simplifies the template.
- Keeps provider credentials out of Coder's database, making it a less valuable
  target for attackers.
- Compatible with agent-based authentication schemes, which handle credential
  rotation or ensure the credentials are not written to disk.

Generally, you can set up an environment to provide credentials to Coder in
these ways:

- A well-known location on disk. For example, `~/.aws/credentials` for AWS on
  POSIX systems.
- Environment variables.

It is usually sufficient to authenticate using the CLI or SDK for the provider
before running Coder, but check the Terraform provider's documentation for
details.

These platforms have Terraform providers that support authenticated
environments:

- [Google Cloud](https://registry.terraform.io/providers/hashicorp/google/latest/docs)
- [Amazon Web Services](https://registry.terraform.io/providers/hashicorp/aws/latest/docs)
- [Microsoft Azure](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs)
- [Kubernetes](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs)
- [Docker](https://registry.terraform.io/providers/kreuzwerker/docker/latest/docs)

## Use a remote Docker host for authentication

There are two ways to use a remote Docker host for authentication:

- Configure the Docker provider to use a
  [remote host over SSH or TCP](https://registry.terraform.io/providers/kreuzwerker/docker/latest/docs#remote-hosts).
- Run an [external provisioner](../../provisioners.md) on the remote docker
  host.

Other providers might also support authenticated environments. Check the
[documentation of the Terraform provider](https://registry.terraform.io/browse/providers)
for details.
