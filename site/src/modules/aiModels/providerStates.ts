import type * as TypesGen from "#/api/typesGenerated";
import { normalizeProvider } from "#/modules/aiModels/helpers";
import { formatProviderLabel } from "#/utils/aiProviders";

export type ProviderState = {
	key: string;
	provider: string;
	label: string;
	providerDescriptor: TypesGen.ChatModelProviderDescriptor;
	models: readonly TypesGen.ChatModel[];
	hasEffectiveAPIKey: boolean;
	allowUserAPIKey: boolean;
};

export const deriveProviderStates = (
	modelConfigs: readonly TypesGen.ChatModel[],
	providerDescriptors: readonly TypesGen.ChatModelProviderDescriptor[],
): readonly ProviderState[] => {
	const modelConfigsByProviderID = new Map<string, TypesGen.ChatModel[]>();
	for (const modelConfig of modelConfigs) {
		const existing = modelConfigsByProviderID.get(modelConfig.ai_provider_id);
		if (existing) {
			existing.push(modelConfig);
		} else {
			modelConfigsByProviderID.set(modelConfig.ai_provider_id, [modelConfig]);
		}
	}

	return providerDescriptors
		.map((providerDescriptor) => {
			const provider = normalizeProvider(providerDescriptor.type);
			return {
				key: providerDescriptor.id,
				provider,
				label:
					providerDescriptor.display_name.trim() ||
					formatProviderLabel(provider),
				providerDescriptor,
				models: modelConfigsByProviderID.get(providerDescriptor.id) ?? [],
				hasEffectiveAPIKey: providerDescriptor.has_effective_api_key,
				allowUserAPIKey: providerDescriptor.allow_user_api_key,
			};
		})
		.toSorted((a, b) => a.label.localeCompare(b.label));
};

export const canManageProviderModels = (
	providerState: ProviderState | undefined,
): boolean =>
	Boolean(
		providerState?.providerDescriptor.enabled &&
			(providerState.hasEffectiveAPIKey || providerState.allowUserAPIKey),
	);
