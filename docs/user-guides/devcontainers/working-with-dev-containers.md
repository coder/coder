# Working with Dev Containers

The dev container integration appears in your Coder dashboard, providing a
visual representation of the running environment:

![Dev container integration in Coder dashboard](../../images/user-guides/devcontainers/devcontainer-agent-ports.png)

This page assumes you have a [dev containers integration](./index.md) ready.

## SSH Access

You can SSH into your dev container directly using the Coder CLI:

```console
coder ssh --container keen_dijkstra my-workspace
```

> [!NOTE]
>
> SSH access is not yet compatible with the `coder config-ssh` command for use
> with OpenSSH. You would need to manually modify your SSH config to include the
> `--container` flag in the `ProxyCommand`.

## Web Terminal Access

Once your workspace and dev container are running, you can use the web terminal
in the Coder interface to execute commands directly inside the dev container.

![Coder web terminal with dev container](../../images/user-guides/devcontainers/devcontainer-web-terminal.png)

## IDE Integration (VS Code)

You can open your dev container directly in VS Code by:

1. Selecting "Open in VS Code Desktop" from the Coder web interface
2. Using the Coder CLI with the container flag:

```console
coder open vscode --container keen_dijkstra my-workspace
```

While optimized for VS Code, other IDEs with dev containers support may also
work.

## Port Forwarding

Coder automatically forwards any port declared in `appPort`, `forwardPorts`,
or exposed by `docker-compose.yml`.
Use the dashboard to open a forwarded port, or the CLI:

```console
coder port-forward my-workspace --tcp 8080,4000
```

If you need a port that isn’t declared, pass it explicitly to `coder port-forward`.

## Dev Container Features

You can use standard dev container features in your `devcontainer.json` file.
Coder also maintains a
[repository of features](https://github.com/coder/devcontainer-features) to
enhance your development experience.

Currently available features include [code-server](https://github.com/coder/devcontainer-features/blob/main/src/code-server).

To use the code-server feature, add the following to your `devcontainer.json`:

```json
{
    "features": {
        "ghcr.io/coder/devcontainer-features/code-server:1": {
            "port": 13337,
            "host": "0.0.0.0"
        }
    },
    "appPort": ["13337:13337"]
}
```

> [!NOTE]
>
> Remember to include the port in the `appPort` section to ensure proper port
> forwarding.
