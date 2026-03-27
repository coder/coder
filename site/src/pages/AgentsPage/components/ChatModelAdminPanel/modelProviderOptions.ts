import type { ChatProviderConfig } from "#/api/typesGenerated";
import { formatProviderLabel } from "../../utils/modelOptions";
import type { ProviderState } from "./ChatModelAdminPanel";
import { readOptionalString } from "./helpers";

const nilUUID = "00000000-0000-0000-0000-000000000000";

/**
 * A selectable provider option for model creation.
 */
export type ModelProviderOption = {
	key: string;
	provider: string;
	label: string;
	iconProvider: string;
};

const getQualifyingDatabaseConfigs = (
	providerState: ProviderState,
): readonly ChatProviderConfig[] => {
	return providerState.providerConfigs.filter(
		(config) =>
			config.source === "database" &&
			config.id !== nilUUID &&
			config.enabled === true &&
			config.has_api_key === true,
	);
};

/**
 * Builds the add-model provider options from provider state.
 */
export function buildModelProviderOptions(
	providerStates: readonly ProviderState[],
): ModelProviderOption[] {
	const options: ModelProviderOption[] = [];

	for (const providerState of providerStates) {
		const qualifyingConfigs = getQualifyingDatabaseConfigs(providerState);
		if (qualifyingConfigs.length > 0) {
			const baseLabel = formatProviderLabel(providerState.provider);
			for (const [index, config] of qualifyingConfigs.entries()) {
				const displayName = readOptionalString(config.display_name);
				const label =
					displayName ??
					(qualifyingConfigs.length === 1
						? baseLabel
						: `${baseLabel} ${index + 1}`);
				options.push({
					key: config.id,
					provider: providerState.provider,
					label,
					iconProvider: providerState.provider,
				});
			}
			continue;
		}

		if (providerState.isEnvPreset && providerState.hasEffectiveAPIKey) {
			options.push({
				key: `env:${providerState.provider}`,
				provider: providerState.provider,
				label: formatProviderLabel(providerState.provider),
				iconProvider: providerState.provider,
			});
		}
	}

	return options;
}

/**
 * Resolves the default provider option for a selected provider family.
 */
export function resolveDefaultOption(
	options: readonly ModelProviderOption[],
	provider: string | null,
): ModelProviderOption | undefined {
	if (provider !== null) {
		return options.find((option) => option.provider === provider);
	}

	return options[0];
}
