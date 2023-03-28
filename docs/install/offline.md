# Offline Deployments

All Coder features are supported in offline / behind firewalls / in air-gapped environments. However, some changes to your configuration are necessary.

> This is a general comparison. Keep reading for a full tutorial running Coder offline with Kubernetes or Docker.

|                    | Public deployments                                                                                                                                                                                                                                                 | Offline deployments                                                                                                                                                                                                                                                   |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Terraform binary   | By default, Coder downloads Terraform binary from [releases.hashicorp.com](https://releases.hashicorp.com)                                                                                                                                                         | Terraform binary must be included in `PATH` for the VM or container image. [Supported versions](https://github.com/coder/coder/blob/main/provisioner/terraform/install.go#L23-L24)                                                                                    |
| Terraform registry | Coder templates will attempt to download providers from [registry.terraform.io](https://registry.terraform.io) or [custom source addresses](https://developer.hashicorp.com/terraform/language/providers/requirements#source-addresses) specified in each template | [Custom source addresses](https://developer.hashicorp.com/terraform/language/providers/requirements#source-addresses) can be specified in each Coder template, or a custom registry/mirror can be used. More details below                                            |
| STUN               | By default, Coder uses Google's public STUN server for direct workspace connections                                                                                                                                                                                | STUN can be safely [disabled](../cli/server.md#--derp-server-stun-addresses), users can still connect via [relayed connections](../networking.md#-geo-distribution). Alternatively, you can set a [custom DERP server](../cli/server.md#--derp-server-stun-addresses) |
| DERP               | By default, Coder's built-in DERP relay can be used, or [Tailscale's public relays](../networking.md#relayed-connections).                                                                                                                                         | By default, Coder's built-in DERP relay can be used, or [custom relays](../networking.md#custom-relays).                                                                                                                                                              |
| PostgreSQL         | If no [PostgreSQL connection URL](../cli/server.md#--postgres-url) is specified, Coder will download Postgres from [repo1.maven.org](https://repo1.maven.org)                                                                                                      | An external database is required, you must specify a [PostgreSQL connection URL](../cli/server.md#--postgres-url)                                                                                                                                                     |
| Telemetry          | Telemetry is on by default, and [can be disabled](../cli/server.md#--telemetry)                                                                                                                                                                                    | Telemetry [can be disabled](../cli/server.md#--telemetry)                                                                                                                                                                                                             |
| Update check       | By default, Coder checks for updates from [GitHub releases](https:/github.com/coder/coder/releases)                                                                                                                                                                | Update checks [can be disabled](../cli/server.md#--update-check)                                                                                                                                                                                                      |

## Offline container images

The following instructions walk you through how to build a custom Coder server image for Docker or Kubernetes

First, build and push a container image extending our official image with the following:

- CLI config (.tfrc) for Terraform referring to [external mirror](https://www.terraform.io/cli/config/config-file#explicit-installation-method-configuration)
- [Terraform Providers](https://registry.terraform.io) for templates
  - These could also be specified via a volume mount (Docker) or [network mirror](https://www.terraform.io/internals/provider-network-mirror-protocol). See below for details.

> Note: Coder includes the latest [supported version](https://github.com/coder/coder/blob/main/provisioner/terraform/install.go#L23-L24) of Terraform in the official Docker images.
> If you need to bundle a different version of terraform, you can do so by customizing the image.

Here's an example Dockerfile:

```Dockerfile
FROM ghcr.io/coder/coder:latest

USER root

RUN apk add curl unzip

# Create directory for the Terraform CLI (and assets)
RUN mkdir -p /opt/terraform

# Terraform is already included in the official Coder image.
# See https://github.com/coder/coder/blob/main/scripts/Dockerfile.base#L15
# If you need to install a different version of Terraform, you can do so here.
# The below step is optional if you wish to keep the existing version.
# See https://github.com/coder/coder/blob/main/provisioner/terraform/install.go#L23-L24
# for supported Terraform versions.
ARG TERRAFORM_VERSION=1.3.0
RUN apk update && \
    apk del terraform && \
    curl -LOs https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip \
    && unzip -o terraform_${TERRAFORM_VERSION}_linux_amd64.zip \
    && mv terraform /opt/terraform \
    && rm terraform_${TERRAFORM_VERSION}_linux_amd64.zip
ENV PATH=/opt/terraform:${PATH}

# Additionally, a Terraform mirror needs to be configured
# to download the Terraform providers used in Coder templates.
# There are two options:

# Option 1) Use a filesystem mirror. We can seed this at build-time
#    or by mounting a volume to /opt/terraform/plugins in the container.
#    https://developer.hashicorp.com/terraform/cli/config/config-file#filesystem_mirror
#    Be sure to add all the providers you use in your templates to /opt/terraform/plugins

RUN mkdir -p /opt/terraform/plugins
ADD filesystem-mirror-example.tfrc /opt/terraform/config.tfrc

# Optionally, we can "seed" the filesystem mirror with common providers.
# Comment out lines 40-49 if you plan on only using a volume or network mirror:
RUN mkdir -p /opt/terraform/plugins/registry.terraform.io
WORKDIR /opt/terraform/plugins/registry.terraform.io
ARG CODER_PROVIDER_VERSION=0.6.10
RUN echo "Adding coder/coder v${CODER_PROVIDER_VERSION}" \
    && mkdir -p coder/coder && cd coder/coder \
    && curl -LOs https://github.com/coder/terraform-provider-coder/releases/download/v${CODER_PROVIDER_VERSION}/terraform-provider-coder_${CODER_PROVIDER_VERSION}_linux_amd64.zip
ARG DOCKER_PROVIDER_VERSION=3.0.1
RUN echo "Adding kreuzwerker/docker v${DOCKER_PROVIDER_VERSION}" \
    && mkdir -p kreuzwerker/docker && cd kreuzwerker/docker \
    && curl -LOs https://github.com/kreuzwerker/terraform-provider-docker/releases/download/v${DOCKER_PROVIDER_VERSION}/terraform-provider-docker_${DOCKER_PROVIDER_VERSION}_linux_amd64.zip
ARG KUBERNETES_PROVIDER_VERSION=2.18.1
RUN echo "Adding kubernetes/kubernetes v${KUBERNETES_PROVIDER_VERSION}" \
    && mkdir -p kubernetes/kubernetes && cd kubernetes/kubernetes \
    && curl -LOs https://releases.hashicorp.com/terraform-provider-kubernetes/${KUBERNETES_PROVIDER_VERSION}/terraform-provider-kubernetes_${KUBERNETES_PROVIDER_VERSION}_linux_amd64.zip
ARG AWS_PROVIDER_VERSION=4.59.0
RUN echo "Adding aws/aws v${AWS_PROVIDER_VERSION}" \
    && mkdir -p aws/aws && cd aws/aws \
    && curl -LOs https://releases.hashicorp.com/terraform-provider-aws/${AWS_PROVIDER_VERSION}/terraform-provider-aws_${AWS_PROVIDER_VERSION}_linux_amd64.zip

RUN chown -R coder:coder /opt/terraform/plugins
WORKDIR /home/coder

# Option 2) Use a network mirror.
#    https://developer.hashicorp.com/terraform/cli/config/config-file#network_mirror
#    Be sure uncomment line 60 and edit network-mirror-example.tfrc to
#    specify the HTTPS base URL of your mirror.

# ADD network-mirror-example.tfrc /opt/terraform/config.tfrc

USER coder

# Use the tfrc file to inform
ENV TF_CLI_CONFIG_FILE=/opt/terraform/config.tfrc
```

> If you are bundling Terraform providers into your Coder image, be sure the
> provider version matches any templates or [example templates](https://github.com/coder/coder/tree/main/examples/templates) you intend to use.

```hcl
# filesystem-mirror-example.tfrc
provider_installation {
  filesystem_mirror {
    path = "/opt/terraform/plugins"
  }
}
```

```hcl
# network-mirror-example.tfrc
provider_installation {
  network_mirror {
    url = "https://terraform.example.com/providers/"
  }
}
```

## Run offline via Docker

Follow our [docker-compose](./docker.md#run-coder-with-docker-compose) documentation and modify the docker-compose file to specify your custom Coder image. Additionally, you can add a volume mount to add providers to the filesystem mirror without re-building the image.

First, make a create an empty plugins directory:

```console
mkdir $HOME/plugins
```

Next, add a volume mount to docker-compose.yaml:

```console
vim docker-compose.yaml
```

```yaml
# docker-compose.yaml
version: "3.9"
services:
  coder:
    image: registry.example.com/coder:latest
    volumes:
      - ./plugins:/opt/terraform/plugins
    # ...
  environment:
    CODER_TELEMETRY_ENABLE: "false" # Disable telemetry
    CODER_DERP_SERVER_STUN_ADDRESSES: "" # Only use relayed connections
    CODER_UPDATE_CHECK: "false" # Disable automatic update checks
  database:
    image: registry.example.com/postgres:13
    # ...
```

> The [terraform providers mirror](https://www.terraform.io/cli/commands/providers/mirror) command can be used to download the required plugins for a Coder template. This can be uploaded into the `plugins` directory on your offline server.

## Run offline via Kubernetes

We publish the Helm chart for download on [GitHub Releases](https://github.com/coder/coder/releases/latest). Follow our [Kubernetes](./kubernetes.md) documentation and modify the Helm values to specify your custom Coder image.

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
    # Only use relayed connections
    - name: "CODER_DERP_SERVER_STUN_ADDRESSES"
      value: ""
    # You must set up an external PostgreSQL database
    - name: "CODER_PG_CONNECTION_URL"
      value: ""
# ...
```
