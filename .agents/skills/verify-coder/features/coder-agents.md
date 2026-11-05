# Coder Agents

An admin adds an AI provider and a model. A user then opens `/agents`, sends a message, and sees the model's reply stream into the chat.

## Sub-features

- `agents-provider` adds a provider on `/ai/settings/providers`.
- `agents-model` adds a model on `/ai/settings/models`.
- `agents-chat` sends a message from `/agents` and receives a reply.
- `agents-empty` shows setup guidance when no model exists.

## How to get to it (user POV)

- `Agents` in the navbar, or `$VC_WEB_URL/agents`. A chat lives at `/agents/<chat id>`.
- `$VC_WEB_URL/ai/settings/providers`, `/ai/settings/models` and `/ai/settings/coder-agents` for setup.

## Driving it with agent-browser and the coder CLI

Preconditions:

- `scripts/doctor.sh` passes and the browser is signed in as the admin.
- A local OpenAI-compatible fake runs: `scripts/coder.sh exp scaletest llm-mock --address 127.0.0.1:<port>` in the background. It serves `/v1/chat/completions`, `/v1/responses` and `/v1/messages`. Stop it by its pid when done.

- **Empty state.** Before setup, `vc_ab open "$VC_WEB_URL/agents"`. The admin sees links to providers and models. The member sees `AI models aren't available yet.`
- **Add a provider.** `vc_ab open "$VC_WEB_URL/ai/settings/providers"`, click the `Add provider` button, pick the OpenAI-compatible type (`openai-compat`), fill `Name`, `Endpoint` with `http://127.0.0.1:<port>/v1` and `API key` with any placeholder, then click `Add provider`.
- **Add a model.** `vc_ab open "$VC_WEB_URL/ai/settings/models"`, click `Add model`, pick the provider, fill `Display name` and the model ID, then click `Add Model`.
- **Chat.** `vc_ab open "$VC_WEB_URL/agents"`, `vc_ab find label "Chat message" fill "Say hello"`, then `vc_ab find role button click --name "Send"`. The URL becomes `/agents/<chat id>` and a `Stop` button shows while the reply streams.
- **Reply proof.** `vc_api /api/v2/chats/<chat id>` reaches status `waiting` after `running`. `vc_api /api/v2/chats/<chat id>/messages` has an `assistant` message. Screenshot the chat with the reply.

## Gotchas

- Nothing seeds a provider or a model, and `launch.sh` unsets `ANTHROPIC_*` and `OPENAI_*`. Use the fake unless the change under test needs a real model, and then never screenshot or snapshot the provider form after you type a real key.
- The `openai-compat` endpoint needs the `/v1` suffix. Coder appends `/chat/completions`.
- Chat titles and status labels call the model with a forced tool choice. A fake that answers with text only leaves the fallback title. Check the title only against a fake that answers tool calls.
- No `site/e2e` test sends a chat message, so there is no Playwright recipe to copy. This flow was written from the page sources and was not run when the skill was created.
- Chats run inside a workspace only when the model calls workspace tools. A text reply needs no workspace.
