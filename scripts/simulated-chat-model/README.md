# Simulated chat model (prototype only)

An OpenAI-compatible HTTP server that stands in for an LLM so the AI workspace
debugging prototype can exercise the real Coder Agents path (chat creation,
system-prompt context injection, chatd tool execution, WebSocket streaming and
the chat UI) without provider credentials.

It is not an LLM. Replies are produced by matching the failure context and tool
results against a small set of known build-failure patterns, and every reply is
prefixed with `(simulated)`.

## Run

```sh
go run ./scripts/simulated-chat-model --addr 127.0.0.1:18080
```

Register it in a local deployment as an `openai-compat` provider and default
model. `CODER_SESSION_TOKEN` must belong to an owner:

```sh
CODER_URL=http://127.0.0.1:3000 CODER_SESSION_TOKEN=... ./scripts/simulated-chat-model/register.sh
```

Then open a workspace whose latest build failed. The workspace page shows the
debugging panel, creates a chat through
`POST /api/experimental/workspacebuilds/{build}/debug-chat`, and the simulated
model answers through chatd exactly as a real model would.

## Behaviour

- First turn: calls the real `get_workspace_build_logs` tool for the failed
  build so the tool path is exercised and visible in the UI.
- After the tool result: streams a diagnosis with `What failed`, `Evidence`,
  `Fix` and `Who` sections. Known patterns: image pull failures, credential
  errors, quota or capacity limits, agent connection timeouts, startup script
  errors, canceled builds and generic Terraform errors.
- Later user messages: a short canned reply that references the question.
- Non-streaming requests (chatd title generation) return a title in the
  structured-output shape chatd expects.

Pass `--dump-context` to log the failure context extracted from each request.
