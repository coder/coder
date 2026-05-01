export type KnownModelSourceMetadata = {
	sourceName: "models.dev";
	sourceRetrievedAt: string;
	lastUpdated: string;
};

export type KnownModel = {
	provider: string;
	modelIdentifier: string;
	displayName: string;
	aliases: readonly string[];
	contextLimit?: number;
	maxOutputTokens?: number;
	reasoningEffort?: "low" | "medium" | "high";
	/**
	 * Anthropic-only: numeric budget for the legacy
	 * `thinking.budget_tokens` API.
	 *
	 * Use this for Anthropic models that do not support adaptive thinking.
	 */
	thinkingBudgetTokens?: number;
	/** USD per million tokens. Flat base rate from models.dev. */
	inputCost?: number;
	outputCost?: number;
	cacheReadCost?: number;
	cacheWriteCost?: number;
	sourceMetadata: KnownModelSourceMetadata;
};
