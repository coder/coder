# Port Forwarding

Port forwarding lets developers securely access processes on their Coder
workspace from a local machine. A common use case is testing web
applications in a browser.

There are three ways to forward ports in Coder:

- The `coder port-forward` command
- Dashboard
- SSH

The `coder port-forward` command is generally more performant.

## coder port-forward

Forward the remote TCP port `8080` to local port `8000` like so:

```console
coder port-forward myworkspace --tcp 8000:8080
```

For more examples, see `coder port-forward --help`.

## Dashboard

> To enable port forwarding via the dashboard, Coder must be configured with a
> [wildcard access URL](../admin/configure.md#wildcard-access-url).

Use the "Port forward" button in the dashboard to access ports
running on your workspace.

![Port forwarding in the UI](../images/port-forward-dashboard.png)

## SSH

First, [configure SSH](../ides.md#ssh-configuration) on your
local machine. Then, use `ssh` to forward like so:

```console
ssh -L 8080:localhost:8000 coder.myworkspace
```

You can read more on SSH port forwarding [here](https://www.ssh.com/academy/ssh/tunneling/example).
