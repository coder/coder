/**
 * AddableProviderType is the UI-facing type set for the "Add provider"
 * dropdown. Bedrock providers ship on the wire as `type:"anthropic"`
 * with a discriminated `settings._type:"bedrock"` blob, but the UI
 * surfaces a dedicated Bedrock entry so admins can pick it directly
 * from the dropdown and land on a Bedrock-specific form.
 *
 * Azure, Google, OpenAI-compatible, OpenRouter, and Vercel route
 * through aibridge's OpenAI client today (per the
 * `ai_provider_type_chatd_values` migration and the matching comment
 * block on `AIProviderType` in `codersdk/aiproviders.go`). The UI keeps
 * them as distinct dropdown entries so admins land on a form that's
 * preconfigured with the canonical endpoint and a friendly name.
 */
type AddableProviderType =
	| "openai"
	| "anthropic"
	| "bedrock"
	| "azure"
	| "google"
	| "openai-compat"
	| "openrouter"
	| "vercel";

type AddableProvider = {
	value: AddableProviderType;
	label: string;
};

/**
 * Provider types listed in the "Add provider" dropdown. Backed by the
 * server's `CreateAIProviderRequest.Validate` accept-list.
 */
export const addableProviders: readonly AddableProvider[] = [
	{ value: "anthropic", label: "Anthropic" },
	{ value: "bedrock", label: "AWS Bedrock" },
	{ value: "azure", label: "Azure OpenAI" },
	{ value: "google", label: "Google" },
	{ value: "openai", label: "OpenAI" },
	{ value: "openai-compat", label: "OpenAI-compatible" },
	{ value: "openrouter", label: "OpenRouter" },
	{ value: "vercel", label: "Vercel" },
];

/**
 * Returns the metadata entry when the value is a known addable
 * provider type, otherwise `undefined`. Callers that receive a
 * `type` query param use this to validate the input and look up the
 * human-friendly label in one pass.
 */
export const findAddableProvider = (
	value: string | null | undefined,
): AddableProvider | undefined =>
	addableProviders.find((p) => p.value === value);

/**
 * Narrowing guard for the addable provider set.
 */
export const isAddableProviderType = (
	value: string | null | undefined,
): value is AddableProviderType => findAddableProvider(value) !== undefined;
