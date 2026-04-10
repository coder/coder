import { formatProviderLabel } from "../../utils/modelOptions";
import type { ProviderState } from "./ChatModelAdminPanel";

export interface ModelProviderOption {
	key: string;
	provider: string;
	label: string;
	configId: string;
	iconProvider: string;
	isEnvFallback: boolean;
}

const nilProviderConfigId = "00000000-0000-0000-0000-000000000000";

export const formatProviderConfigLabel = (
	config: {
		display_name?: string | null;
		base_url?: string | null;
		id?: string;
		provider_config_id?: string;
	},
	suffix = "",
): string =>
	`${config.display_name || config.base_url || (config.id ?? config.provider_config_id ?? "").slice(0, 8)}${suffix}`;

export const buildModelProviderOptions = (
	providerStates: readonly ProviderState[],
): ModelProviderOption[] => {
	const options: ModelProviderOption[] = [];

	for (const state of providerStates) {
		const qualifyingConfigs = state.providerConfigs.filter(
			(config) => config.id !== nilProviderConfigId && config.enabled,
		);

		if (qualifyingConfigs.length === 0) {
			options.push({
				key: `${state.provider}:env`,
				provider: state.provider,
				label: formatProviderLabel(state.provider),
				configId: "",
				iconProvider: state.provider,
				isEnvFallback: true,
			});
			continue;
		}

		for (const [index, config] of qualifyingConfigs.entries()) {
			options.push({
				key: `${state.provider}:${config.id}`,
				provider: state.provider,
				label: formatProviderConfigLabel(
					config,
					qualifyingConfigs.length > 1 ? ` (${index + 1})` : "",
				),
				configId: config.id,
				iconProvider: state.provider,
				isEnvFallback: false,
			});
		}
	}

	return options;
};

export const resolveDefaultOption = (
	options: readonly ModelProviderOption[],
	provider: string,
): ModelProviderOption | undefined => {
	return options.find((option) => option.provider === provider);
};
