import type { KnownModel } from "./types";

// Array order controls suggestion order. Keep sourceMetadata.lastUpdated in
// sync with the corresponding models.dev last_updated value for each model.
// Coder currently persists flat pricing only. Tiered models.dev pricing,
// such as context_over_200k, is intentionally omitted.
//
// The `reasoningEffort` value is editorial, not from models.dev. It reflects
// the provider's documented default for reasoning-capable models in this
// catalog and should be reviewed when the catalog is refreshed.
export const openAIKnownModels = [
	{
		provider: "openai",
		modelIdentifier: "gpt-5.5",
		displayName: "GPT-5.5",
		aliases: [],
		contextLimit: 1_050_000,
		maxOutputTokens: 128_000,
		reasoningEffort: "medium",
		inputCost: 5,
		outputCost: 30,
		cacheReadCost: 0.5,
		sourceMetadata: {
			sourceName: "models.dev",
			sourceRetrievedAt: "2026-04-30",
			lastUpdated: "2026-04-23",
		},
	},
	{
		provider: "openai",
		modelIdentifier: "gpt-5.5-pro",
		displayName: "GPT-5.5 Pro",
		aliases: [],
		contextLimit: 1_050_000,
		maxOutputTokens: 128_000,
		reasoningEffort: "high",
		inputCost: 30,
		outputCost: 180,
		sourceMetadata: {
			sourceName: "models.dev",
			sourceRetrievedAt: "2026-04-30",
			lastUpdated: "2026-04-23",
		},
	},
	{
		provider: "openai",
		modelIdentifier: "gpt-5.4",
		displayName: "GPT-5.4",
		aliases: [],
		contextLimit: 1_050_000,
		maxOutputTokens: 128_000,
		inputCost: 2.5,
		outputCost: 15,
		cacheReadCost: 0.25,
		sourceMetadata: {
			sourceName: "models.dev",
			sourceRetrievedAt: "2026-04-30",
			lastUpdated: "2026-03-05",
		},
	},
	{
		provider: "openai",
		modelIdentifier: "gpt-5.4-mini",
		displayName: "GPT-5.4 mini",
		aliases: [],
		contextLimit: 400_000,
		maxOutputTokens: 128_000,
		reasoningEffort: "medium",
		inputCost: 0.75,
		outputCost: 4.5,
		cacheReadCost: 0.075,
		sourceMetadata: {
			sourceName: "models.dev",
			sourceRetrievedAt: "2026-04-30",
			lastUpdated: "2026-03-17",
		},
	},
	{
		provider: "openai",
		modelIdentifier: "gpt-5.4-nano",
		displayName: "GPT-5.4 nano",
		aliases: [],
		contextLimit: 400_000,
		maxOutputTokens: 128_000,
		inputCost: 0.2,
		outputCost: 1.25,
		cacheReadCost: 0.02,
		sourceMetadata: {
			sourceName: "models.dev",
			sourceRetrievedAt: "2026-04-30",
			lastUpdated: "2026-03-17",
		},
	},
	{
		provider: "openai",
		modelIdentifier: "gpt-5.3-codex",
		displayName: "GPT-5.3 Codex",
		aliases: [],
		contextLimit: 400_000,
		maxOutputTokens: 128_000,
		reasoningEffort: "medium",
		inputCost: 1.75,
		outputCost: 14,
		cacheReadCost: 0.175,
		sourceMetadata: {
			sourceName: "models.dev",
			sourceRetrievedAt: "2026-04-30",
			lastUpdated: "2026-02-05",
		},
	},
] as const satisfies readonly KnownModel[];
