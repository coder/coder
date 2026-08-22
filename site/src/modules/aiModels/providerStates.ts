import type * as TypesGen from "#/api/typesGenerated";
import { normalizeProvider } from "#/modules/aiModels/helpers";
import { formatProviderLabel } from "#/utils/aiProviders";

export type ProviderState = {
	key: string;
	provider: string;
	label: string;
	providerDescriptor: TypesGen.ChatModelProviderDescriptor;
	models: readonly TypesGen.ChatModel[];
	catalogModelCount: number;
	hasEffectiveAPIKey: boolean;
	allowUserAPIKey: boolean;
};

type GeneratedAvailableProvider =
	TypesGen.ChatModelAvailabilityResponse["providers"][number];
type AvailableProvider = Omit<GeneratedAvailableProvider, "models"> & {
	models: GeneratedAvailableProvider["models"] | null;
};
type ChatModelAvailability = Omit<
	TypesGen.ChatModelAvailabilityResponse,
	"providers"
> & {
	providers: readonly AvailableProvider[];
};

export const deriveProviderStates = (
	modelConfigs: readonly TypesGen.ChatModel[],
	providerDescriptors: readonly TypesGen.ChatModelProviderDescriptor[],
	availability: ChatModelAvailability | null | undefined,
): readonly ProviderState[] => {
	const availableProvidersByType = new Map<string, AvailableProvider>();
	for (const availableProvider of availability?.providers ?? []) {
		availableProvidersByType.set(
			normalizeProvider(availableProvider.provider),
			availableProvider,
		);
	}

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
			const availableProvider = availableProvidersByType.get(provider);
			return {
				key: providerDescriptor.id,
				provider,
				label:
					providerDescriptor.display_name.trim() ||
					formatProviderLabel(provider),
				providerDescriptor,
				models: modelConfigsByProviderID.get(providerDescriptor.id) ?? [],
				catalogModelCount: availableProvider?.models?.length ?? 0,
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
