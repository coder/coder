# Custom Agents

> [!NOTE]
>
> This functionality is in early access and subject to change. Do not run in
> production as it is unstable. Instead, deploy these changes into a demo or
> staging environment.
>
> Join our [Discord channel](https://discord.gg/coder) or
> [contact us](https://coder.com/contact) to get help or share feedback.

Custom agents beyond the ones listed in the [Coder registry](https://registry.coder.com/modules?tag=agent) can be used with Coder.

## Prerequisites

- A Coder deployment with v2.21 or later
- A [Coder workspace / template](./create-template.md)
- A custom agent that supports Model Context Protocol (MCP)

## Getting Started

Coder uses the [MCP protocol](https://modelcontextprotocol.io/introduction) to report activity back to the Coder control plane. From there, activity is displayed in the Coder dashboard.

First, your template will need a [coder_app](https://registry.terraform.io/providers/coder/coder/latest/docs/resources/app) for the agent. This can be a web app or command run in the terminal and ideally gives the user a UI to interact with or view more details about the agent.

From there, the agent can run the MCP server with the `coder exp mcp server` command. You will need to set the `CODER_MCP_APP_STATUS_SLUG` environment variable to match the slug in the coder_app resource.

## Example

Inside a Coder workspace, run the following commands:

```sh
coder login # be sure to be authenticated with the Coder CLI
export CODER_MCP_APP_STATUS_SLUG=my-agent # needs to be the same as the slug in the coder_app resource

# Use your own agent's logic and syntax here:
any-custom-agent configure-mcp --name "coder" --command "coder exp mcp server"
```

This will start the MCP server and report activity back to the Coder control plane on behalf of the coder_app resource.

> See the [Goose module](https://github.com/coder/modules/blob/main/goose/main.tf) source code for a real world example.

## Contributing

We welcome contributions for various agents via the [Coder registry](https://registry.coder.com/modules?tag=agent)!

See our [contributing guide](https://github.com/coder/modules/blob/main/CONTRIBUTING.md) for more information.
