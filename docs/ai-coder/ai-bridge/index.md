# AI Bridge

![AI bridge diagram](../../images/aibridge/aibridge_diagram.png)

AI Bridge is a smart proxy for AI. It acts as a man-in-the-middle between your users' coding agents / IDEs
and providers like OpenAI and Anthropic. By intercepting all the AI traffic between these clients and
the upstream APIs, AI Bridge can record user prompts, token usage, and tool invocations.

AI Bridge solves 3 key problems:

1. **Centralized authn/z management**: no more issuing & managing API tokens for OpenAI/Anthropic usage.
   Users use their Coder session or API tokens to authenticate with `coderd` (Coder control plane), and
   `coderd` securely communicates with the upstream APIs on their behalf.
2. **Auditing and attribution**: all interactions with AI services, whether autonomous or human-initiated,
   will be audited and attributed back to a user.
3. **Centralized MCP administration**: define a set of approved MCP servers and tools which your users may
   use.

## When to use AI Bridge

As the library of LLMs and their associated tools grow, administrators are pressured to provide auditing, measure adoption, provide tools through MCP, and track token spend. Disparate SAAS platforms provide _some_ of these for _some_ tools, but there is no centralized, secure solution for these challenges.

If you are an administrator or devops leader looking to:

- Measure AI tooling adoption across teams or projects
- Provide an LLM audit trail to security administrators
- Manage token spend in a central dashboard
- Investigate opportunities for AI automation
- Uncover the high-leverage use cases from experienced engineers

We advise trying AI Bridge as self-hosted proxy to monitor LLM usage agnostically across AI powered IDEs like Cursor and headless agents like Claude Code.

