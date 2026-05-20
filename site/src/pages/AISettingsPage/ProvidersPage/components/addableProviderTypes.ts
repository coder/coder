import type { AIProviderType } from "#/api/typesGenerated";

/**
 * AddableProviderType is the UI-facing type set for the "Add provider"
 * dropdown. It mirrors the server-side `AIProviderType` enum plus a
 * `bedrock` value: Bedrock providers ship on the wire as
 * `type:"anthropic"` with a discriminated `settings._type:"bedrock"`
 * blob, but the UI surfaces a dedicated Bedrock entry so admins can
 * pick it directly from the dropdown and land on a Bedrock-specific
 * form.
 */
type AddableProviderType = AIProviderType | "bedrock";

/**
 * Single source of truth for the "Add provider" dropdown. Every entry
 * is rendered in the menu so the menu's set always matches the
 * documented provider types; `isSupported` controls whether clicking
 * the item routes to `/ai/settings/add?type=...` or renders as a
 * disabled "coming soon" row.
 *
 * Today only OpenAI, Anthropic, and Bedrock land cleanly through the
 * server-side `CreateAIProviderRequest.Validate`; the other five
 * surface in the menu for discoverability but stay disabled until
 * backend support lands.
 */
type AddableProvider = {
	value: AddableProviderType;
	label: string;
	isSupported: boolean;
};

export const addableProviders: readonly AddableProvider[] = [
	{ value: "anthropic", label: "Anthropic", isSupported: true },
	{ value: "openai", label: "OpenAI", isSupported: true },
	{ value: "bedrock", label: "AWS Bedrock", isSupported: true },
	{ value: "azure", label: "Azure OpenAI Service", isSupported: false },
	{ value: "google", label: "Google", isSupported: false },
	{ value: "openai-compat", label: "OpenAI via bridge", isSupported: false },
	{ value: "openrouter", label: "OpenRouter", isSupported: false },
	{ value: "vercel", label: "Vercel AI Gateway", isSupported: false },
];

/**
 * Returns the metadata entry for an arbitrary string when it matches
 * a known addable provider type, otherwise `undefined`. Callers that
 * receive a `type` query param can use this to both validate the
 * input and look up the human-friendly label in a single pass.
 */
export const findAddableProvider = (
	value: string | null | undefined,
): AddableProvider | undefined =>
	addableProviders.find((p) => p.value === value);

/**
 * Narrowing helper for the supported subset: returns true when the
 * value is a known provider AND the server can accept a create
 * request for it today.
 */
export const isSupportedAddableProviderType = (
	value: string | null | undefined,
): value is "openai" | "anthropic" | "bedrock" => {
	const entry = findAddableProvider(value);
	return entry?.isSupported === true;
};
