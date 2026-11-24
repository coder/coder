import {
	aiBridgeClients,
	aiBridgeModels,
	aiBridgeProviders,
} from "#/api/queries/aiBridge";
import { filterOptionsByText } from "#/components/Filter/FilterCombobox/filterQuery";
import type { FilterOption } from "#/components/Filter/FilterCombobox/types";
import { ProviderIcon } from "#/modules/aiModels/ProviderIcon";
import type { OptionsQueryClient } from "#/utils/filters";
import { AIBridgeClientIcon } from "../icons/AIBridgeClientIcon";
import { AIBridgeModelIcon } from "../icons/AIBridgeModelIcon";

const SUGGESTIONS_LIMIT = 25;

export const getProviderFilterOptions = async (
	query: string,
	queryClient: OptionsQueryClient,
): Promise<FilterOption[]> => {
	// The providers endpoint has no search, so filter here.
	const providers = await queryClient.fetchQuery(aiBridgeProviders());
	const options = providers.map((provider) => ({
		label: provider.display_name || provider.name,
		value: provider.name,
		startIcon: <ProviderIcon provider={provider.type} icon={provider.icon} />,
	}));
	return [...filterOptionsByText(options, query)];
};

export const getClientFilterOptions = async (
	query: string,
	queryClient: OptionsQueryClient,
): Promise<FilterOption[]> => {
	const clients = await queryClient.fetchQuery(
		aiBridgeClients({ q: query, limit: SUGGESTIONS_LIMIT }),
	);
	return clients.map((client) => ({
		label: client,
		value: client,
		startIcon: <AIBridgeClientIcon client={client} className="size-icon-sm" />,
	}));
};

export const getModelFilterOptions = async (
	query: string,
	queryClient: OptionsQueryClient,
): Promise<FilterOption[]> => {
	const models = await queryClient.fetchQuery(
		aiBridgeModels({ model: query, limit: SUGGESTIONS_LIMIT }),
	);
	return models.map((model) => ({
		label: model,
		value: model,
		startIcon: <AIBridgeModelIcon model={model} className="size-icon-sm" />,
	}));
};
