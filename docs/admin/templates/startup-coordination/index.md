# Workspace Startup Coordination

> [!NOTE]
> This feature is experimental and may change without notice in future releases.

When workspaces start, scripts often need to run in a specific order.
For example, an IDE or coding agent might need the repository cloned
before it can start. Without explicit coordination, these scripts can
race against each other, leading to startup failures and inconsistent
workspace states.

Coder's workspace startup coordination feature lets you declare
dependencies between startup scripts and ensure they run in the correct order.
This eliminates race conditions and makes workspace startup predictable and
reliable.

## Why use this?

Simply placing all of your workspace initialization logic in a single script works, but leads to slow workspace startup times.
Breaking this out into multiple independent `coder_script` resources improves startup times by allowing the scripts to run in parallel.
However, this can lead to intermittent failures between dependent scripts due to timing issues.
Up until now, template authors have had to rely on manual coordination methods (for example, touching a file upon completion).
The goal of startup script coordination is to provide a single reliable source of truth for coordination between workspace startup scripts.

## Quick Start

To start using workspace startup coordination, follow these steps:

1. Set the environment variable `CODER_AGENT_SOCKET_SERVER_ENABLED=true` in your template to enable the agent socket server. The environment variable *must* be readable to the agent process. For example, in a template using the `kreuzwerker/docker` provider:

  ```terraform
  resource "docker_container" "workspace" {
    image = "codercom/enterprise-base:ubuntu"
    env = [
      "CODER_AGENT_TOKEN=${coder_agent.main.token}",
      "CODER_AGENT_SOCKET_SERVER_ENABLED=true",
    ]
  }
  ```
2. Add calls to `coder exp sync (start|complete)` in your startup scripts where required:
	```bash
		trap 'coder exp sync complete my-script' EXIT
	  coder exp sync want my-script my-other-script
	  coder exp sync start my-script
    # Existing startup logic
	```

For more information, refer to the [usage documentation](./usage.md) or [troubleshooting documentation](./troubleshooting.md).
