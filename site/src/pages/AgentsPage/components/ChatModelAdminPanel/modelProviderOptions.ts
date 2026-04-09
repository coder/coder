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
			const baseLabel =
				config.display_name || config.base_url || config.id.slice(0, 8);
			const label =
				qualifyingConfigs.length === 1
					? baseLabel
					: `${baseLabel} (${index + 1})`;

			options.push({
				key: `${state.provider}:${config.id}`,
				provider: state.provider,
				label,
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
