# Custom Agents

> [!WARNING]
> Starting June 2, 2026, Coder Tasks will move to a 12-month Extended Support Release (ESR) for Premium customers.
>
> Tasks will be removed from new Coder releases beginning with v2.37 (September 1, 2026) and will only be available via the ESR during the support period.
>
> We recommend transitioning to [Coder Agents](./agents/index.md), the long-term replacement.

Custom agents beyond the ones listed in the [Coder registry](https://registry.coder.com/modules?search=tag%3Aagent) can be used with Coder Tasks.

## Prerequisites

- A Coder deployment with v2.21 or later
- A [Coder workspace / template](../admin/templates/creating-templates.md)
- A custom agent that supports Model Context Protocol (MCP)

## Getting Started

Coder uses the [MCP protocol](https://modelcontextprotocol.io/introduction) to report activity back to the Coder control plane. From there, activity is displayed in the Coder dashboard.

First, your template will need a [coder_app](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/app) for the agent. This can be a web app or command run in the terminal and ideally gives the user a UI to interact with or view more details about the agent.

From there, the agent can run the MCP server with the `coder exp mcp server` command. You will need to set the `CODER_MCP_APP_STATUS_SLUG` environment variable to match the slug in the coder_app resource. `CODER_AGENT_TOKEN` must also be set, but will be present inside a Coder workspace.

## Example

Inside a Coder workspace, run the following commands:

```sh
coder login
export CODER_MCP_APP_STATUS_SLUG=my-agent

# Use your own agent's logic and syntax here:
any-custom-agent configure-mcp --name "coder" --command "coder exp mcp server"
```

This will start the MCP server and report activity back to the Coder control plane on behalf of the coder_app resource.

> [!NOTE]
> See [this version of the Goose module](https://github.com/coder/registry/blob/release/coder/goose/v1.3.0/registry/coder/modules/goose/main.tf) source code for a real-world example of configuring reporting via MCP. Note that in addition to setting up reporting, you'll need to make your template [compatible with Tasks](./tasks.md#option-2-create-or-duplicate-your-own-template), which is not shown in the example.

## Pause and resume

Custom agents can support task pause and resume by enabling state
persistence on the agentapi module. Set `enable_state_persistence = true`
so that AgentAPI saves and restores conversation history across pause and
resume cycles:

```hcl
module "agentapi" {
  source                   = "registry.coder.com/coder/agentapi/coder"
  version                  = ">= 2.2.0"
  agent_id                 = coder_agent.main.id
  enable_state_persistence = true
  # ...
}
```

Your template also needs persistent storage and a sufficient graceful
shutdown timeout. See [Task lifecycle](./tasks-lifecycle.md) for the full
requirements.

## Contributing

We welcome contributions for various agents via the [Coder registry](https://registry.coder.com/modules?tag=agent)! See our [contributing guide](https://github.com/coder/registry/blob/main/CONTRIBUTING.md) for more information.
