import type * as TypesGen from "api/typesGenerated";
import type { ModelSelectorOption } from "#/components/ai-elements";
import { asNumber, asString } from "#/components/ai-elements/runtimeTypeUtils";

type RuntimeModelRef = {
	readonly provider?: unknown;
	readonly model?: unknown;
};

type ModelRefLike =
	| Pick<TypesGen.ChatModel, "provider" | "model">
	| Pick<TypesGen.ChatModelConfig, "provider" | "model">
	| RuntimeModelRef;

type CatalogModelLike =
	| TypesGen.ChatModel
	| (RuntimeModelRef & {
			readonly id?: unknown;
			readonly display_name?: unknown;
	  });

type CatalogProviderLike = Omit<TypesGen.ChatModelProvider, "models"> & {
	readonly models?: readonly CatalogModelLike[];
};

type ModelCatalogLike = {
	readonly providers?: readonly CatalogProviderLike[];
};

type ChatModelConfigLike =
	| Pick<TypesGen.ChatModelConfig, "provider" | "model" | "context_limit">
	| (RuntimeModelRef & Pick<TypesGen.ChatModelConfig, "context_limit">);

export const getNormalizedModelRef = (
	value: ModelRefLike,
): { readonly provider: string; readonly model: string } => {
	const modelRef = value ?? {};
	return {
		provider: asString(modelRef.provider).trim().toLowerCase(),
		model: asString(modelRef.model).trim(),
	};
};

/**
 * Build a lookup from model reference strings (both "provider:model" and
 * "provider/model" forms) to model config IDs.
 */
export const buildModelConfigIDByModelID = (
	configs:
		| readonly Pick<TypesGen.ChatModelConfig, "id" | "provider" | "model">[]
		| undefined,
): ReadonlyMap<string, string> => {
	const byModelID = new Map<string, string>();
	for (const config of configs ?? []) {
		const { provider, model } = getNormalizedModelRef(config);
		if (!provider || !model) continue;
		const colonRef = `${provider}:${model}`;
		if (!byModelID.has(colonRef)) byModelID.set(colonRef, config.id);
		const slashRef = `${provider}/${model}`;
		if (!byModelID.has(slashRef)) byModelID.set(slashRef, config.id);
	}
	return byModelID;
};

/**
 * Build a reverse lookup from model config IDs back to model reference
 * strings. Uses the first matching reference for each config ID.
 */
export const buildModelIDByConfigID = (
	modelConfigIDByModelID: ReadonlyMap<string, string>,
): ReadonlyMap<string, string> => {
	const byConfigID = new Map<string, string>();
	for (const [modelID, configID] of modelConfigIDByModelID.entries()) {
		if (!byConfigID.has(configID)) byConfigID.set(configID, modelID);
	}
	return byConfigID;
};

const getCatalogProviders = (
	catalog: ModelCatalogLike | null | undefined,
): readonly CatalogProviderLike[] => {
	const providers = catalog?.providers;
	return Array.isArray(providers) ? providers : [];
};

const getProviderModels = (
	provider: CatalogProviderLike,
): readonly CatalogModelLike[] => {
	const models = provider.models;
	return Array.isArray(models) ? models : [];
};

const isProviderConfiguredInCatalog = (
	provider: CatalogProviderLike,
): boolean => {
	if (getProviderModels(provider).length > 0) {
		return true;
	}
	if (provider.available === true) {
		return true;
	}
	const unavailableReason = asString(provider.unavailable_reason).trim();
	return unavailableReason !== "" && unavailableReason !== "missing_api_key";
};

export const hasConfiguredModelsInCatalog = (
	catalog: ModelCatalogLike | null | undefined,
): boolean => {
	return getCatalogProviders(catalog).some(isProviderConfiguredInCatalog);
};

export const getModelOptionsFromCatalog = (
	catalog: ModelCatalogLike | null | undefined,
	configs?: readonly ChatModelConfigLike[],
): readonly ModelSelectorOption[] => {
	const optionsByID = new Map<string, ModelSelectorOption>();

	// Build a lookup of context limits from admin model configs so
	// we can surface this in the model selector tooltip.
	const contextLimitByKey = new Map<string, number>();
	if (configs) {
		for (const config of configs) {
			const contextLimit = asNumber(config.context_limit);
			if (contextLimit === undefined || contextLimit <= 0) {
				continue;
			}
			const { provider, model } = getNormalizedModelRef(config);
			if (!provider || !model) {
				continue;
			}
			const key = `${provider}:${model}`;
			if (!contextLimitByKey.has(key)) {
				contextLimitByKey.set(key, contextLimit);
			}
		}
	}

	for (const provider of getCatalogProviders(catalog)) {
		const models = getProviderModels(provider);
		if (provider.available !== true || models.length === 0) {
			continue;
		}
		for (const model of models) {
			if (!model) {
				continue;
			}

			const modelID = asString(model.id).trim();
			const { provider: modelProvider, model: modelRef } =
				getNormalizedModelRef(model);
			if (!modelID || !modelProvider || !modelRef) {
				continue;
			}
			if (optionsByID.has(modelID)) {
				continue;
			}

			const configKey = `${modelProvider.toLowerCase()}:${modelRef}`;

			optionsByID.set(modelID, {
				id: modelID,
				provider: modelProvider,
				model: modelRef,
				displayName: asString(model.display_name).trim() || modelRef,
				contextLimit: contextLimitByKey.get(configKey),
			});
		}
	}

	return Array.from(optionsByID.values()).sort((a, b) => {
		const providerCompare = a.provider.localeCompare(b.provider);
		if (providerCompare !== 0) {
			return providerCompare;
		}
		return a.displayName.localeCompare(b.displayName);
	});
};

export const formatProviderLabel = (provider: string): string => {
	const normalized = provider.trim().toLowerCase();
	switch (normalized) {
		case "openai":
			return "OpenAI";
		case "anthropic":
			return "Anthropic";
		case "azure":
			return "Azure OpenAI";
		case "bedrock":
			return "AWS Bedrock";
		case "google":
			return "Google";
		case "openai-compatible":
		case "openai_compatible":
			return "OpenAI-compatible";
		case "openrouter":
			return "OpenRouter";
		case "vercel":
			return "Vercel AI Gateway";
		default:
			if (!normalized) {
				return "Unknown";
			}
			return `${normalized[0].toUpperCase()}${normalized.slice(1)}`;
	}
};

export const getModelSelectorPlaceholder = (
	modelOptions: readonly ModelSelectorOption[],
	isModelCatalogLoading: boolean,
	hasConfiguredModels: boolean,
): string => {
	if (modelOptions.length > 0) {
		return "Select model";
	}
	if (isModelCatalogLoading) {
		return "Loading models...";
	}
	if (hasConfiguredModels) {
		return "No available models";
	}
	return "No models configured";
};

export const getModelCatalogStatusMessage = (
	catalog: TypesGen.ChatModelsResponse | null | undefined,
	modelOptions: readonly ModelSelectorOption[],
	isModelCatalogLoading: boolean,
	hasModelCatalogError: boolean,
): string | null => {
	if (modelOptions.length > 0) {
		return null;
	}
	if (isModelCatalogLoading) {
		return "Loading model catalog...";
	}
	if (hasModelCatalogError) {
		return "Model catalog unavailable. Unable to verify model availability.";
	}
	if (hasConfiguredModelsInCatalog(catalog)) {
		return "Models are configured but unavailable. Check provider settings.";
	}
	return "No chat models are configured. Ask an admin to configure one.";
};
