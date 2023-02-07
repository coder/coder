# Offline Deployments

Coder can run in offline / air-gapped environments.

## Building & push a custom Coder image

First, build and push a container image extending our official image with the following:

- Terraform [(supported versions)](https://github.com/coder/coder/blob/main/provisioner/terraform/install.go#L23-L24)
- CLI config (.tfrc) for Terraform referring to [external mirror](https://www.terraform.io/cli/config/config-file#explicit-installation-method-configuration)
- [Terraform Providers](https://registry.terraform.io) for templates
  - These could also be specified via a volume mount (Docker) or [network mirror](https://www.terraform.io/internals/provider-network-mirror-protocol). See below for details.

Here's an example:

```Dockerfile
# Dockerfile
FROM ghcr.io/coder/coder:latest

USER root

RUN apk add curl unzip

# Create directory for the Terraform CLI (and assets)
RUN mkdir -p /opt/terraform

# In order to run Coder airgapped or within private networks,
# Terraform has to be bundled into the image in PATH or /opt.
#
# See https://github.com/coder/coder/blob/main/provisioner/terraform/install.go#L23-L24
# for supported Terraform versions.
ARG TERRAFORM_VERSION=1.3.0
RUN curl -LOs https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip \
    && unzip -o terraform_${TERRAFORM_VERSION}_linux_amd64.zip \
    && mv terraform /opt/terraform \
    && rm terraform_${TERRAFORM_VERSION}_linux_amd64.zip
ENV PATH=/opt/terraform:${PATH}

# Additionally, a Terraform mirror needs to be configured
# to download the Terraform providers used in Coder templates.
#
# There are two options:

# Option 1) Use a filesystem mirror. We can seed this at build-time
#    or by mounting a volume to /opt/terraform/plugins in the container.
#    https://developer.hashicorp.com/terraform/cli/config/config-file#filesystem_mirror
#
#    Be sure to add all the providers you use in your templates to /opt/terraform/plugins

RUN mkdir -p /opt/terraform/plugins
ADD filesystem-mirror-example.tfrc /opt/terraform/config.tfrc

# Optionally, we can "seed" the filesystem mirror with common providers.
# Coder and Docker. Comment out lines 40-49 if you plan on only using a
# volume or network mirror:
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
# ...
```
