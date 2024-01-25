# JFrog Artifactory Integration

<div>
  <a href="https://github.com/matifali" style="text-decoration: none; color: inherit;">
    <span style="vertical-align:middle;">M Atif Ali</span>
    <img src="https://github.com/matifali.png" width="24px" height="24px" style="vertical-align:middle; margin: 0px;"/>
  </a>
</div>
January 24, 20204

---

Use Coder and JFrog Artifactory together to secure your development environments
without disturbing your developers' existing workflows.

This guide will demonstrate how to use JFrog Artifactory as a package registry
within a workspace. We'll use Docker as the underlying compute. But, these
concepts apply to any compute platform.

The full example template can be found
[here](https://github.com/coder/coder/tree/main/examples/jfrog/docker).

## Requirements

- A JFrog Artifactory instance
- 1:1 mapping of users in Coder to users in Artifactory by email address or
  username
- Repositories configured in Artifactory for each package manager you want to
  use

## Provisioner Authentication

The most straight-forward way to authenticate your template with Artifactory is
by using our official Coder [modules](https://registry.coder.com). We publish
two type of modules that automate the JFrog Artifactory and Coder integration.

1. JFrog-OAuth:
2. JFrog-Token:

### JFrog-OAuth

This module is usable by JFrog self-hosted (on-premises) Artifactory as it
requires configuring a custom integration. This integration benefits from
Coder's [external-auth](https://coder.com/docs/v2/latest/admin/external-auth)
feature and allows each user to authenticate with Artifactory using an OAuth
flow and issues user-scoped tokens to each user. For instructions on how to set
this up, please see the details at:
https://registry.coder.com/modules/jfrog-oauth

```hcl
module "jfrog" {
  source = "registry.coder.com/modules/jfrog-oauth/coder"
  version = "1.0.0"
  agent_id = coder_agent.example.id
  jfrog_url = "https://jfrog.example.com"
  configure_code_server = true # this depends on the code-server
  username_field = "username" # If you are using GitHub to login to both Coder and Artifactory, use username_field = "username"
  package_managers = {
    "npm": "npm",
    "go": "go",
    "pypi": "pypi"
  }
}
```

### JFrog-Token

This module makes use of the
[Artifactory terraform provider](https://registry.terraform.io/providers/jfrog/artifactory/latest/docs)
and an admin-scoped token to create user-scoped tokens for each user by matching
their Coder email or username with Artifactory. This can be used for both SaaS
and self-hosted(on-premises) Artifactory instances. For Instructions on how to
configure this, please see the details at:
https://registry.coder.com/modules/jfrog-token

```hcl
module "jfrog" {
  source = "registry.coder.com/modules/jfrog-token/coder"
  version = "1.0.0"
  agent_id = coder_agent.example.id
  jfrog_url = "https://XXXX.jfrog.io"
  configure_code_server = true # this depends on the code-server
  artifactory_access_token = var.artifactory_access_token
  package_managers = {
    "npm": "npm",
    "go": "go",
    "pypi": "pypi"
  }
}
```

<blockquote class="info">
The admin-level access token is used to provision user tokens and is never exposed to
developers or stored in workspaces.
</blockquote>

## Offline Deployments

See the [offline deployments](../install/offline.md#coder-modules) section for
instructions on how to use coder-modules in an offline environment with
Artifactory.

## More reading

- See the full example template
  [here](https://github.com/coder/coder/tree/main/examples/jfrog/docker).
- To serve extensions from your own VS Code Marketplace, check out
  [code-marketplace](https://github.com/coder/code-marketplace#artifactory-storage).
- To store templates in Artifactory, check out our
  [Artifactory modules](../templates/modules.md#artifactory) docs.
