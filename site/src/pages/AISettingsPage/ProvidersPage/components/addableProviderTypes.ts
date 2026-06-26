import type { AIProviderType } from "#/api/typesGenerated";

export type AddableProvider = {
	value: AIProviderType;
	label: string;
};

export const addableProviders: readonly AddableProvider[] = [
	{ value: "anthropic", label: "Anthropic" },
	{ value: "bedrock", label: "AWS Bedrock" },
	{ value: "azure", label: "Azure OpenAI" },
	{ value: "copilot", label: "GitHub Copilot" },
	{ value: "google", label: "Google" },
	{ value: "openai", label: "OpenAI" },
	{ value: "openai-compat", label: "OpenAI-compatible" },
	{ value: "openrouter", label: "OpenRouter" },
	{ value: "vercel", label: "Vercel" },
];
