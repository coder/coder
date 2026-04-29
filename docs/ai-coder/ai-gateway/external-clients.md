# Using AI Gateway Without Workspaces

AI Gateway does not require users to work inside Coder workspaces. AI coding
tools running on a developer's local machine (IDE extensions, CLI tools,
desktop applications) can route traffic through AI Gateway as long as the
tool supports AI Gateway and the user
can authenticate with a Coder API token. Not every client supports
customizing the API base URL; see the
[compatibility table](https://coder.com/docs/ai-coder/ai-gateway/clients#compatibility) for the current list.

This page describes what administrators need to set up and what end users
need to do to connect their local tools.

## Admin setup

### Prerequisites

Before sharing AI Gateway with external users, complete the
[AI Gateway setup](./setup.md): enable the feature flag and configure at
least one upstream provider.

### 1. Create Coder accounts for external users

Every request to AI Gateway is authenticated with a Coder API token, so each
external user must have a Coder account. Users do not need workspace
permissions; they only need the ability to generate an API token.

#### Users without individual Coder accounts

Some organizations have developers who need AI Gateway access but do not
have individual Coder accounts (for example, contractors or teams that do
not use Coder workspaces). In this case, an admin can create a
[service account](../../admin/users/headless-auth.md) and distribute its
API token to those users.

> [!NOTE]
> Using a shared service account token is a temporary workaround.
> A dedicated authentication-only account tier is planned to give each
> external user their own identity for auditing and access control.

When every user shares a single token, AI Gateway audit logs attribute
all traffic to the service account. To get per-user attribution, generate
a separate named token for each developer or create a dedicated service
account per user or group. See
[Headless Authentication](../../admin/users/headless-auth.md) for details
on creating service accounts and generating tokens.

### 2. Share connection details

Provide users with the following information:

- **Coder deployment URL**: for example `https://coder.example.com`
- **AI Gateway base URLs**:
  - OpenAI-compatible: `https://coder.example.com/api/v2/aibridge/openai/v1`
  - Anthropic-compatible: `https://coder.example.com/api/v2/aibridge/anthropic`
- **How to generate an API token**: link users to the Coder dashboard or
  provide the CLI command (see below).

### 3. Optional: disable BYOK

By default AI Gateway allows users to bring their own LLM API keys. To
require all traffic to use the centralized org key, disable BYOK. See
[Enabling or disabling BYOK](./clients/index.md#enabling-or-disabling-byok)
for the server flag.

## User setup

### 1. Generate a long-lived API token

Create a Coder API token to authenticate with AI Gateway. If you do not
have a Coder account, ask your admin to provision a
[service account](#users-without-individual-coder-accounts) and provide
you with a token.

See
[Sessions and API tokens](../../admin/users/sessions-tokens.md#generate-a-long-lived-api-token-on-behalf-of-yourself)
for instructions on creating tokens via the dashboard or CLI.

### 2. Configure your AI tool

Point your tool at the AI Gateway base URL and set the Coder API token as
the authentication credential. The general pattern is:

1. Set the base URL to the appropriate AI Gateway endpoint.
2. Set the API key or auth token to your Coder API token.

The exact variable names and configuration format differ by tool. See
[Client Configuration](./clients/index.md) for base URLs and
authentication details, and individual client pages for tool-specific
instructions:

- [Claude Code](./clients/claude-code.md)
- [Codex CLI](./clients/codex.md)
- [All supported clients](./clients/index.md#all-supported-clients)

### 3. Verify the connection

Run a request through your tool. If authentication succeeds, the request
appears in the [AI Gateway audit log](./audit.md). If you receive a `401`
error, confirm your API token is valid. If you receive a `403`, BYOK may
be disabled and the tool may be sending a personal LLM key instead of the
Coder token.
