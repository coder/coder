# Coding Agents

> [!NOTE]
>
> This page is not exhaustive and the landscape is evolving rapidly. Please
> [open an issue](https://github.com/coder/coder/issues/new) or submit a pull
> request if you'd like to see your favorite agent added or updated.

There are several types of coding agents emerging:

- **Headless agents** can run without an IDE open and are great for rapid
  prototyping, background tasks, and chat-based supervision.
- **In-IDE agents** require developers keep their IDE opens and are great for
  interactive, focused coding on more complex tasks.

## Headless agents

Headless agents can run without an IDE open, or alongside any IDE. They
typically run as CLI commands or web apps. With Coder, developers can interact
with agents via any preferred tool such as via PR comments, within the IDE,
inside the Coder UI, or even via the REST API or an MCP client such as Claude
Desktop or Cursor.

| Agent          | Supported Models                                        | Coder Support                                                      | Limitations                                             |
| -------------- | ------------------------------------------------------- | ------------------------------------------------------------------ | ------------------------------------------------------- |
| Claude Code ⭐ | Anthropic Models Only (+ AWS Bedrock and GCP Vertex AI) | First class integration ✅                                         | Beta (research preview)                                 |
| Goose          | Most popular AI models + gateways                       | First class integration ✅                                         | Less effective compared to Claude Code                  |
| Aider          | Most popular AI models + gateways                       | Requires [MCP Plugin](https://github.com/lutzleonhardt/mcpm-aider) | Can only run 1-2 defined commands (e.g. build and test) |
| OpenHands      | Most popular AI models + gateways                       | In progress ⏳                                                     | Challenging setup, no MCP support                       |

[Claude Code](https://github.com/anthropics/claude-code) is our recommended
coding agent due to its strong performance on complex programming tasks.

## In-IDE agents

Coding agents can also run within an IDE, such as VS Code, Cursor or Windsurf.
These editors and extensions are fully supported in Coder and work well for more
complex and focused tasks where an IDE is strictly required.

| Agent                       | Supported Models                  | Coder Support                                                 |
| --------------------------- | --------------------------------- | ------------------------------------------------------------- |
| Cursor (Agent Mode)         | Most popular AI models + gateways | ✅ [Cursor Module](https://registry.coder.com/modules/cursor) |
| Windsurf (Agents and Flows) | Most popular AI models + gateways | ✅ via Remote SSH                                             |
| Cline                       | Most popular AI models + gateways | ✅ via VS Code Extension                                      |

In-IDE agents do not require a special template as they cannot be used in a
headless fashion. However, they can still be run in isolated Coder workspaces
and report activity to the Coder UI.

## Next Steps

- [Create a Coder template for agents](./create-template.md)
