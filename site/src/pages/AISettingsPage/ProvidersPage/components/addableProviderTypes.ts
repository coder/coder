/**
 * AddableProviderType is the UI-facing type set for the "Add provider"
 * dropdown. Bedrock providers ship on the wire as `type:"anthropic"`
 * with a discriminated `settings._type:"bedrock"` blob, but the UI
 * surfaces a dedicated Bedrock entry so admins can pick it directly
 * from the dropdown and land on a Bedrock-specific form.
 */
type AddableProviderType = "openai" | "anthropic" | "bedrock";

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
	{ value: "openai", label: "OpenAI" },
	{ value: "bedrock", label: "AWS Bedrock" },
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
