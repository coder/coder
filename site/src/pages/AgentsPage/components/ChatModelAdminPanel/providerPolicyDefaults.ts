import type * as TypesGen from "#/api/typesGenerated";

type ProviderPolicyFields = Pick<
	TypesGen.ChatProviderConfig,
	| "central_api_key_enabled"
	| "allow_user_api_key"
	| "allow_central_api_key_fallback"
>;

export type ProviderConfigWithOptionalPolicyFields = Omit<
	TypesGen.ChatProviderConfig,
	keyof ProviderPolicyFields
> &
	Partial<ProviderPolicyFields>;

export function normalizeProviderPolicyDefaults(
	providerConfig: ProviderConfigWithOptionalPolicyFields,
): TypesGen.ChatProviderConfig {
	return {
		...providerConfig,
		central_api_key_enabled: providerConfig.central_api_key_enabled ?? true,
		allow_user_api_key: providerConfig.allow_user_api_key ?? false,
		allow_central_api_key_fallback:
			providerConfig.allow_central_api_key_fallback ?? false,
	};
}
