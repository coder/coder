# Working with Dev Containers

The dev container integration appears in your Coder dashboard, providing a
visual representation of the running environment:

![Dev container integration in Coder dashboard](../../images/user-guides/devcontainers/devcontainer-agent-ports.png)

This page assumes you have a template with the [dev containers integration](./index.md) ready.

## SSH Access

You can connect directly to your dev container.

1. Run `coder config-ssh` to configure your SSH local client:

   ```shell
    coder config-ssh
    ```

1. SSH to your workspace:

    ```shell
    ssh <agent>.<workspace-name>.me.coder
    ```

   Example:

    ```shell
    ssh devcontainer.myworkspace.me.coder
    ```

### Coder CLI

```shell
coder ssh <workspace-name>
```

Coder CLI connects to your dev container based on your workspace configuration.

### Web Terminal Access

Once your workspace and dev container are running, you can use the **Terminal**
in the Coder workspace to execute commands directly inside the dev container.

![Coder web terminal with dev container](../../images/user-guides/devcontainers/devcontainer-web-terminal.png)

## IDE Integration (VS Code)

To open your dev container directly in VS Code, select "Open in VS Code Desktop" from the Coder dashboard.

Alternatively, you can use the CLI:

```shell
coder open vscode <workspace-name>
```

Coder CLI connects to your dev container based on your workspace configuration.

## Port Forwarding

Coder supports port forwarding for dev containers through the following mechanisms:

1. **Defined Ports**: Ports defined in your `devcontainer.json` file via the [`appPort`](https://containers.dev/implementors/json_reference/#image-specific) property.

1. **Dynamic Ports**: For ports not defined in your `devcontainer.json`, you can use the Coder CLI to forward them:

   ```shell
   coder port-forward <workspace-name> --tcp <local-port>:<container-port>
   ```

For example, with this `devcontainer.json` configuration:

```json
{
    "appPort": ["8080:8080", "4000:3000"]
}
```

You can access these ports directly through your browser via the Coder dashboard, or forward them to your local machine:

```shell
coder port-forward <workspace-name> --tcp 8080,4000
```

This forwards port 8080 (local) → 8080 (container) and port 4000 (local) → 3000 (container).

## Dev Container Features

Dev container features allow you to enhance your development environment with pre-configured tooling.

Coder supports the standard [dev container features specification](https://containers.dev/implementors/features/), allowing you to use any compatible features in your `devcontainer.json` file.

### Example: Add code-server

Coder maintains a [repository of features](https://github.com/coder/devcontainer-features) designed specifically for Coder environments.

To add code-server (VS Code in the browser), add this to your `devcontainer.json`:

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

After rebuilding your container, code-server will be available on the configured port.

### Using Multiple Features

You can combine multiple features in a single `devcontainer.json`:

```json
{
    "features": {
        "ghcr.io/devcontainers/features/docker-in-docker:2": {},
        "ghcr.io/devcontainers/features/python:1": {
            "version": "3.10"
        },
        "ghcr.io/coder/devcontainer-features/code-server:1": {
            "port": 13337
        }
    }
}
```

## Rebuilding Dev Containers

When you make changes to your `devcontainer.json` file, you need to rebuild the container for those changes to take effect.

From the Coder dashboard, click the rebuild button on the dev container to apply your changes.

You can also restart your workspace, which will rebuild containers when it restarts.
