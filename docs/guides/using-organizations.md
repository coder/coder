# Using Organizations (Alpha)

> Note: Organizations is still under active development and requires a
> non-standard enterprise license to use. For more details,
> [contact your account team](https://coder.com/contact).

Organizations allow you to run a Coder deployment with multiple platform teams,
all with uniquely scoped templates, provisioners, users, groups, and workspaces.

## Prerequisites

- Coder deployment with non-standard license with Organizations enabled
  ([contact your account team](https://coder.com/contact))
- User with `Owner` role
- Coder CLI installed on local machine

## Enable the experiment

Organizations is still under an
[experimental flag](../cli/server.md#--experiments). To enable it, set the
following environment variable for the Coder server:

```sh
CODER_EXPERIMENTS=multi-organization
```

## The default organization

All Coder deployments start with one organization called `Default`.

To edit the organization details, navigate to `Deployment -> Organizations` in
the top bar:

![](../images/guides/using-organizations/deployment-organizations.png)

From there, you can manage the name, icon, description, users, and groups:

![](../images/guides/using-organizations/default-organization.png)

## Guide: Your first organization

### 1. Create the organization

Within the sidebar, click `New organization` to create an organization. In this
example, we'll create the `data-platform` org.

![](../images/guides/using-organizations/new-organization.png)

From there, let's deploy a provisioner and template for this organization.

### 2. Deploy a provisioner

[Provisioners](../admin/provisioners.md) are organization-scoped and are
responsible for executing Terraform/OpenTofu to provision the infrastructure for
workspaces and testing templates. Before creating templates, we must deploy at
least one provisioner:

using Coder CLI, run the following command to create a key that will be used to
authenticate the provisioner:

```sh
coder provisioner keys create data-cluster --org data-platform
Successfully created provisioner key data-cluster! Save this authentication token, it will not be shown again.

< key omitted >>
```

Next, on your desired platform, start the provisioner with the key. See our
[provisioner documentation](../admin/provisioners.md) for details on running on
additional platforms (e.g. Kubernetes). In this example, we'll start it directly
with the Coder CLI on a host with Docker:

```sh
coder provisionerd start --key=<key>
```

### 3. Create a template

WIP!

### 4. Add members

Navigate to the

## Planned work

Organizations is under active development. The following features are planned
before organizations are generally available:

- [ ] Sync OIDC claims to auto-assign users to organizations / roles
- [ ] View provisioner health and manage PSKs in the Coder UI
