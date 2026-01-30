# Client Configuration

Once AI Bridge is setup on your deployment, the AI coding tools used by your users will need to be configured to route requests via AI Bridge.

## Base URLs

Most AI coding tools allow the "base URL" to be customized. In other words, when a request is made to OpenAI's API from your coding tool, the API endpoint such as [`/v1/chat/completions`](https://platform.openai.com/docs/api-reference/chat) will be appended to the configured base. Therefore, instead of the default base URL of `https://api.openai.com/v1`, you'll need to set it to `https://coder.example.com/api/v2/aibridge/openai/v1`.

The exact configuration method varies by client — some use environment variables, others use configuration files or UI settings:

- **OpenAI-compatible clients**: Set the base URL (commonly via the `OPENAI_BASE_URL` environment variable) to `https://coder.example.com/api/v2/aibridge/openai/v1`
- **Anthropic-compatible clients**: Set the base URL (commonly via the `ANTHROPIC_BASE_URL` environment variable) to `https://coder.example.com/api/v2/aibridge/anthropic`

Replace `coder.example.com` with your actual Coder deployment URL.

## Authentication

Instead of distributing provider-specific API keys (OpenAI/Anthropic keys) to users, they authenticate to AI Bridge using their **Coder session token** or **API key**:

- **OpenAI clients**: Users set `OPENAI_API_KEY` to their Coder session token or API key
- **Anthropic clients**: Users set `ANTHROPIC_API_KEY` to their Coder session token or API key

> [!NOTE]
> Only Coder-issued tokens are accepted at this time.
> Provider-specific API keys (such as OpenAI or Anthropic keys) will not work with AI Bridge.

Again, the exact environment variable or setting naming may differ from tool to tool; consult your tool's documentation.
### Retrieving your session token

If you're logged in with the Coder CLI, you can retrieve your current session
token using [`coder login token`](../../../reference/cli/login_token.md):

```sh
export ANTHROPIC_API_KEY=$(coder login token)
export ANTHROPIC_BASE_URL="https://coder.example.com/api/v2/aibridge/anthropic"
```

## Configuring In-Workspace Tools

AI coding tools running inside a Coder workspace, such as IDE extensions, can be configured to use AI Bridge.

While users can manually configure these tools with a long-lived API key, template admins can provide a more seamless experience by pre-configuring them. Admins can automatically inject the user's session token with `data.coder_workspace_owner.me.session_token` and the AI Bridge base URL into the workspace environment.

In this example, Claude code respects these environment variables and will route all requests via AI Bridge.

This is the fastest way to bring existing agents like Roo Code, Cursor, or Claude Code into compliance without adopting Coder Tasks.

```hcl
data "coder_workspace_owner" "me" {}

data "coder_workspace" "me" {}

resource "coder_agent" "dev" {
	arch = "amd64"
	os   = "linux"
	dir  = local.repo_dir
	env = {
		ANTHROPIC_BASE_URL : "${data.coder_workspace.me.access_url}/api/v2/aibridge/anthropic",
		ANTHROPIC_AUTH_TOKEN : data.coder_workspace_owner.me.session_token
	}
	... # other agent configuration
}
```

### Using Coder Tasks

Agents like Claude Code can be configured to route through AI Bridge in any template by pre-configuring the agent with the session token. [Coder Tasks](../../tasks.md) is particularly useful for this pattern, providing a framework for agents to complete background development operations autonomously. To route agents through AI Bridge in a Coder Tasks template, pre-configure it to install Claude Code and configure it with the session token:

```hcl
data "coder_workspace_owner" "me" {}

data "coder_workspace" "me" {}

data "coder_task" "me" {}

resource "coder_agent" "dev" {
	arch = "amd64"
	os   = "linux"
	dir  = local.repo_dir
	env = {
		ANTHROPIC_BASE_URL : "${data.coder_workspace.me.access_url}/api/v2/aibridge/anthropic",
		ANTHROPIC_AUTH_TOKEN : data.coder_workspace_owner.me.session_token
	}
	... # other agent configuration
}

# See https://registry.coder.com/modules/coder/claude-code for more information
module "claude-code" {
	count               = data.coder_task.me.enabled ? data.coder_workspace.me.start_count : 0
	source              = "dev.registry.coder.com/coder/claude-code/coder"
	version             = ">= 4.0.0"
	agent_id            = coder_agent.dev.id
	workdir             = "/home/coder/project"
	claude_api_key      = data.coder_workspace_owner.me.session_token # Use the Coder session token to authenticate with AI Bridge
	ai_prompt           = data.coder_task.me.prompt
	... # other claude-code configuration
}

# The coder_ai_task resource associates the task to the app.
resource "coder_ai_task" "task" {
	count  = data.coder_task.me.enabled ? data.coder_workspace.me.start_count : 0
	app_id = module.claude-code[0].task_app_id
}
```

