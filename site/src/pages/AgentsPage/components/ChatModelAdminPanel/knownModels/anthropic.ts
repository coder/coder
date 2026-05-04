import type { KnownModel } from "./types";

// Array order controls suggestion order. Keep sourceMetadata.lastUpdated in
// sync with the corresponding models.dev last_updated value for each model.
// Coder currently persists flat pricing only. Tiered models.dev pricing,
// such as context_over_200k, is intentionally omitted.
//
// The `reasoningEffort` value is editorial, not from models.dev. It reflects
// the provider's documented default for reasoning-capable models in this
// catalog and should be reviewed when the catalog is refreshed.
//
// Reasoning configuration is split per model based on Anthropic API support:
// models that support adaptive thinking (Opus 4.7, Opus 4.6, Sonnet 4.6)
// carry `reasoningEffort`, which Coder maps to `thinking.type: "adaptive"`
// with the `effort` parameter. Models that do not (Haiku 4.5, Sonnet 4.5)
// carry `thinkingBudgetTokens` instead, which Coder maps to the legacy
// `thinking.type: "enabled"` path with `budget_tokens`. Setting `effort` on
// the legacy path produces an "adaptive thinking is not supported on this
// model" HTTP 400 from Anthropic.
export const anthropicKnownModels = [
	{
		provider: "anthropic",
		modelIdentifier: "claude-opus-4-7",
		displayName: "Claude Opus 4.7",
		aliases: [],
		contextLimit: 1_000_000,
		maxOutputTokens: 128_000,
		reasoningEffort: "high",
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
		modelIdentifier: "claude-opus-4-6",
		displayName: "Claude Opus 4.6",
		aliases: [],
		contextLimit: 1_000_000,
		maxOutputTokens: 128_000,
		reasoningEffort: "high",
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
		modelIdentifier: "claude-sonnet-4-6",
		displayName: "Claude Sonnet 4.6",
		aliases: [],
		contextLimit: 1_000_000,
		maxOutputTokens: 64_000,
		reasoningEffort: "medium",
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
		modelIdentifier: "claude-haiku-4-5",
		displayName: "Claude Haiku 4.5",
		aliases: ["claude-haiku-4-5-20251001"],
		contextLimit: 200_000,
		maxOutputTokens: 64_000,
		thinkingBudgetTokens: 8192,
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
		modelIdentifier: "claude-sonnet-4-5",
		displayName: "Claude Sonnet 4.5",
		aliases: ["claude-sonnet-4-5-20250929"],
		contextLimit: 200_000,
		maxOutputTokens: 64_000,
		thinkingBudgetTokens: 8192,
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
