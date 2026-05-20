import type { Chat, ChatModelConfig } from "#/api/typesGenerated";
import { getNormalizedModelRef } from "../../../utils/modelOptions";
import type { ModelSelectorOption } from "../../ChatElements";
import { asString } from "../../ChatElements/runtimeTypeUtils";

export const getModelDisplayName = (
	lastModelConfigID: Chat["last_model_config_id"] | undefined,
	modelConfigs: readonly ChatModelConfig[],
	modelOptions: readonly ModelSelectorOption[],
) => {
	const normalizedModelConfigID = asString(lastModelConfigID).trim();
	if (!normalizedModelConfigID) {
		return "Default model";
	}

	const modelOption = modelOptions.find(
		(option) => option.id === normalizedModelConfigID,
	);
	if (modelOption?.displayName) {
		return modelOption.displayName;
	}

	const modelConfig = modelConfigs.find(
		(config) => config.id === normalizedModelConfigID,
	);
	if (!modelConfig) {
		const legacyModelOption = modelOptions.find(
			(option) =>
				`${option.provider}:${option.model}` === normalizedModelConfigID,
		);
		if (legacyModelOption?.displayName) {
			return legacyModelOption.displayName;
		}
		return "Default model";
	}

	const displayName = asString(modelConfig.display_name).trim();
	if (displayName) {
		return displayName;
	}

	const { provider, model } = getNormalizedModelRef(modelConfig);
	if (!provider || !model) {
		return "Default model";
	}

	const fallbackModelOption = modelOptions.find(
		(option) => option.provider === provider && option.model === model,
	);
	if (fallbackModelOption?.displayName) {
		return fallbackModelOption.displayName;
	}

	return model;
};
