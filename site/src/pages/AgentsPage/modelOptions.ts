import type { ChatModelsResponse } from "api/api";
import type { ModelSelectorOption } from "components/ai-elements";

type CatalogProvider = ChatModelsResponse["providers"][number];

const getCatalogProviders = (
	catalog: ChatModelsResponse | null | undefined,
): readonly CatalogProvider[] => {
	const providers = catalog?.providers;
	return Array.isArray(providers) ? providers : [];
};

const getProviderModels = (
	provider: CatalogProvider,
): readonly CatalogProvider["models"][number][] => {
	const models = provider.models;
	return Array.isArray(models) ? models : [];
};

const isProviderConfiguredInCatalog = (provider: CatalogProvider): boolean => {
	if (getProviderModels(provider).length > 0) {
		return true;
	}
	if (provider.available) {
		return true;
	}
	return (
		Boolean(provider.unavailable_reason) &&
		provider.unavailable_reason !== "missing_api_key"
	);
};

export const hasConfiguredModelsInCatalog = (
	catalog: ChatModelsResponse | null | undefined,
): boolean => {
	return getCatalogProviders(catalog).some(isProviderConfiguredInCatalog);
};

export const getModelOptionsFromCatalog = (
	catalog: ChatModelsResponse | null | undefined,
): readonly ModelSelectorOption[] => {
	const optionsByID = new Map<string, ModelSelectorOption>();

	for (const provider of getCatalogProviders(catalog)) {
		const models = getProviderModels(provider);
		if (!provider.available || models.length === 0) {
			continue;
		}
		for (const model of models) {
			if (!model) {
				continue;
			}

			const modelID = model.id.trim();
			const modelProvider = model.provider.trim();
			const modelRef = model.model.trim();
			if (!modelID || !modelProvider || !modelRef) {
				continue;
			}
			if (optionsByID.has(modelID)) {
				continue;
			}

			optionsByID.set(modelID, {
				id: modelID,
				provider: modelProvider,
				model: modelRef,
				displayName:
					(typeof model.display_name === "string" &&
						model.display_name.trim()) ||
					modelRef,
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
	catalog: ChatModelsResponse | null | undefined,
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
