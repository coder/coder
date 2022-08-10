# Upgrade

This article walks you through how to upgrade your Coder server.

To upgrade your Coder server, simply reinstall Coder using your original method
of [install](../install.md).

<blockquote class="danger">
  <p>
  Prior to upgrading Coder, we _highly_ recommend taking a database snapshot, as
  Coder does not support rollbacks.
  </p>
</blockquote>

## Via install.sh

If you installed Coder using the `install.sh` script, simply re-run the below
command on the host:

```console
curl -L https://coder.com/install.sh | sh
```

The script will unpack the new `coder` binary version over the one currently installed.
Once this is run, you can restart Coder with the following command (if running
it as a system service):

```console
systemctl restart coder
```

## Via docker-compose

If you installed using the `docker-compose`, run the below command to upgrade the
Coder container:

```console
docker-compose pull coder && docker-compose up coder -d
```
