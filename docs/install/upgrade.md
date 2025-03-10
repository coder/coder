# Upgrade

This article walks you through how to upgrade your Coder server.

> [!CAUTION]
> Prior to upgrading a production Coder deployment, take a database snapshot since
> Coder does not support rollbacks.

To upgrade your Coder server, simply reinstall Coder using your original method
of [install](../install).

## Via install.sh

If you installed Coder using the `install.sh` script, re-run the below command
on the host:

```shell
curl -L https://coder.com/install.sh | sh
```

The script will unpack the new `coder` binary version over the one currently
installed. Next, you can restart Coder with the following commands (if running
it as a system service):

```shell
systemctl daemon-reload
systemctl restart coder
```

## Via docker-compose

If you installed using `docker-compose`, run the below command to upgrade the
Coder container:

```shell
docker-compose pull coder && docker-compose up -d coder
```

## Via Kubernetes

See
[Upgrading Coder via Helm](../install/kubernetes.md#upgrading-coder-via-helm).

## Via Windows

Download the latest Windows installer or binary from
[GitHub releases](https://github.com/coder/coder/releases/latest), or upgrade
from Winget.

```pwsh
winget install Coder.Coder
```
