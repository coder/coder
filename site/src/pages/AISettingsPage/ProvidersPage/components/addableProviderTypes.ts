import type { AIProviderType } from "#/api/typesGenerated";

type AddableProvider = {
	value: AIProviderType;
	label: string;
};

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

export const findAddableProvider = (
	value: string | null | undefined,
): AddableProvider | undefined =>
	addableProviders.find((p) => p.value === value);

export const isAddableProviderType = (
	value: string | null | undefined,
): value is AIProviderType => findAddableProvider(value) !== undefined;
