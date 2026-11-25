---
title: Air-gapped Deployments
description: Run Coder in air-gapped, disconnected, or offline environments where all features remain supported.
icon_path: ./images/icons/lan.svg
---

All Coder features are supported in air-gapped / behind firewalls / disconnected / offline.
This is a general comparison.
Keep reading for a full tutorial running Coder air-gapped with Kubernetes or Docker.

| Component                                                          | Public deployments                                                                                                                                                                                                                                                 | Air-gapped deployments                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
|--------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Terraform binary                                                   | By default, Coder downloads Terraform binary from [releases.hashicorp.com](https://releases.hashicorp.com)                                                                                                                                                         | Terraform binary must be included in `PATH` for the VM or container image. [Supported versions](../../../provisioner/terraform/install.go#L27-L28)                                                                                                                                                                                                                                                                                                                                                      |
| Terraform registry                                                 | Coder templates will attempt to download providers from [registry.terraform.io](https://registry.terraform.io) or [custom source addresses](https://developer.hashicorp.com/terraform/language/providers/requirements#source-addresses) specified in each template | [Custom source addresses](https://developer.hashicorp.com/terraform/language/providers/requirements#source-addresses) can be specified in each Coder template, or a custom registry/mirror can be used. More details below                                                                                                                                                                                                                                                                              |
| Session Traversal Utilities for Network Address Translation (STUN) | By default, Coder uses Google's public STUN server for direct workspace connections                                                                                                                                                                                | STUN can be safely [disabled](../../reference/cli/server/index.md#--derp-server-stun-addresses), and users can still connect via [relayed connections](../../admin/networking/index.md#-geo-distribution). Alternatively, you can set a [custom Designated Encrypted Relay for Packets (DERP) server](../../reference/cli/server/index.md#--derp-server-stun-addresses). To keep direct connections without public STUN servers, [self-host a STUN server](../../admin/networking/self-hosted-stun.md). |
| DERP                                                               | By default, Coder's built-in DERP relay can be used, or [Tailscale's public relays](../../admin/networking/index.md#relayed-connections).                                                                                                                          | By default, Coder's built-in DERP relay can be used, or [custom relays](../../admin/networking/index.md#custom-relays).                                                                                                                                                                                                                                                                                                                                                                                 |
| PostgreSQL                                                         | If no [PostgreSQL connection URL](../../reference/cli/server/index.md#--postgres-url) is specified, Coder downloads PostgreSQL from [repo.maven.apache.org](https://repo.maven.apache.org/maven2) the first time it starts                                         | An external database is required, so you must specify a [PostgreSQL connection URL](../../reference/cli/server/index.md#--postgres-url)                                                                                                                                                                                                                                                                                                                                                                 |
| Telemetry                                                          | Telemetry is on by default, and [can be disabled](../../reference/cli/server/index.md#--telemetry)                                                                                                                                                                 | Telemetry [can be disabled](../../reference/cli/server/index.md#--telemetry)                                                                                                                                                                                                                                                                                                                                                                                                                            |
| Update check                                                       | By default, Coder checks for updates from [GitHub releases](https://github.com/coder/coder/releases)                                                                                                                                                               | Update checks [can be disabled](../../reference/cli/server/index.md#--update-check)                                                                                                                                                                                                                                                                                                                                                                                                                     |
| License validation                                                 | License keys are validated locally using cryptographic signatures. No outbound connection to Coder is required                                                                                                                                                     | No changes needed. See [offline license validation](./licensing.md#offline-license-validation)                                                                                                                                                                                                                                                                                                                                                                                                          |
| Trial license                                                      | Starting a trial when you create the first user requests a license from `v2-licensor.coder.com`                                                                                                                                                                    | Do not start a trial. Add a license key that you get from Coder instead, which [validates offline](./licensing.md#offline-license-validation)                                                                                                                                                                                                                                                                                                                                                           |
| Access URL tunnel                                                  | If no [access URL](../../reference/cli/server/index.md#--access-url) is set, Coder creates a public `*.try.coder.app` URL through a tunnel server that Coder hosts                                                                                                 | Set an [access URL](../../admin/setup/index.md#access-url) that users and workspaces can reach on your network                                                                                                                                                                                                                                                                                                                                                                                          |
| GitHub sign-in and external auth                                   | A new deployment that has no GitHub providers configured adds a GitHub sign-in provider and a GitHub external auth provider that Coder manages, and both connect to `github.com`                                                                                   | Turn off the [default sign-in provider](../../reference/cli/server/index.md#--oauth2-github-default-provider-enable) and the [default external auth provider](../../reference/cli/server/index.md#--external-auth-github-default-provider-enable), or configure providers on your network                                                                                                                                                                                                               |
| Template builder                                                   | The template builder resolves module sources from `registry.coder.com`                                                                                                                                                                                             | Turn off the builder or point it to a mirror. Refer to [Template builder](#template-builder)                                                                                                                                                                                                                                                                                                                                                                                                            |

## What templates and workspaces download

Coder can start without internet access and still fail when you create your first template or build your first workspace, because templates download providers, modules, images, and tools.
The starter templates ship inside the `coder` binary, but creating a template from one still runs `terraform init`, which downloads the template's providers and modules.

The table covers the Docker and Kubernetes starter templates.
Check your own templates for the same kinds of downloads.

| Download                                                                                                                            | When it happens                                                       | What fails without it                                                                                                                                                                                  | Offline replacement                                                                                                                                                                                                                                           |
|-------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Terraform providers: `coder/coder`, `kreuzwerker/docker` or `hashicorp/kubernetes`, and `hashicorp/http` for the `jetbrains` module | When you create or push the template                                  | The push fails at `terraform init`                                                                                                                                                                     | Add the providers to a filesystem or network mirror. Refer to [Air-gapped container images](#air-gapped-container-images)                                                                                                                                     |
| Coder Registry modules from `registry.coder.com`, such as `code-server` and `jetbrains` in the Docker starter template              | When you create or push the template                                  | The push fails when Terraform resolves the module source                                                                                                                                               | Mirror the registry or vendor the modules. Refer to [Coder Modules](#coder-modules)                                                                                                                                                                           |
| JetBrains IDE versions, from `data.services.jetbrains.com`                                                                          | When a workspace build includes JetBrains IDEs that the user selected | The workspace build fails                                                                                                                                                                              | Set `ide_config` in the `jetbrains` module to the IDE build numbers you want, which skips the lookup                                                                                                                                                          |
| The workspace image, `codercom/example-base:ubuntu`                                                                                 | When a workspace starts                                               | The workspace doesn't start unless the image is already on the host or node. The Kubernetes starter template sets `image_pull_policy = "Always"`, so its pods pull the image even when the node has it | Push the image to a registry on your network and change `image` in the template. If you load images onto nodes instead, set `image_pull_policy` to `IfNotPresent`                                                                                             |
| code-server, from `code-server.dev` and GitHub                                                                                      | When a workspace starts                                               | The code-server app doesn't start                                                                                                                                                                      | Install code-server in the workspace image. In the Kubernetes starter template, replace the `curl` command in `startup_script`. For the `code-server` module, set `offline = true`, and set `install_prefix` to the directory that contains `bin/code-server` |
| code-server extensions, from `open-vsx.org`                                                                                         | When a workspace starts, if the template lists extensions             | The extensions aren't installed                                                                                                                                                                        | Install the extensions in the workspace image. The `code-server` module doesn't install extensions in offline mode                                                                                                                                            |

## Air-gapped container images

The following instructions walk you through how to build a custom control plane image for Docker or Kubernetes

First, build and push a container image extending our official image with the following:

- CLI config (.tfrc) for Terraform referring to [external mirror](https://www.terraform.io/cli/config/config-file#explicit-installation-method-configuration)
- [Terraform Providers](https://registry.terraform.io) for templates
  - These could also be specified via a volume mount (Docker) or [network mirror](https://www.terraform.io/internals/provider-network-mirror-protocol).
    See below for details.

> [!NOTE]
> Coder includes the latest [supported version](../../../provisioner/terraform/install.go#L27-L28) of Terraform in the official Docker images.
> If you need to bundle a different version of terraform, you can do so by customizing the image.

Here's an example Dockerfile:

```dockerfile
FROM ghcr.io/coder/coder:latest

USER root

RUN apk add curl unzip

# Create directory for the Terraform CLI (and assets)
RUN mkdir -p /opt/terraform

# Terraform is already included in the official Coder image.
# See ../../scripts/Dockerfile.base#L15
# If you need to install a different version of Terraform, you can do so here.
# The below step is optional if you wish to keep the existing version.
# See ../../provisioner/terraform/install.go#L27-L28
# for supported Terraform versions.
ARG TERRAFORM_VERSION=1.17.2
RUN apk update && \
    curl -LOs https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip \
    && unzip -o terraform_${TERRAFORM_VERSION}_linux_amd64.zip \
    && mv terraform /opt/terraform \
    && rm terraform_${TERRAFORM_VERSION}_linux_amd64.zip
ENV PATH=/opt/terraform:${PATH}

# Additionally, a Terraform mirror needs to be configured
# to download the Terraform providers used in Coder templates.
# There are two options:

# Option 1) Use a filesystem mirror.
#  We can seed this at build-time or by mounting a volume to
#  /opt/terraform/plugins in the container.
#  https://developer.hashicorp.com/terraform/cli/config/config-file#filesystem_mirror
#  Be sure to add all the providers you use in your templates to /opt/terraform/plugins

RUN mkdir -p /opt/terraform/plugins/registry.terraform.io
ADD filesystem-mirror-example.tfrc /home/coder/.terraformrc

# Optionally, we can "seed" the filesystem mirror with common providers.
# Comment out lines 40-49 if you plan on only using a volume or network mirror:
WORKDIR /opt/terraform/plugins/registry.terraform.io
ARG CODER_PROVIDER_VERSION=2.2.0
RUN echo "Adding coder/coder v${CODER_PROVIDER_VERSION}" \
    && mkdir -p coder/coder && cd coder/coder \
    && curl -LOs https://github.com/coder/terraform-provider-coder/releases/download/v${CODER_PROVIDER_VERSION}/terraform-provider-coder_${CODER_PROVIDER_VERSION}_linux_amd64.zip
ARG DOCKER_PROVIDER_VERSION=3.0.2
RUN echo "Adding kreuzwerker/docker v${DOCKER_PROVIDER_VERSION}" \
    && mkdir -p kreuzwerker/docker && cd kreuzwerker/docker \
    && curl -LOs https://github.com/kreuzwerker/terraform-provider-docker/releases/download/v${DOCKER_PROVIDER_VERSION}/terraform-provider-docker_${DOCKER_PROVIDER_VERSION}_linux_amd64.zip
ARG KUBERNETES_PROVIDER_VERSION=2.36.0
RUN echo "Adding kubernetes/kubernetes v${KUBERNETES_PROVIDER_VERSION}" \
    && mkdir -p hashicorp/kubernetes && cd hashicorp/kubernetes \
    && curl -LOs https://releases.hashicorp.com/terraform-provider-kubernetes/${KUBERNETES_PROVIDER_VERSION}/terraform-provider-kubernetes_${KUBERNETES_PROVIDER_VERSION}_linux_amd64.zip
ARG AWS_PROVIDER_VERSION=5.89.0
RUN echo "Adding aws/aws v${AWS_PROVIDER_VERSION}" \
    && mkdir -p aws/aws && cd aws/aws \
    && curl -LOs https://releases.hashicorp.com/terraform-provider-aws/${AWS_PROVIDER_VERSION}/terraform-provider-aws_${AWS_PROVIDER_VERSION}_linux_amd64.zip

RUN chown -R coder:coder /home/coder/.terraform*
WORKDIR /home/coder

# Option 2) Use a network mirror.
#  https://developer.hashicorp.com/terraform/cli/config/config-file#network_mirror
#  Be sure uncomment line 60 and edit network-mirror-example.tfrc to
#  specify the HTTPS base URL of your mirror.

# ADD network-mirror-example.tfrc /home/coder/.terraformrc

USER coder

# Use the .terraformrc file to inform Terraform of the locally installed providers.
ENV TF_CLI_CONFIG_FILE=/home/coder/.terraformrc
```

> [!NOTE]
> If you are bundling Terraform providers into your Coder image, be sure the provider version matches any templates or [example templates](../../../examples/templates) you intend to use.
> After you change the bundled provider versions, push new versions of your templates and update workspaces to them.

```tf
# filesystem-mirror-example.tfrc
provider_installation {
  filesystem_mirror {
    path = "/opt/terraform/plugins"
  }
}
```

```tf
# network-mirror-example.tfrc
provider_installation {
  network_mirror {
    url = "https://terraform.example.com/providers/"
  }
}
```

<div class="tabs">

### Docker

Follow our [docker-compose](../server/docker.md#install-coder-via-docker-compose) documentation and modify the docker-compose file to specify your custom Coder image.
Additionally, you can add a volume mount to add providers to the filesystem mirror without re-building the image.

First, create an empty plugins directory:

```sh
mkdir $HOME/plugins
```

Next, add a volume mount to compose.yaml:

```sh
vim compose.yaml
```

```yaml
# compose.yaml
services:
  coder:
    image: registry.example.com/coder:latest
    volumes:
      - ./plugins:/opt/terraform/plugins
    # ...
    environment:
      CODER_TELEMETRY_ENABLE: "false" # Disable telemetry
      CODER_BLOCK_DIRECT: "true" # force SSH traffic through control plane's DERP proxy
      CODER_DERP_SERVER_STUN_ADDRESSES: "disable" # Only use relayed connections
      CODER_UPDATE_CHECK: "false" # Disable automatic update checks
  database:
    image: registry.example.com/postgres:17
    # ...
```

The [terraform providers mirror](https://www.terraform.io/cli/commands/providers/mirror) command can be used to download the required plugins for a Coder template.
This can be uploaded into the `plugins` directory on your offline server.

### Kubernetes

We publish the Helm chart for download on the [Coder downloads page](https://coder.com/downloads) and on [GitHub Releases](https://github.com/coder/coder/releases/latest).
Follow our [Kubernetes](../server/kubernetes/index.md) documentation and modify the Helm values to specify your custom Coder image.

```yaml
# values.yaml
coder:
  image:
    repo: "registry.example.com/coder"
    tag: "latest"
  env:
    # Disable telemetry
    - name: "CODER_TELEMETRY_ENABLE"
      value: "false"
    # Disable automatic update checks
    - name: "CODER_UPDATE_CHECK"
      value: "false"
    # force SSH traffic through control plane's DERP proxy
    - name: CODER_BLOCK_DIRECT
      value: "true"
    # Only use relayed connections
    - name: "CODER_DERP_SERVER_STUN_ADDRESSES"
      value: "disable"
    # You must set up an external PostgreSQL database
    - name: "CODER_PG_CONNECTION_URL"
      value: ""
# ...
```

For a tutorial that stages every artifact and installs Coder on a single-node K3s cluster, refer to [Install Coder on K3s in an air-gapped environment](../server/kubernetes/k3s-air-gapped.md).

</div>

## Air-gapped docs

Coder also provides air-gapped documentation in case you want to host it on your own server.
The docs are exported as static files that you can host on any web server, as demonstrated in the example below:

1. Go to the release page.
   In this case, we want to use the [latest version](https://github.com/coder/coder/releases/latest).
2. Download the documentation files from the "Assets" section.
   It is named as `coder_docs_<version>.tgz`.
3. Extract the file and move its contents to your server folder.
4. If you are using NodeJS, you can execute the following command: `cd docs && npx http-server .`
5. Set the [CODER_DOCS_URL](../../reference/cli/server/index.md#--docs-url) environment variable to use the URL of your hosted docs.

   This way, the Coder UI will reference the documentation from your specified URL.

With these steps, you'll have the Coder documentation hosted on your server and accessible for your team to use.

## Template builder

The template builder requires outbound access to `registry.coder.com` for `terraform init` to resolve module sources at template composition time.

For fully air-gapped deployments, disable the template builder:

```sh
CODER_DISABLE_TEMPLATE_BUILDER=true
```

When the builder is disabled, template creation falls back to the standard upload and CLI workflows.
The **New Template** button on the **Templates** page links to the starter templates page instead of the builder.

For deployments using a self-hosted module registry mirror, set the registry URL instead of disabling the builder:

```sh
CODER_TEMPLATE_BUILDER_REGISTRY_URL=registry.internal.example.com
```

This makes the builder generate module source paths pointing at your mirror rather than `registry.coder.com`.

The value is a bare host, optionally with a port (for example, `mirror.internal:8443`).
A leading `http(s)://` scheme and trailing slash are stripped, and a path, query, fragment, or credentials is rejected at server start.

For a complete walkthrough of setting up the mirror, see [Mirror the Coder Registry with JFrog Artifactory](./registry-mirror.md).

## Coder Modules

To use Coder modules in offline installations, you can either:

- [Mirror the Coder Registry with JFrog Artifactory](./registry-mirror.md) (recommended)
- [Manually publish modules to Artifactory or use a private git repository](../../templates/extending-templates/modules.md#offline-installations)

## Firewall exceptions

In a network that blocks most internet traffic, you can allow the addresses that templates and workspaces use instead of replacing each download.
For the control plane's own connections, such as telemetry, the update check, and the access URL tunnel, use the settings in the table at the start of this page.

Allow these addresses from the control plane and any external provisioners:

- `registry.terraform.io`, and the hosts it sends provider downloads to, such as `releases.hashicorp.com` and `github.com`, to create and push templates.
- `registry.coder.com`, for modules and the template builder.
- `data.services.jetbrains.com`, when a workspace build looks up JetBrains IDE versions.
- `v2-licensor.coder.com`, only to start a trial when you create the first user.

Allow these addresses from workspaces:

- `code-server.dev` and `github.com`, to install code-server when a workspace starts.
- `open-vsx.org`, to install code-server extensions.

The hosts or nodes that run workspaces also need access to the registry that hosts your workspace images, such as Docker Hub.

## JetBrains IDEs

Gateway, JetBrains' remote development product that works with Coder, [has documented offline deployment steps.](../../templates/extending-templates/jetbrains-airgapped.md)

## Microsoft VS Code Remote - SSH

Installation of the [Visual Studio Code Remote - SSH extension](https://code.visualstudio.com/docs/remote/ssh) (for connecting a local VS Code to a remote Coder workspace) requires that your local machine has outbound HTTPS (port 443) connectivity to:

- update.code.visualstudio.com
- vscode.blob.core.windows.net
- \*.vo.msecnd.net

## Next steps

- [Create your first template](../../templates/template-from-scratch.md)
- [Control plane configuration](../../admin/setup/index.md)