## External and Desktop Clients

You can also configure AI tools running outside of a Coder workspace, such as local IDE extensions or desktop applications, to connect to AI Bridge.

The configuration is the same: point the tool to the AI Bridge [base URL](#base-urls) and use a Coder API key for authentication.

Users can generate a long-lived API key from the Coder UI or CLI. Follow the instructions at [Sessions and API tokens](../../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself) to create one.


## Compatibility

The table below shows tested AI clients and their compatibility with AI Bridge.

| Client | OpenAI | Anthropic | Notes |
|---|---|---|---|
<<<<<<< HEAD
| [Claude Code](./claude-code.md) | - | ✅ | Native Anthropic support. |
| [Cline](./cline.md) | ✅ | ✅ | Supports both provider types. |
| [Codex CLI](./codex.md) | ⚠️ | - | Requires v0.58.0. |
| [Copilot](./copilot.md) | ✅ | ❌ | Requires pre-release extension. |
| Cursor | ❌ | ❌ | See [broken OpenAI support](https://forum.cursor.com/t/requests-are-sent-to-incorrect-endpoint-when-using-base-url-override/144894) and no Anthropic override. |
| [Goose](./goose.md) | ✅ | ✅ | CLI and Desktop supported. |
| [JetBrains](./jetbrains.md) | ✅ | ✅ | Via "Bring Your Own Key". |
| [Kilo Code](./kilo-code.md) | ✅ | ✅ | Similar to Roo/Cline. |
| [OpenCode](./opencode.md) | ✅ | ✅ | Via `api_base`. |
| [Roo Code](./roo-code.md) | ✅ | ✅ | Supports both provider types. |
| [Zed](./zed.md) | ✅ | ✅ | Configured via `settings.json`. |
=======
| [Claude Code](./claude-code.md) | - | ✅ | Works out of the box and can be preconfigured in templates. |
| [Claude Code (VS Code)](./claude-code.md) | - | ✅ | May require signing in once; afterwards respects workspace environment variables. |
| [Cursor](./cursor.md) | ✅ | ❌ | Supports `v1/responses` endpoints. |
| [Roo Code](./roo-code.md) | ✅ | ✅ | Use the **OpenAI Compatible** provider with the legacy format to avoid `/v1/responses`. |
| [Codex CLI](./codex.md) | ⚠️ | N/A | • Use v0.58.0 (`npm install -g @openai/codex@0.58.0`). Newer versions have a [bug](https://github.com/openai/codex/issues/8107) breaking the request payload. <br/>• `gpt-5-codex` support is [in progress](https://github.com/coder/aibridge/issues/16). |
| [GitHub Copilot](./copilot.md) | ✅ | ❌ | Requires the pre-release extension. Anthropic endpoints are not supported. |
| [Goose](./goose.md) | ✅ | ✅ | Support via custom provider configuration. |
| [Goose Desktop](./goose.md) | ❓ | ✅ | |
| [Cline](./cline.md) | ✅ | ✅ | Similar to Roo Code. |
| [OpenCode](./opencode.md) | ✅ | ✅ | Support via `api_base` configuration. |
| WindSurf | ❌ | ❌ | No option to override the base URL. |
| Sourcegraph Amp | ❌ | ❌ | No option to override the base URL. |
| Kiro | ❌ | ❌ | No option to override the base URL. |
| [Copilot CLI](https://github.com/github/copilot-cli/issues/104) | ❌ | ❌ | No support for custom base URLs and uses a `GITHUB_TOKEN` for authentication. |
| [Kilo Code](./kilo-code.md) | ✅ | ✅ | Similar to Roo Code. |
| Gemini CLI | ❌ | ❌ | Not supported yet. |
| [Amazon Q CLI](https://aws.amazon.com/q/) | ❌ | ❌ | Limited to Amazon Q subscriptions; no custom endpoint support. |
| [Zed](./zed.md) | ✅ | ✅ | Configure via `settings.json`. |
| [JetBrains](./jetbrains.md) | ✅ | ✅ | Use "Bring Your Own Key" (BYOK) with AI Assistant plugin. |
>>>>>>> e437368cb (docs: refine AI bridge client documentation structure and content)

*Legend: ✅ works, ⚠️ limited support, ❌ not supported, - not applicable.*

### Unsupported Clients

The following clients currently do not support custom base URLs or are otherwise incompatible:

*   **WindSurf**: No option to override base URL.
*   **Sourcegraph Amp**: No option to override base URL.
*   **Kiro**: No option to override base URL.
*   **Copilot CLI**: No custom base URL support; uses `GITHUB_TOKEN`.
*   **Gemini CLI**: Not supported.
*   **Amazon Q CLI**: No custom endpoint support.
