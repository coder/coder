# Pre-install JetBrains Gateway in a template

For a faster JetBrains Gateway experience, pre-install the IDEs backend in your template.

> [!NOTE]
> This guide only talks about installing the IDEs backend. For a complete guide on setting up JetBrains Gateway with client IDEs, refer to the [JetBrains Gateway air-gapped guide](./jetbrains-airgapped.md).

## Install the Client Downloader

Install the JetBrains Client Downloader binary:

```shell
wget https://download.jetbrains.com/idea/code-with-me/backend/jetbrains-clients-downloader-linux-x86_64-1867.tar.gz && \
tar -xzvf jetbrains-clients-downloader-linux-x86_64-1867.tar.gz
rm jetbrains-clients-downloader-linux-x86_64-1867.tar.gz
```

## Install Gateway backend

```shell
mkdir ~/JetBrains
./jetbrains-clients-downloader-linux-x86_64-1867/bin/jetbrains-clients-downloader --products-filter <product-code> --build-filter <build-number> --platforms-filter linux-x64 --download-backends ~/JetBrains
```

For example, to install the build `243.26053.27` of IntelliJ IDEA:

```shell
./jetbrains-clients-downloader-linux-x86_64-1867/bin/jetbrains-clients-downloader --products-filter IU --build-filter 243.26053.27 --platforms-filter linux-x64 --download-backends ~/JetBrains
tar -xzvf ~/JetBrains/backends/IU/*.tar.gz -C ~/JetBrains/backends/IU
rm -rf ~/JetBrains/backends/IU/*.tar.gz
```

## Register the Gateway backend

Add the following command to your template's `startup_script`:

```shell
~/JetBrains/backends/IU/ideaIU-243.26053.27/bin/remote-dev-server.sh registerBackendLocationForGateway
```

## Configure JetBrains Gateway Module

If you are using our [jetbrains-gateway](https://registry.coder.com/modules/jetbrains-gateway) module, you can configure it by adding the following snippet to your template:

```tf
module "jetbrains_gateway" {
  count          = data.coder_workspace.me.start_count
  source         = "registry.coder.com/modules/jetbrains-gateway/coder"
  version        = "1.0.28"
  agent_id       = coder_agent.main.id
  folder         = "/home/coder/example"
  jetbrains_ides = ["IU"]
  default        = "IU"
  latest         = false
  jetbrains_ide_versions = {
    "IU" = {
      build_number = "243.26053.27"
      version      = "2024.3"
    }
  }
}

resource "coder_agent" "main" {
	...
	startup_script = <<-EOF
	~/JetBrains/backends/IU/ideaIU-243.26053.27/bin/remote-dev-server.sh registerBackendLocationForGateway
	EOF
}
```

## Dockerfile example

If you are using Docker based workspaces, you can add the command to your Dockerfile:

```dockerfile
FROM ubuntu

# Combine all apt operations in a single RUN command
# Install only necessary packages
# Clean up apt cache in the same layer
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
    curl \
    git \
    golang \
    sudo \
    vim \
    wget \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Create user in a single layer
ARG USER=coder
RUN useradd --groups sudo --no-create-home --shell /bin/bash ${USER} \
    && echo "${USER} ALL=(ALL) NOPASSWD:ALL" >/etc/sudoers.d/${USER} \
    && chmod 0440 /etc/sudoers.d/${USER}

USER ${USER}
WORKDIR /home/${USER}

# Install JetBrains Gateway in a single RUN command to reduce layers
# Download, extract, use, and clean up in the same layer
RUN mkdir -p ~/JetBrains \
    && wget -q https://download.jetbrains.com/idea/code-with-me/backend/jetbrains-clients-downloader-linux-x86_64-1867.tar.gz -P /tmp \
    && tar -xzf /tmp/jetbrains-clients-downloader-linux-x86_64-1867.tar.gz -C /tmp \
    && /tmp/jetbrains-clients-downloader-linux-x86_64-1867/bin/jetbrains-clients-downloader \
       --products-filter IU \
       --build-filter 243.26053.27 \
       --platforms-filter linux-x64 \
       --download-backends ~/JetBrains \
    && tar -xzf ~/JetBrains/backends/IU/*.tar.gz -C ~/JetBrains/backends/IU \
    && rm -f ~/JetBrains/backends/IU/*.tar.gz \
    && rm -rf /tmp/jetbrains-clients-downloader-linux-x86_64-1867* \
    && rm -rf /tmp/*.tar.gz
```

## Next steps

- [Pre install the Client IDEs](./jetbrains-airgapped.md#1-deploy-the-server-and-install-the-client-downloader)
