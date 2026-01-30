As the AI landscape is evolving, we are working to ensure Coder remains a secure
platform for running AI agents just as it is for other cloud development
environments.

## Use Trusted Models

Most agents can be configured to either use a local LLM (e.g.
llama3), an agent proxy (e.g. OpenRouter), or a Cloud-Provided LLM (e.g. AWS
Bedrock). Research which models you are comfortable with and configure your
Coder templates to use those.

## Set up Firewalls and Proxies

Many enterprises run Coder workspaces behind a firewall or a proxy to prevent
threats or bad actors. These same protections can be used to ensure AI agents do
not access or upload sensitive information.

## Separate API keys and scopes for agents

Many agents require API keys to access external services. It is recommended to
create a separate API key for your agent with the minimum permissions required.
This will likely involve editing your template for Agents to set different scopes or tokens from the standard one.

Additional guidance and tooling is coming in future releases of Coder.

## Set Up Agent Boundary

Agent Boundary is a process-level "agent firewall" that lets you restrict and audit what AI agents can access within Coder workspaces. To learn more about this feature, see [Agent Boundary](./agent-boundary/).
