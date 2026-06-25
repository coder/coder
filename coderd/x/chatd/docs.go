// Package chatd implements the internal chat service used by Agents.
//
// # Provider configuration glossary
//
// This package uses AI Provider language for new provider configuration code:
//
//   - AI Provider: a database-backed LLM provider configuration stored in
//     ai_providers. It is the source of truth for Agents provider identity.
//   - Legacy Chat Provider: the pre-migration chat-specific provider source.
//     Legacy rows only exist as migration input during the stack.
//   - Provider Type: the provider implementation family stored in
//     ai_providers.type, such as openai, anthropic, azure, bedrock, google,
//     openai-compat, openrouter, and vercel.
//   - Provider Name: the unique instance identifier stored in
//     ai_providers.name. It is not the implementation family.
//   - Model Config: an Agents model selection record. In the target state it
//     references one concrete AI Provider by ID.
//   - Provider-scoped AI Provider Key: an administrator-managed credential in
//     ai_provider_keys, attached to one AI Provider.
//   - User AI Provider Key: a user-owned credential attached to one user and
//     one AI Provider.
//   - BYOK: the deployment-level AI Gateway policy that controls whether user
//     keys may be written or used. Disabling BYOK does not delete stored user
//     keys.
//   - AI Gateway: the product area that introduced AI provider records. Agents
//     consume the same records through chatd, but this package does not define
//     the full AI Gateway runtime roadmap.
//
// Model configs should use provider IDs for identity. Provider types choose
// runtime implementation details. Provider names are instance identifiers for
// administrators and APIs.
//
// When BYOK is enabled, a user key for the selected provider takes precedence
// over provider-scoped keys. When BYOK is disabled, chatd ignores user keys and
// uses provider-scoped keys only.
package chatd
