# Pre-install JetBrains Gateway in a template

For a faster JetBrains Gateway experience, pre-install the IDE in your template.

## Deploy the server and install the Client Downloader

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
tar -xzvf ~/JetBrains/backends/IU/ideaIU-2024.3.5.tar.gz -C ~/JetBrains/backends/IU
rm -rf ~/JetBrains/backends/IU/ideaIU-2024.3.5.tar.gz
```

## Register the Gateway backend

Run the following script in the downloaded JetBrains IDE backend directory to configure the Gateway backend:

```shell
~/JetBrains/backends/IU/ideaIU-2024.3.5/bin/remote-dev-server.sh registerBackendLocationForGateway
```

## Configure JetBrains Gateway Module

If you are using our [jetbrains-gateway](https://registry.coder.com/modules/jetbrains-gateway) module, you can configure it by adding the following snippet to your template:

```tf
module "jetbrains_gateway" {
  count          = data.coder_workspace.me.start_count
  source         = "registry.coder.com/modules/jetbrains-gateway/coder"
  version        = "1.0.28"
  agent_id       = coder_agent.example.id
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
}```
