import type * as TypesGen from "#/api/typesGenerated";

type UpdateModelConfig = (
	modelConfigId: string,
	req: TypesGen.UpdateChatModelConfigRequest,
) => Promise<unknown>;

export const cascadeDisableProviderModels = async ({
	associatedModels,
	allModels,
	updateModelConfig,
}: {
	associatedModels: readonly TypesGen.ChatModelConfig[];
	allModels: readonly TypesGen.ChatModelConfig[];
	updateModelConfig: UpdateModelConfig;
}) => {
	const disabledIds = new Set(associatedModels.map((model) => model.id));
	const hadDefault = associatedModels.some((model) => model.is_default);

	for (const model of associatedModels) {
		await updateModelConfig(model.id, { enabled: false });
	}

	if (!hadDefault) {
		return;
	}

	const newDefault = allModels.find(
		(model) => model.enabled && !disabledIds.has(model.id),
	);
	if (newDefault) {
		await updateModelConfig(newDefault.id, { is_default: true });
	}
};
