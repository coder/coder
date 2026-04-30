import type { KnownModel } from "./types";

// Array order controls suggestion order. Keep sourceMetadata.lastUpdated in
// sync with the corresponding models.dev last_updated value for each model.
export const anthropicKnownModels = [
	{
		provider: "anthropic",
		model: "claude-opus-4-7",
		displayName: "Claude Opus 4.7",
		aliases: [],
		contextLimit: 1_000_000,
		maxOutputTokens: 128_000,
		// Coder currently persists flat pricing only. Tiered models.dev pricing,
		// such as context_over_200k, is intentionally omitted.
		inputCost: 5,
		outputCost: 25,
		cacheReadCost: 0.5,
		cacheWriteCost: 6.25,
		sourceMetadata: {
			sourceName: "models.dev",
			sourceRetrievedAt: "2026-04-30",
			lastUpdated: "2026-04-16",
		},
	},
	{
		provider: "anthropic",
		model: "claude-opus-4-6",
		displayName: "Claude Opus 4.6",
		aliases: [],
		contextLimit: 1_000_000,
		maxOutputTokens: 128_000,
		// Coder currently persists flat pricing only. Tiered models.dev pricing,
		// such as context_over_200k, is intentionally omitted.
		inputCost: 5,
		outputCost: 25,
		cacheReadCost: 0.5,
		cacheWriteCost: 6.25,
		sourceMetadata: {
			sourceName: "models.dev",
			sourceRetrievedAt: "2026-04-30",
			lastUpdated: "2026-03-13",
		},
	},
	{
		provider: "anthropic",
		model: "claude-sonnet-4-6",
		displayName: "Claude Sonnet 4.6",
		aliases: [],
		contextLimit: 1_000_000,
		maxOutputTokens: 64_000,
		// Coder currently persists flat pricing only. Tiered models.dev pricing,
		// such as context_over_200k, is intentionally omitted.
		inputCost: 3,
		outputCost: 15,
		cacheReadCost: 0.3,
		cacheWriteCost: 3.75,
		sourceMetadata: {
			sourceName: "models.dev",
			sourceRetrievedAt: "2026-04-30",
			lastUpdated: "2026-03-13",
		},
	},
	{
		provider: "anthropic",
		model: "claude-haiku-4-5",
		displayName: "Claude Haiku 4.5 (latest)",
		aliases: ["claude-haiku-4-5-20251001"],
		contextLimit: 200_000,
		maxOutputTokens: 64_000,
		// Coder currently persists flat pricing only. Tiered models.dev pricing,
		// such as context_over_200k, is intentionally omitted.
		inputCost: 1,
		outputCost: 5,
		cacheReadCost: 0.1,
		cacheWriteCost: 1.25,
		sourceMetadata: {
			sourceName: "models.dev",
			sourceRetrievedAt: "2026-04-30",
			lastUpdated: "2025-10-15",
		},
	},
	{
		provider: "anthropic",
		model: "claude-sonnet-4-5",
		displayName: "Claude Sonnet 4.5 (latest)",
		aliases: ["claude-sonnet-4-5-20250929"],
		contextLimit: 200_000,
		maxOutputTokens: 64_000,
		// Coder currently persists flat pricing only. Tiered models.dev pricing,
		// such as context_over_200k, is intentionally omitted.
		inputCost: 3,
		outputCost: 15,
		cacheReadCost: 0.3,
		cacheWriteCost: 3.75,
		sourceMetadata: {
			sourceName: "models.dev",
			sourceRetrievedAt: "2026-04-30",
			lastUpdated: "2025-09-29",
		},
	},
] as const satisfies readonly KnownModel[];
