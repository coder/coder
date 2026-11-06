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
- A local OpenAI-compatible fake runs: `"$(vc_coder_bin)" exp scaletest llm-mock --address 127.0.0.1:<port> &`. It answers every model name on `/v1/chat/completions` with `This is a mock response from OpenAI.` Record its pid as `$VC_RUN/<name>.pid` so `scripts/cleanup.sh` stops it if you do not.

- **Whole journey.** Run `journeys/coder-agents.sh`. It starts and stops its own mock, puts the instance back (chat archived, the previous default model restored, its model and provider deleted, also when a check fails or the run is interrupted), ends with `coder-agents: 7/7 checks passed`, and writes evidence to `build/verify-coder/evidence/<stamp>-coder-agents/`.
- **Empty state.** Before any model exists, `/agents` shows the admin a `provider` link and a `model` link, and the model picker reads `No Models Configured`.
- **Add a provider.** On `$VC_WEB_URL/ai/settings/providers` (title `AI Providers - Coder`), click the `Add provider` button, then `vc_ab find role menuitem click --name "OpenAI-compatible"`. The URL becomes `/ai/settings/providers/add?type=openai-compat`. Wait for `input[name=apiKey]`, then fill `input[name=name]` (prefilled `openai-compat`, must be unique), `input[name=displayName]`, `input[name=baseUrl]` with `http://127.0.0.1:<port>/v1` and `input[name=apiKey]` with any placeholder. Click `Add provider`. A toast reads `Provider "<display name>" added.` Second view: `vc_api /api/v2/ai/providers` lists it with `type` `openai-compat` and that `base_url`.
- **Add a model.** On `$VC_WEB_URL/ai/settings/models` (title `Models - AI Settings - Coder`), click `Add model`, then the menu item named after the provider's display name. Wait for `#contextLimit`, then fill `#model` (any ID, such as `mock-1`), `#displayName` and `#contextLimit` (`128000`). Tick `find label "Set as Coder Agents default model" check`, then click `Add Model`. When a default already exists, a `Replace default model` dialog asks first: click `Confirm`. A toast reads `Model "<display name>" added.` Second view: `vc_api /api/v2/organizations/<org id>/chats/models` returns `{models, providers}`, and the model has `is_default` `true`.
- **Chat.** `vc_ab open "$VC_WEB_URL/agents"`, then `vc_ab wait '[data-testid=chat-message-input]'` and `vc_ab wait --text "<model display name>"`. The model picker is a combobox named after the default model. `vc_ab find label "Chat message" fill "<text>"`, then `vc_ab find role button click --name "Send"`. The URL becomes `/agents/<chat id>`, the reply appears, and the sidebar row reads `Finished latest turn`.
- **Reply proof.** `vc_api /api/v2/chats/<chat id>` reaches `status` `waiting`. `vc_api /api/v2/chats/<chat id>/messages` returns the messages newest first: an `assistant` message whose text part is the mock reply, then the `user` message.

## Gotchas

- Nothing seeds a provider or a model, and `launch.sh` unsets `ANTHROPIC_*` and `OPENAI_*`. Use the mock unless the change under test needs a real model, and then never screenshot or snapshot the provider form after you type a real key. The journey screenshots the provider form before it types the placeholder key.
- The `openai-compat` endpoint needs the `/v1` suffix. A loopback URL is accepted.
- Label fills on the provider and model forms failed when they ran before the form rendered. Wait for an input first, and fill by `name` or `id`.
- With `llm-mock`, the chat title is the first message's text.
- A text-only chat created no workspace.
- On a reused `.coderv2`, a hand-driven run must undo its setup as the journey does: `PATCH /api/v2/chats/<id>` with `{"archived": true}`, `PATCH /api/v2/organizations/<org id>/chats/models/<old default id>` with `{"is_default": true}`, then `DELETE` the model and `DELETE /api/v2/ai/providers/<name>`. Use `vc_curl`.
- On the Vite dev server (`$VC_VITE_URL`) at load average 25 to 30, `/agents` twice showed the app's `Something went wrong` page. The console held `Failed to fetch dynamically imported module: .../AgentCreatePage.tsx`. Journeys drive the embedded frontend on `$VC_WEB_URL` instead, which loads bundled chunks.
