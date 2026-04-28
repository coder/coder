# Using AI Gateway Without Workspaces

AI Gateway does not require users to work inside Coder workspaces. Any AI
coding tool running on a developer's local machine (IDE extensions, CLI tools,
desktop applications) can route traffic through AI Gateway as long as the user
can authenticate with a Coder API token.

This page describes what administrators need to set up and what end users
need to do to connect their local tools.

## Admin setup

### 1. Enable AI Gateway

AI Gateway must be enabled on the Coder deployment. See [Setup](./setup.md)
for full instructions.

```sh
coder server --aibridge-enabled=true
```

### 2. Configure upstream providers

Configure at least one upstream LLM provider so AI Gateway has somewhere to
forward requests. See
[Configure Providers](./setup.md#configure-providers) for the full list of
options.

```sh
# Example: Anthropic
export CODER_AIBRIDGE_ANTHROPIC_KEY="sk-ant-..."

# Example: OpenAI
export CODER_AIBRIDGE_OPENAI_KEY="sk-..."
```

### 3. Create Coder accounts for external users

Every request to AI Gateway is authenticated with a Coder API token, so each
external user must have a Coder account. Users do not need workspace
permissions; they only need the ability to generate an API token.

#### Users without individual Coder accounts

Some organizations have developers who need AI Gateway access but should not
have full Coder accounts (for example, contractors or teams that do not use
Coder workspaces at all). In this case, an admin can create a
[service account](../../admin/users/headless-auth.md) and distribute its
API token to those users.

> [!NOTE]
> Using a shared service account token is a temporary workaround.
> A dedicated authentication-only account tier is planned to give each
> external user their own identity for auditing and access control.
> Until then, a service account provides a practical path for teams
> that need AI Gateway governance without individual Coder logins.

To create a service account and generate a token for it:

```sh
# Create the service account (requires User Admin role or above).
coder users create \
  --username="ai-gateway-external" \
  --service-account

# Generate a long-lived API token for the service account.
coder tokens create \
  --name=ai-gateway \
  --lifetime=2160h \
  --user="ai-gateway-external"
```

Distribute the resulting token to the external users along with the
connection details from the next step.

When every user shares a single token, AI Gateway audit logs attribute
all traffic to the service account rather than to individual developers.
There are two ways to get per-user attribution:

- **One token per user from the same service account.** Generate a
  separate named token for each developer (`--name=alice`,
  `--name=bob`, etc.). Each token is logged as a distinct API key,
  so audit records can be traced back to an individual even though
  they all belong to the same account.
- **One service account per user or group.** Create a dedicated
  service account for each developer or team. This provides a
  distinct user identity in audit logs but requires more accounts
  to manage.

### 4. Share connection details

Provide users with the following information:

- **Coder deployment URL**: for example `https://coder.example.com`
- **AI Gateway base URLs**:
  - OpenAI-compatible: `https://coder.example.com/api/v2/aibridge/openai/v1`
  - Anthropic-compatible: `https://coder.example.com/api/v2/aibridge/anthropic`
- **How to generate an API token**: link users to the Coder dashboard or
  provide the CLI command (see below).

### 5. Optional: disable BYOK

By default AI Gateway allows users to bring their own LLM API keys. To
require all traffic to use the centralized org key, disable BYOK:

```sh
coder server --aibridge-allow-byok=false
```

## User setup

### 1. Generate a long-lived API token

Create a token from the Coder dashboard or CLI. This token authenticates
you with AI Gateway and does not require a workspace.

<div class="tabs">

#### Dashboard

1. Navigate to `https://coder.example.com/settings/account`.
1. Open the **Tokens** page in the sidebar.
1. Click **Create token**, give it a name, and save the value.

#### CLI

```sh
coder tokens create --name=ai-gateway --lifetime=720h
```

</div>

See [Sessions and API tokens](../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)
for more details on token management.

### 2. Configure your AI tool

Point your tool at the AI Gateway base URL and set the Coder API token as
the authentication credential. The exact settings vary by tool.

<div class="tabs">

#### Claude Code

```bash
export ANTHROPIC_BASE_URL="https://coder.example.com/api/v2/aibridge/anthropic"
export ANTHROPIC_AUTH_TOKEN="<your-coder-api-token>"
```

#### Codex CLI

In `~/.codex/config.toml`:

```toml
model_provider = "aibridge"

[model_providers.aibridge]
name = "AI Bridge"
base_url = "https://coder.example.com/api/v2/aibridge/openai/v1"
env_key = "OPENAI_API_KEY"
wire_api = "responses"
```

Then set the environment variable:

```bash
export OPENAI_API_KEY="<your-coder-api-token>"
```

#### Other tools

For tools not listed here, the general pattern is:

1. Set the base URL to `https://coder.example.com/api/v2/aibridge/openai/v1`
   (OpenAI-compatible) or
   `https://coder.example.com/api/v2/aibridge/anthropic`
   (Anthropic-compatible).
2. Set the API key to your Coder API token.

See the [individual client pages](./clients/index.md#all-supported-clients)
for tool-specific instructions.

</div>

Replace `coder.example.com` with your Coder deployment URL in all examples.

### 3. Verify the connection

Run a request through your tool. If authentication succeeds, the request
appears in the [AI Gateway audit log](./audit.md). If you receive a `401`
error, confirm your API token is valid. If you receive a `403`, BYOK may
be disabled and the tool may be sending a personal LLM key instead of the
Coder token.
