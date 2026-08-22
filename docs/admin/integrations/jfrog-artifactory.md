---
title: JFrog Artifactory Integration
---

Use Coder and JFrog Artifactory together to secure your development environments
without disturbing your developers' existing workflows.

This guide will demonstrate how to use JFrog Artifactory as a package registry
within a workspace.

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

1. [JFrog-OAuth](https://registry.coder.com/modules/jfrog-oauth)
1. [JFrog-Token](https://registry.coder.com/modules/jfrog-token)

### JFrog-OAuth

This module works with both JFrog SaaS (for example, `example.jfrog.io`) and self-hosted (on-premises) Artifactory.
It uses Coder's [external-auth](../external-auth/index.md) feature, so each user authenticates with Artifactory through an OAuth flow, and Coder issues a user-scoped access token to each workspace.

To set this up, follow these steps:

1. Create an application integration in Artifactory.
   Use `https://<CODER_URL>/external-auth/jfrog/callback` (your Coder deployment URL) as the callback URL and `applied-permissions/user` as the scope.

   **JFrog SaaS** (`example.jfrog.io`): Create the integration from the JFrog Platform UI as an administrator:

   1. Go to **Administration** > **General Management** > **Manage Integrations**, or open `https://<JFROG_URL>/ui/admin/configuration/integrations/application` directly.
   1. Select **New Integration**.
   1. Select **External Applications**.
   1. On the **Create New Application Integration** form, set **Application Name** to `Coder`, set **Application Type** to **Custom Integration**, and enter the callback URL.
   1. Select **Generate Client ID & Secret**.

   **Self-hosted (on-premises)**: First register an integration template in your Helm chart `values.yaml`, then create the application integration in the UI, and select that template as the **Application Type**.
   Replace `CODER_URL` with your Coder deployment URL:

   ```yaml
   artifactory:
     enabled: true
     frontend:
     extraEnvironmentVariables:
       - name: JF_FRONTEND_FEATURETOGGLER_ACCESSINTEGRATION
         value: "true"
     access:
     accessConfig:
       integrations-enabled: true
       integration-templates:
         - id: "1"
           name: "CODER"
           redirect-uri: "https://CODER_URL/external-auth/jfrog/callback"
           scope: "applied-permissions/user"
   ```

1. Add a new [external authentication](../external-auth/index.md) to Coder by setting these environment variables in a manner consistent with your Coder deployment.
   Replace `JFROG_URL` with your JFrog Artifactory base URL, and the client ID and secret with the values from step 1:

   ```dotenv
   # JFrog Artifactory External Auth
   CODER_EXTERNAL_AUTH_1_ID="jfrog"
   CODER_EXTERNAL_AUTH_1_TYPE="jfrog"
   CODER_EXTERNAL_AUTH_1_CLIENT_ID="YYYYYYYYYYYYYYY"
   CODER_EXTERNAL_AUTH_1_CLIENT_SECRET="XXXXXXXXXXXXXXXXXXX"
   CODER_EXTERNAL_AUTH_1_DISPLAY_NAME="JFrog Artifactory"
   CODER_EXTERNAL_AUTH_1_DISPLAY_ICON="/icon/jfrog.svg"
   CODER_EXTERNAL_AUTH_1_AUTH_URL="https://JFROG_URL/ui/authorization"
   CODER_EXTERNAL_AUTH_1_SCOPES="applied-permissions/user"
   ```

1. Create or edit a Coder template and use the [JFrog-OAuth](https://registry.coder.com/modules/jfrog-oauth) module to configure the integration:

   ```tf
   module "jfrog" {
     count          = data.coder_workspace.me.start_count
     source         = "registry.coder.com/coder/jfrog-oauth/coder"
     version        = "~> 1.0"
     agent_id       = coder_agent.example.id
     jfrog_url      = "https://example.jfrog.io"
     username_field = "username" # If you are using GitHub to login to both Coder and Artifactory, use username_field = "username"

     package_managers = {
       npm    = ["npm", "@scoped:npm-scoped"]
       go     = ["go", "another-go-repo"]
       pypi   = ["pypi", "extra-index-pypi"]
       docker = ["example-docker-staging.jfrog.io", "example-docker-production.jfrog.io"]
     }
   }
   ```

### JFrog-Token

This module makes use of the [Artifactory terraform
provider](https://registry.terraform.io/providers/jfrog/artifactory/latest/docs) and an admin-scoped token to create
user-scoped tokens for each user by matching their Coder email or username with
Artifactory. This can be used for both SaaS and self-hosted (on-premises)
Artifactory instances.

To set this up, follow these steps:

1. Get a JFrog access token from your Artifactory instance. The token must be an [admin token](https://registry.terraform.io/providers/jfrog/artifactory/latest/docs#access-token) with scope `applied-permissions/admin`.

1. Create or edit a Coder template and use the [JFrog-Token](https://registry.coder.com/modules/jfrog-token) module to configure the integration and pass the admin token. It is recommended to store the token in a sensitive Terraform variable to prevent it from being displayed in plain text in the terraform state:

   ```tf
   variable "artifactory_access_token" {
     type      = string
     sensitive = true
   }

   module "jfrog" {
     source                   = "registry.coder.com/coder/jfrog-token/coder"
     version                  = "~> 1.0"
     agent_id                 = coder_agent.example.id
     jfrog_url                = "https://XXXX.jfrog.io"
     artifactory_access_token = var.artifactory_access_token
     package_managers = {
       npm    = ["npm", "@scoped:npm-scoped"]
       go     = ["go", "another-go-repo"]
       pypi   = ["pypi", "extra-index-pypi"]
       docker = ["example-docker-staging.jfrog.io", "example-docker-production.jfrog.io"]
     }
   }
   ```

> [!NOTE]
> The admin-level access token is used to provision user tokens and is never exposed to developers or stored in workspaces.

If you don't want to use the official modules, you can read through the [example template](../../../examples/jfrog/docker), which uses Docker as the underlying compute. The
same concepts apply to all compute types.

## Air-Gapped Deployments

See the [air-gapped deployments](../templates/extending-templates/modules.md#offline-installations) section for instructions on how to use Coder modules in an offline environment with Artifactory.

## Next Steps

- See the [full example Docker template](../../../examples/jfrog/docker).

- To serve extensions from your own VS Code Marketplace, check out
  [code-marketplace](https://github.com/coder/code-marketplace#artifactory-storage).
