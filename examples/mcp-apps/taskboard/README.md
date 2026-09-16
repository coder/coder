# Task board MCP App example

A minimal MCP server that keeps an in-memory task list and exposes it as an
MCP App (MCP Apps extension, protocol version 2026-01-26). It exists to
exercise a Coder Agents chat host that renders MCP Apps in a side panel.

The server registers four tools (`add_task`, `complete_task`, `delete_task`,
`list_tasks`), each tagged with `_meta.ui.resourceUri = "ui://taskboard/board"`,
and one resource `ui://taskboard/board` served as `text/html;profile=mcp-app`.
Every tool result carries a plain text summary for text-only hosts plus
`structuredContent: { tasks }` for the view.

`src/board.html` is a self-contained page with no SDK and no build step. It
talks to the host with raw JSON-RPC over `postMessage`
(`ui/initialize`, `ui/notifications/initialized`, `tools/call`,
`ui/update-model-context`, and the `ui/notifications/*` notifications). The
same file is also used as a frontend test fixture for the host.

## Run

```sh
cd examples/mcp-apps/taskboard
pnpm install
pnpm start
```

The server listens on `http://localhost:3333/mcp` (override with `PORT`). It
uses stateless Streamable HTTP, so every request is served by a fresh MCP
server instance while the task list is shared at module scope. Because the
server does not keep per-session state, it cannot inspect the client's
`io.modelcontextprotocol/ui` capability after `initialize`, so the UI tools
are registered unconditionally. Text-only hosts still get the `content` text.

`pnpm check` runs `tsc --noEmit`.

## Register in Coder

1. Start Coder with the `chat-mcp-apps` experiment and a wildcard access URL.
   For a dev server:

   ```sh
   CODER_EXPERIMENTS=chat-mcp-apps CODER_WILDCARD_ACCESS_URL='*.localhost' ./scripts/develop.sh
   ```

2. In the dashboard go to Deployment > MCP servers and add a server with
   transport `streamable_http` and URL `http://localhost:3333/mcp`. The URL
   must be reachable from coderd, not from your browser, so adjust the host
   if coderd runs elsewhere.

## Try it

In a chat: "add a task to buy milk". The board appears in the side panel.
Check the task off in the panel, then ask the model to list tasks; the model
sees the completed state because the view pushes updates with
`ui/update-model-context`.
