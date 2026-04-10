import type * as TypesGen from "#/api/typesGenerated";

const defaultNow = "2026-02-18T12:00:00.000Z";

export type StoryFixtureOptions = {
	now?: string;
};

export const createProviderConfig = (
	overrides: Partial<TypesGen.ChatProviderConfig> &
		Pick<TypesGen.ChatProviderConfig, "id" | "provider">,
	options: StoryFixtureOptions = {},
): TypesGen.ChatProviderConfig => {
	const now = options.now ?? defaultNow;

	return {
		id: overrides.id,
		provider: overrides.provider,
		display_name: overrides.display_name ?? "",
		enabled: overrides.enabled ?? true,
		has_api_key: overrides.has_api_key ?? false,
		has_effective_api_key:
			overrides.has_effective_api_key ?? overrides.has_api_key ?? false,
		central_api_key_enabled: overrides.central_api_key_enabled ?? true,
		allow_user_api_key: overrides.allow_user_api_key ?? false,
		allow_central_api_key_fallback:
			overrides.allow_central_api_key_fallback ?? false,
		base_url: overrides.base_url ?? "",
		source: overrides.source ?? "database",
		created_at: overrides.created_at ?? now,
		updated_at: overrides.updated_at ?? now,
	};
};

export const createOpenAIProductionStagingPair = (
	productionId: string,
	stagingId: string,
): [TypesGen.ChatProviderConfig, TypesGen.ChatProviderConfig] => [
	createProviderConfig({
		id: productionId,
		provider: "openai",
		display_name: "OpenAI (Production)",
		has_api_key: true,
		has_effective_api_key: true,
		base_url: "https://api.openai.com/v1",
		source: "database",
	}),
	createProviderConfig({
		id: stagingId,
		provider: "openai",
		display_name: "OpenAI (Staging)",
		has_api_key: true,
		has_effective_api_key: true,
		base_url: "https://staging.openai.example.com/v1",
		source: "database",
	}),
];

type ModelProviderAttachmentOverrides = Partial<
	Omit<TypesGen.ChatModelProviderAttachment, "provider_config_id">
>;

export const createModelProviderAttachment = (
	providerConfigId: string,
	overrides: ModelProviderAttachmentOverrides = {},
): TypesGen.ChatModelProviderAttachment => ({
	id: overrides.id ?? `attachment-${providerConfigId}`,
	provider_config_id: providerConfigId,
	provider: overrides.provider ?? "openai",
	priority: overrides.priority ?? 0,
	display_name: overrides.display_name ?? providerConfigId,
	enabled: overrides.enabled ?? true,
	has_api_key: overrides.has_api_key ?? false,
});

export const createModelProviderAttachments = (
	providerConfigs: readonly TypesGen.ChatProviderConfig[],
): TypesGen.ChatModelProviderAttachment[] =>
	providerConfigs.map((providerConfig, priority) =>
		createModelProviderAttachment(providerConfig.id, {
			provider: providerConfig.provider,
			priority,
			display_name:
				providerConfig.display_name ||
				providerConfig.base_url ||
				providerConfig.id,
			enabled: providerConfig.enabled,
			has_api_key: providerConfig.has_api_key,
		}),
	);

export const createModelConfig = (
	overrides: Partial<TypesGen.ChatModelConfig> &
		Pick<TypesGen.ChatModelConfig, "id" | "provider" | "model">,
	options: StoryFixtureOptions = {},
): TypesGen.ChatModelConfig => {
	const now = options.now ?? defaultNow;

	return {
		id: overrides.id,
		provider: overrides.provider,
		provider_configs: overrides.provider_configs ?? [],
		model: overrides.model,
		display_name: overrides.display_name ?? overrides.model,
		enabled: overrides.enabled ?? true,
		is_default: overrides.is_default ?? false,
		context_limit: overrides.context_limit ?? 200000,
		compression_threshold: overrides.compression_threshold ?? 70,
		model_config: overrides.model_config,
		created_at: overrides.created_at ?? now,
		updated_at: overrides.updated_at ?? now,
	};
};
