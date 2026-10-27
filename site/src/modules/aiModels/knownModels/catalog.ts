import { normalizeProvider } from "#/modules/aiModels/helpers";
import knownModelsGenerated from "./knownModelsGenerated.json";
import type { KnownModel } from "./types";

// knownModelsGenerated.json is produced by `make gen/aibridge-prices` from
// models.dev joined with the editorial curation in
// scripts/aibridgepricesgen/curation.json. Do not edit it manually. JSON
// imports widen literal types (e.g. reasoningEffort becomes string), so this
// cast is the single typed boundary; knownModelsGenerated.test.ts validates
// shape and enum values for every entry. The keyof cast preserves the
// literal provider-key union so isKnownProvider narrows usefully.
const knownModelsByProvider = knownModelsGenerated as Record<
	keyof typeof knownModelsGenerated,
	readonly KnownModel[]
>;

type KnownProvider = keyof typeof knownModelsByProvider;

const isKnownProvider = (provider: string): provider is KnownProvider =>
	provider in knownModelsByProvider;

const normalizeSearchText = (value: string): string =>
	value.toLowerCase().replace(/[\s._-]/g, "");

export const getKnownModelsForProvider = (
	provider: string,
): readonly KnownModel[] => {
	const normalizedProvider = normalizeProvider(provider);
	if (!isKnownProvider(normalizedProvider)) {
		return [];
	}
	return knownModelsByProvider[normalizedProvider];
};

export const searchKnownModels = (
	provider: string,
	query: string,
): readonly KnownModel[] => {
	const providerModels = getKnownModelsForProvider(provider);
	if (query.trim() === "") {
		return providerModels;
	}

	const normalizedQuery = normalizeSearchText(query);
	if (normalizedQuery === "") {
		return providerModels;
	}

	return providerModels.filter((knownModel) =>
		[
			knownModel.modelIdentifier,
			knownModel.displayName,
			...knownModel.aliases,
		].some((value) => normalizeSearchText(value).includes(normalizedQuery)),
	);
};

export const findKnownModelByExactAlias = (
	provider: string,
	value: string,
): KnownModel | undefined => {
	const lowercaseValue = value.toLowerCase();
	return getKnownModelsForProvider(provider).find((knownModel) =>
		knownModel.aliases.some((alias) => alias.toLowerCase() === lowercaseValue),
	);
};

export const findKnownModelByCanonicalId = (
	provider: string,
	modelId: string,
): KnownModel | undefined => {
	const normalizedProvider = normalizeProvider(provider);
	if (normalizedProvider === "" || modelId === "") {
		return undefined;
	}
	return getKnownModelsForProvider(normalizedProvider).find(
		(knownModel) => knownModel.modelIdentifier === modelId,
	);
};

// Canonical identifiers verified to accept chatd's computer use tool
// (Anthropic computer_20251124, OpenAI Responses computer tool).
export const knownComputerUseModels: readonly {
	readonly provider: string;
	readonly modelIdentifier: string;
}[] = [
	{ provider: "anthropic", modelIdentifier: "claude-fable-5-1" },
	{ provider: "anthropic", modelIdentifier: "claude-fable-5" },
	{ provider: "anthropic", modelIdentifier: "claude-opus-5" },
	{ provider: "anthropic", modelIdentifier: "claude-opus-4-8" },
	{ provider: "anthropic", modelIdentifier: "claude-opus-4-7" },
	{ provider: "anthropic", modelIdentifier: "claude-opus-4-6" },
	{ provider: "anthropic", modelIdentifier: "claude-sonnet-5" },
	{ provider: "anthropic", modelIdentifier: "claude-sonnet-4-6" },
	{ provider: "openai", modelIdentifier: "gpt-6-astra" },
	{ provider: "openai", modelIdentifier: "gpt-6.1-sol" },
	{ provider: "openai", modelIdentifier: "gpt-6-sol" },
	{ provider: "openai", modelIdentifier: "gpt-6-luna" },
	{ provider: "openai", modelIdentifier: "gpt-5.6-sol" },
	{ provider: "openai", modelIdentifier: "gpt-5.6-terra" },
	{ provider: "openai", modelIdentifier: "gpt-5.6-luna" },
	{ provider: "openai", modelIdentifier: "gpt-5.5" },
	{ provider: "openai", modelIdentifier: "gpt-5.4" },
	{ provider: "openai", modelIdentifier: "gpt-5.4-mini" },
];

/**
 * Reports whether a model is on the curated computer use list. Only the exact
 * canonical identifier matches; aliases and dated snapshots do not.
 */
export const isKnownComputerUseModel = (
	provider: string,
	model: string,
): boolean => {
	const normalizedProvider = normalizeProvider(provider);
	return knownComputerUseModels.some(
		(entry) =>
			entry.provider === normalizedProvider && entry.modelIdentifier === model,
	);
};

const formatCompactNumber = (value: number): string => {
	if (Number.isInteger(value)) {
		return String(value);
	}
	return value.toFixed(2).replace(/\.?0+$/, "");
};

export const formatContextBadge = (contextLimit: number): string => {
	if (!Number.isInteger(contextLimit) || contextLimit <= 0) {
		throw new Error("contextLimit must be a positive finite integer");
	}

	if (contextLimit < 1_000) {
		return `${contextLimit} context`;
	}
	if (contextLimit < 1_000_000) {
		return `${formatCompactNumber(contextLimit / 1_000)}K context`;
	}
	return `${formatCompactNumber(contextLimit / 1_000_000)}M context`;
};

export const formatPricePerMillionTokens = (
	value: number,
): { belowThreshold: boolean; value: string } => {
	if (!Number.isFinite(value)) {
		throw new Error("price must be a finite number");
	}
	if (Number.isInteger(value)) {
		return { belowThreshold: false, value: String(value) };
	}
	// A positive price too small to show at four decimals would otherwise
	// render as $0.00 and read as free, so show it as a threshold instead.
	if (value > 0 && value < 0.0001) {
		return { belowThreshold: true, value: "0.0001" };
	}
	// Keep two decimals so cents read as cents (0.10, not 0.1). Keep up to
	// four so sub-cent prices stay visible (0.075, 0.0036).
	const [whole, decimals = ""] = value.toFixed(4).split(".");
	const trimmed = decimals.slice(0, 2) + decimals.slice(2).replace(/0+$/, "");
	return { belowThreshold: false, value: `${whole}.${trimmed}` };
};
