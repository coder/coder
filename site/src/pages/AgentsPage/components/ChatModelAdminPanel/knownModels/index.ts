import { normalizeProvider } from "../helpers";
import { anthropicKnownModels } from "./anthropic";
import { openAIKnownModels } from "./openai";
import type { KnownModel, KnownModelSourceMetadata } from "./types";

export type { KnownModel, KnownModelSourceMetadata };

const knownModelsByProvider = {
	anthropic: anthropicKnownModels,
	openai: openAIKnownModels,
} as const satisfies Record<string, readonly KnownModel[]>;

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
