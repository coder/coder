import type { Chat, ChatModel } from "#/api/typesGenerated";
import type { ModelSelectorOption } from "../../ChatElements";
import { asString } from "../../ChatElements/runtimeTypeUtils";

export const getModelDisplayName = (
	lastModelConfigID: Chat["last_model_config_id"] | undefined,
	models: readonly ChatModel[],
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

	const modelConfig = models.find(
		(config) => config.id === normalizedModelConfigID,
	);
	if (!modelConfig) {
		return "Default model";
	}

	const displayName = asString(modelConfig.display_name).trim();
	if (displayName) {
		return displayName;
	}

	const model = asString(modelConfig.model).trim();
	return model || "Default model";
};
