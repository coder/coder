export type KnownModelSourceMetadata = {
	sourceName: "models.dev";
	sourceRetrievedAt: string;
	lastUpdated: string;
};

export type KnownModel = {
	provider: string;
	model: string;
	displayName: string;
	aliases: readonly string[];
	contextLimit?: number;
	maxOutputTokens?: number;
	inputCost?: number;
	outputCost?: number;
	cacheReadCost?: number;
	cacheWriteCost?: number;
	sourceMetadata: KnownModelSourceMetadata;
};
