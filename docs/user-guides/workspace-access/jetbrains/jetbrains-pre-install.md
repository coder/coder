# Pre-install JetBrains Gateway in a template

For a faster JetBrains Gateway experience, pre-install the IDE in your template.

## Deploy the server and install the Client Downloader

Install the JetBrains Client Downloader binary:

```shell
wget https://download.jetbrains.com/idea/code-with-me/backend/jetbrains-clients-downloader-linux-x86_64-1867.tar.gz && \
tar -xzvf jetbrains-clients-downloader-linux-x86_64-1867.tar.gz
```

## Install Gateway backend

```shell
mkdir ~/backends
./jetbrains-clients-downloader-linux-x86_64-1867/bin/jetbrains-clients-downloader --products-filter <product-code> --build-filter <build-number> --platforms-filter linux-x64 --download-backends ~/backends
```

## Register the Gateway backend

Run the following script in the JetBrains IDE directory to point the default Gateway directory to the IDE
directory:

```shell
cd /opt/idea/bin
./remote-dev-server.sh registerBackendLocationForGateway
```
