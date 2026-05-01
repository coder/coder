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
	/** USD per million tokens. Flat base rate from models.dev. */
	inputCost?: number;
	outputCost?: number;
	cacheReadCost?: number;
	cacheWriteCost?: number;
	sourceMetadata: KnownModelSourceMetadata;
};
