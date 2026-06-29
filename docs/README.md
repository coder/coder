# About

<!-- Warning for docs contributors: The first route in manifest.json must be titled "About" for the static landing page to work correctly. -->

Coder is a self-hosted platform for running AI coding agents and cloud development environments on infrastructure you control.
It works with any cloud, IDE, OS, Git provider, and IDP.

![Coder platform showing templates and a running workspace](./images/hero-image.png)

## Coder Workspaces

[Coder Workspaces](./user-guides/index.md) are cloud development environments defined with Terraform, connected through a secure WireGuard tunnel, and automatically shut down when not in use.
Agents and developers share the same workspace infrastructure.

- **Defined in Terraform**: Templates describe the infrastructure for each workspace, from EC2 VMs and Kubernetes Pods to Docker containers.
- **Any architecture and OS**: Support ARM and x86-64 across Windows, Linux, and macOS from a single deployment.
- **Managed by admins**: Platform teams create and maintain templates that enforce approved images, resource limits, and security policies.
- **Accessed from any IDE**: Connect through VS Code, JetBrains, Cursor, a web terminal, remote desktop, or SSH.
- **Automatic shutdown**: Idle workspaces stop automatically to reduce cloud spend, and restart in seconds when needed.

## Coder Agents

[Coder Agents](./ai-coder/index.md) is a native AI coding agent built into Coder.
The agent loop runs in the Coder control plane on your infrastructure, not in the workspace and not in a vendor's cloud.
Developers interact with agents through the web UI or the REST API for programmatic and CI-driven workflows.

- **Self-hosted agent loop**: The control plane handles planning, model calls, and tool dispatch. Workspaces have zero AI awareness.
- **No API keys in workspaces**: LLM credentials stay in the control plane.
- **Any model**: Anthropic, OpenAI, Google, Bedrock, or self-hosted endpoints. Switching is a configuration change.
- **Governance and cost controls**: Centralized model approval, per-user spend limits, and audit logging.
- **Open source and inspectable**: The full platform is available to audit and extend.

![Coder Agents chat interface with git diff sidebar](./images/agents-hero-image.png)

## IDE support

![IDE icons](./images/ide-icons.svg)

Coder workspaces support desktop IDEs, browser-based editors, and terminal access.

### Desktop IDEs

Connect your local IDE to a remote workspace:

- [VS Code](./user-guides/workspace-access/vscode.md)
- [Cursor](./user-guides/workspace-access/cursor.md)
- [Windsurf](./user-guides/workspace-access/windsurf.md)
- [Zed](./user-guides/workspace-access/zed.md)
- [JetBrains Gateway](./user-guides/workspace-access/jetbrains/gateway.md)

### Browser-based IDEs

Run a full editor in the browser without a local IDE installation:

- [code-server](./user-guides/workspace-access/code-server.md) (VS Code in the browser)
- [Jupyter](./user-guides/workspace-access/port-forwarding.md) via port forwarding

### Terminal and other access

- [Coder Desktop](./user-guides/desktop/index.md) (macOS and Windows)
- [Web terminal](./user-guides/workspace-access/web-terminal.md)
- [SSH](./user-guides/workspace-access/index.md#ssh)
- [Emacs TRAMP](./user-guides/workspace-access/emacs-tramp.md)

## How Coder works

Coder workspaces are represented with Terraform, but you do not need to know Terraform to get started.
The [Coder Registry](https://registry.coder.com/templates) provides production-ready templates for AWS EC2, Azure, Google Cloud, Kubernetes, and other providers.

![Providers and compute environments](./images/providers-compute.png)

<small>Providers and compute environments</small>

Workspaces can include more than just compute.
Terraform can add storage buckets, secrets, sidecars, and [other resources](https://developer.hashicorp.com/terraform/tutorials).

Refer to the [templates documentation](./admin/templates/index.md) for details.

## Pricing

Coder is free and open source under the [GNU Affero General Public License v3.0](../LICENSE).
All developer productivity features are included in the open source version.
A [Premium license](https://coder.com/pricing#compare-plans) is available for enhanced support and custom deployments.

## Get started

**New to Coder:**

- [Quickstart](./tutorials/quickstart.md)

**Platform administrators:**

- [Install Coder](./install/index.md)
- [Manage templates](./admin/templates/index.md)
- [Manage users](./admin/users/index.md)
- [Set up Coder Agents](./ai-coder/index.md)

**Developers:**

- [Access your workspace](./user-guides/workspace-access/index.md)
- [Use Coder Agents](./ai-coder/index.md)
- [Dev Containers](./user-guides/devcontainers/index.md)
