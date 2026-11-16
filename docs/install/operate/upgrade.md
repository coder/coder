---
title: Upgrade
---

This article describes how to upgrade your Coder deployment.

> [!CAUTION]
> Prior to upgrading a production Coder deployment, take a database snapshot since Coder does not support rollbacks.

For upgrade recommendations and troubleshooting, see [Upgrading Best Practices](./upgrade/best-practices.md).

## Reinstall Coder to upgrade

To upgrade your Coder deployment, reinstall Coder using your original method of [install](../index.md).

### Coder install script

1. If you installed Coder using the `install.sh` script, re-run the below command on the host:

   ```sh
   curl -fsSL https://coder.com/install.sh | sh
   ```

1. If you're running Coder as a system service, you can restart it with `systemctl`:

   ```sh
   systemctl daemon-reload
   systemctl restart coder
   ```

> [!NOTE]
> In Coder v2.34.0 and later, an option set to an empty value in `/etc/coder.d/coder.env` no longer keeps its default.
> The deb, RPM, and apk packages for Coder v2.39 and earlier shipped a `CODER_HTTP_ADDRESS=` line with no value, and an upgrade keeps your existing file.
> If the service fails to start with `TLS is disabled. Enable with --tls-enable or specify a HTTP address`, add a `#` at the start of that line, and then restart the service.

### Other upgrade methods

<div class="tabs">

### docker-compose

If you installed using `docker-compose`, run the below command to upgrade the Coder container:

```sh
docker-compose pull coder && docker-compose up -d coder
```

### Kubernetes

See [Upgrade Coder via Helm](../server/kubernetes/index.md#upgrade-coder-via-helm).

### Coder AMI on AWS

1. Run the Coder installation script on the host:

   ```sh
   curl -fsSL https://coder.com/install.sh | sh
   ```

   The script will unpack the new `coder` binary version over the one currently installed.

1. Restart the Coder system process with `systemctl`:

   ```sh
   systemctl daemon-reload
   systemctl restart coder
   ```

### Windows

Download the latest Windows installer or binary from the [Coder downloads page](https://coder.com/downloads), or upgrade from Winget.

```ps1
winget install Coder.Coder
```

</div>
