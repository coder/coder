# Coder Agents

Coder Agents route model requests through AI Gateway automatically by using the
provider and model settings from the Agents dashboard. They do not use the AI
Gateway external client base URL flow.

For setup, configure providers and models in
[Coder Agents models and providers](../../agents/models.md). Set each provider's
endpoint/base URL to the upstream provider or proxy, not
`https://<coder-host>/api/v2/aibridge/...`.

For credential behavior, see
[Credential selection](../../agents/models.md#credential-selection). Coder Agents
BYOK follows the [global AI Gateway BYOK setting](../auth.md#bring-your-own-key-byok).

For external tools that call AI Gateway over HTTP, use the
[AI Gateway client setup guides](./index.md).
