# Agents transcript tool icons design

## Problem statement

The AgentsPage chat transcript can become visually dense when many tool calls and log lines appear together. Specialized tool rows, especially shell execution rows, are currently harder to distinguish from surrounding transcript text. The transcript should use a consistent icon vocabulary so users can scan tool activity quickly without relying only on row text.

## Approved design

Add decorative lucide icons to every tool-call row in the AgentsPage transcript. Icons should appear at the row header level before the tool label or action summary, not inside command output or log bodies.

Use the existing `ToolIcon` component as the centralized source for icon selection. Generic tools already use this path, so the implementation should expand the mapping for all known tool names and make specialized tool renderers consume the same icon vocabulary where they currently bypass it.

The initial icon set is:

| Tool names | Icon intent |
| --- | --- |
| `execute`, `process_output`, `process_list`, `process_signal` | Terminal or shell activity |
| `read_file` | File reading |
| `write_file`, `edit_files` | File modification |
| `list_templates`, `read_template` | Template or file inspection |
| `create_workspace` | Workspace creation |
| `start_workspace` | Workspace start |
| `read_skill`, `read_skill_file` | Skill documentation |
| `propose_plan` | Planning checklist |
| `advisor` | Advice or idea |
| `ask_user_question` | User question |
| `computer` | Desktop or computer interaction |
| `chat_summarized` | Assistant summary |
| Subagent tools | Bot or monitor based on subagent descriptor |
| Unknown MCP or custom tools | MCP icon when available, otherwise wrench fallback |

Icons should use the same visual treatment across rows: 16px, non-growing, `text-current`, and grayscale for running MCP icons when applicable. They should not introduce new color categories. Existing running, failure, backgrounded, and killed status icons remain separate status indicators.

## Rejected alternatives

### Add icons only to `execute`

This would improve the most obvious dense case but leave the transcript visually inconsistent. Users would still need to parse text-only rows for other tools.

### Put icons inside output/log blocks

Icons inside command output would add noise to the highest-density part of the UI. The useful scan point is the row header, before the output expands into logs.

### Use bespoke icons inside each renderer

One-off icon selection in every specialized renderer would drift over time. Centralizing in `ToolIcon` keeps the vocabulary easier to audit and update.

## Edge cases

- MCP tools may provide external icons. Those should continue to render in monochrome when valid, with `WrenchIcon` as fallback after image load failure.
- Unknown tool names must still render an icon so the transcript never mixes icon and non-icon tool rows.
- Icons are decorative when text already labels the row. Status icons remain semantic where they are the visible indicator for running, failure, backgrounded, or killed states.
- Specialized rows that do not use the generic renderer must still include their icon at the row header level.
- The change should avoid altering transcript parsing, display-mode logic, or tool visibility rules.

## Verification plan

- Update or confirm Storybook coverage for dense transcript and tool-row examples, including `execute` and generic tools.
- Run formatting for the frontend files changed.
- Run targeted frontend tests or Storybook tests for the affected tool components when practical.
- Run TypeScript checking for the frontend before claiming completion.

## Open questions

None.
