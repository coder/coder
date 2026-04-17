# vscode-coder template

This template is for developing the
[coder/vscode-coder](https://github.com/coder/vscode-coder) VS Code extension.

## Personalization

The template includes a `personalize` module that runs your `~/personalize`
file if it exists.

## Testing

The workspace comes with Playwright Chromium, GTK libraries, xauth, and a
D-Bus daemon pre-configured for running tests headlessly — the same way CI
does.

| Task                  | Command                                |
| --------------------- | -------------------------------------- |
| Unit tests            | `pnpm test`                            |
| Extension tests       | `pnpm test:extension`                  |
| Webview tests         | `pnpm test:webview`                    |
| Integration tests     | `xvfb-run -a pnpm test:integration`    |
| Type check            | `pnpm typecheck`                       |
| Lint                  | `pnpm lint`                            |
| Format check          | `pnpm format:check`                    |
| Build                 | `pnpm build`                           |
| Package               | `pnpm package`                         |

Integration tests require a virtual framebuffer because they launch a real
VS Code instance. Use `xvfb-run -a` to run them headlessly, matching CI
behavior.

## Hosting

Coder dogfoods on a single Teraswitch bare metal machine for best-in-class
cost-to-performance. Workspaces run as Docker containers with regional
Tailscale endpoints for Pittsburgh, Falkenstein, Sydney, and Cape Town.

## Provisioner Configuration

The dogfood coderd box runs an SSH tunnel to the Docker host's socket,
mounted at `/var/run/dogfood-docker.sock`. The tunnel runs in a screen
session named `forward` and is owned by root.
