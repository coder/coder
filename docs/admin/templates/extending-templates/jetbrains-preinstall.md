# Pre-install JetBrains IDEs in your template

For a faster first time connection with JetBrains IDEs, pre-install the IDEs backend in your template.

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
~/JetBrains/*/bin/remote-dev-server.sh registerBackendLocationForGateway
```

## Configure JetBrains Gateway Module

If you are using our [jetbrains-gateway](https://registry.coder.com/modules/coder/jetbrains-gateway) module, you can configure it by adding the following snippet to your template:

```tf
module "jetbrains_gateway" {
  count          = data.coder_workspace.me.start_count
  source         = "registry.coder.com/modules/jetbrains-gateway/coder"
  version        = "1.0.29"
  agent_id       = coder_agent.main.id
  folder         = "/home/coder/example"
  jetbrains_ides = ["IU"]
  default        = "IU"
  latest         = false
  jetbrains_ide_versions = {
    "IU" = {
      build_number = "251.25410.129"
      version      = "2025.1"
    }
  }
}

resource "coder_agent" "main" {
    ...
    startup_script = <<-EOF
    ~/JetBrains/*/bin/remote-dev-server.sh registerBackendLocationForGateway
    EOF
}
```

## Dockerfile example

If you are using Docker based workspaces, you can add the command to your Dockerfile:

```dockerfile
FROM codercom/enterprise-base:ubuntu

# JetBrains IDE installation (configurable)
ARG IDE_CODE=IU
ARG IDE_VERSION=2025.1

# Fetch and install IDE dynamically
RUN mkdir -p ~/JetBrains \
    && IDE_URL=$(curl -s "https://data.services.jetbrains.com/products/releases?code=${IDE_CODE}&majorVersion=${IDE_VERSION}&latest=true" | jq -r ".${IDE_CODE}[0].downloads.linux.link") \
    && IDE_NAME=$(curl -s "https://data.services.jetbrains.com/products/releases?code=${IDE_CODE}&majorVersion=${IDE_VERSION}&latest=true" | jq -r ".${IDE_CODE}[0].name") \
    && echo "Installing ${IDE_NAME}..." \
    && wget -q ${IDE_URL} -P /tmp \
    && tar -xzf /tmp/$(basename ${IDE_URL}) -C ~/JetBrains \
    && rm -f /tmp/$(basename ${IDE_URL}) \
    && echo "${IDE_NAME} installed successfully"
```

## Next steps

- [Pre-install the Client IDEs](./jetbrains-airgapped.md#1-deploy-the-server-and-install-the-client-downloader)
