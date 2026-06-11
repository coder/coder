# Tools

Coder Agents completes work by calling tools: structured functions the agent
invokes during a chat to gather context and take action, such as listing
templates, reading files, running commands, or creating a workspace.

This page explains how tool calls work and documents the tools the agent uses
to select a template and create a workspace.

## How tool calls work

Each turn of a conversation follows the same loop:

1. You send a message.
2. The model decides whether it needs a tool and calls it with structured
   arguments. For example, `list_templates` with `{"query": "docker"}`.
3. Coder executes the tool in the control plane using your identity and
   permissions, and returns a JSON result to the model.
4. The model uses the result to decide what to do next: call another tool or
   reply to you.

Tool calls and their results are visible in the chat timeline, so you can
always inspect what the agent did and why.

Two properties hold for every built-in tool:

- **Your permissions apply.** Tools run with the chat owner's RBAC identity.
  The agent cannot list templates or create workspaces that you could not
  access yourself.
- **Results carry instructions.** Where a decision matters, the tool result
  includes a `next_step` field containing a fixed instruction for the agent,
  such as asking you to choose between similar templates. This keeps agent
  behavior consistent without relying on long prompt rules.

## Workspace creation tools

A chat starts without a workspace, and many requests are answered without
one. When the agent needs compute (to read files, run commands, or edit
code), it provisions a workspace using three tools:

| Tool               | Purpose                                                   |
|--------------------|-----------------------------------------------------------|
| `list_templates`   | Rank available templates and recommend one when confident |
| `read_template`    | Read a template's parameters and presets                  |
| `create_workspace` | Create the workspace from a chosen template               |

Administrators can restrict which templates these tools can see with the
[template allowlist](../platform-controls/template-optimization.md#restrict-available-templates).

### list_templates

`list_templates` returns a ranked shortlist of the active, non-deprecated
templates in the chat's organization, so the agent can pick a template the
same way a colleague would: prefer what matches the request, what you already
use, and what the rest of the organization uses.

#### Arguments

| Argument | Type    | Description                                                                   |
|----------|---------|-------------------------------------------------------------------------------|
| `query`  | string  | Optional text matched against template names, display names, and descriptions |
| `page`   | integer | Optional page number, starting at 1. Each page holds 10 templates             |

#### How templates are ranked

Templates are ordered by three signals, in priority order:

1. **Query relevance.** When a query is provided, an exact name match ranks
   above a name prefix match, then a name substring match, then a description
   match. Matching is case-insensitive and ignores spaces, hyphens, and
   underscores, so `python gpu` matches `python-gpu`. Templates that do not
   match the query at all are excluded.
2. **Your recent usage.** Workspaces you used in the last 60 days count
   toward a template's score. Recent usage counts more than old usage (the
   weight halves every 14 days), and recently deleted workspaces count at
   reduced weight.
3. **Organization popularity.** The number of developers with an active
   workspace on the template. Prebuilt workspaces that have not been claimed
   are excluded so they do not inflate popularity.

#### Recommendation and next_step

Beyond the ranked list, the result tells the agent what to do next:

- `recommended_template_id` is present only when the top template is a clear
  winner: it is the only available template, it decisively matches the query,
  or your usage and organization usage clearly favor it over the runner-up.
- `next_step` is always present and contains one of four fixed instructions:

| Situation                           | `next_step` instruction for the agent                 |
|-------------------------------------|-------------------------------------------------------|
| A template is recommended           | Use `recommended_template_id` with `create_workspace` |
| Top templates are too close to call | Ask you to choose a template                          |
| The query matched nothing           | Retry without a query or ask you                      |
| No templates are available          | Inform you that no templates are available            |

When the agent is told to ask, it presents the choices instead of guessing.
You can always override the recommendation by naming a template yourself.

#### Result

```json
{
  "templates": [
    {
      "id": "0f9ab36e-43f6-4d8a-b3e6-6803d9a06f72",
      "name": "docker",
      "display_name": "Docker",
      "description": "Provision Docker containers as Coder workspaces.",
      "active_developers": 14,
      "your_workspace_count": 2,
      "last_used_by_you": "2026-06-09T10:04:18.123456Z"
    }
  ],
  "page": 1,
  "recommended_template_id": "0f9ab36e-43f6-4d8a-b3e6-6803d9a06f72",
  "next_step": "Use recommended_template_id with create_workspace. Call read_template first only if you need parameter or preset details."
}
```

Each template includes evidence for its position: `active_developers`,
`your_workspace_count`, and `last_used_by_you` appear when they are non-zero.
`next_page` is present only when more results exist.

### read_template

`read_template` reads one template's details: its configurable parameters
(name, type, default, options, and validation rules) and its presets,
including preset parameter values and whether prebuilt workspaces are
available for faster startup.

The agent calls it only when needed, typically when a template has required
parameters or when a preset should be applied. For templates that work with
defaults, the agent skips straight to `create_workspace`.

### create_workspace

`create_workspace` provisions a workspace from a template and waits for it to
become ready, then attaches it to the chat.

| Argument      | Type   | Description                                                                        |
|---------------|--------|------------------------------------------------------------------------------------|
| `template_id` | string | Required. The template UUID from `list_templates`                                  |
| `name`        | string | Optional workspace name. One is generated if omitted                               |
| `parameters`  | object | Optional template parameter values from `read_template`                            |
| `preset_id`   | string | Optional preset UUID. Applies preset parameters and may claim a prebuilt workspace |

Guardrails:

- The agent is instructed to create a workspace only when the task requires
  one or when you explicitly ask for it, and to follow the `next_step` from
  `list_templates`, which means asking you first when no template was
  recommended.
- The tool is idempotent: if the chat already has a workspace building or
  running, that workspace is returned instead of creating a duplicate.
- Templates outside the administrator's allowlist are rejected.
