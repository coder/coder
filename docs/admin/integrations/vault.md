# Integrating HashiCorp Vault with Coder

<div>
  <a href="https://github.com/matifali" style="text-decoration: none; color: inherit;">
    <span style="vertical-align:middle;">Muhammad Atif Ali</span>
    <img src="https://github.com/matifali.png" alt="matifali" width="24px" height="24px" style="vertical-align:middle; margin: 0px;"/>

  </a>
</div>
August 05, 2024

---

This guide describes the process of integrating [HashiCorp Vault](https://www.vaultproject.io/) into Coder workspaces.

Coder makes it easy to integrate HashiCorp Vault with your workspaces by
providing official Terraform modules to integrate Vault with Coder. This guide
will show you how to use these modules to integrate HashiCorp Vault with Coder.

## The `vault-github` module

The [`vault-github`](https://registry.coder.com/modules/vault-github) module is a Terraform module that allows you to
authenticate with Vault using a GitHub token. This module uses the existing
GitHub [external authentication](../external-auth.md) to get the token and authenticate with Vault.

To use this module, add the following code to your Terraform configuration.

```tf
module "vault" {
  source               = "registry.coder.com/modules/vault-github/coder"
  version              = "1.0.7"
  agent_id             = coder_agent.example.id
  vault_addr           = "https://vault.example.com"
  coder_github_auth_id = "my-github-auth-id"
}
```

This module installs and authenticates the `vault` CLI in your Coder workspace.

Users then can use the `vault` CLI to interact with Vault; for example, to fetch
a secret stored in the KV backend.

```shell
vault kv get -namespace=YOUR_NAMESPACE -mount=MOUNT_NAME SECRET_NAME
```
