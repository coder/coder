import type { Chat, ChatModel } from "#/api/typesGenerated";
import { isUnsetModelRef } from "../../../utils/modelOptions";
import { asString } from "../../ChatElements/runtimeTypeUtils";

export const getModelDisplayName = (
	lastModelConfigID: Chat["last_model_config_id"] | undefined,
	modelConfigs: readonly ChatModel[],
) => {
	if (isUnsetModelRef(lastModelConfigID)) {
		return "Default model";
	}

	const normalizedModelConfigID = asString(lastModelConfigID).trim();
	const modelConfig = modelConfigs.find(
		(config) => config.id === normalizedModelConfigID,
	);
	if (!modelConfig) {
		return "Unavailable model";
	}

	const displayName = asString(modelConfig.display_name).trim();
	if (displayName) {
		return displayName;
	}

	const model = asString(modelConfig.model).trim();
	return model || "Default model";
};
