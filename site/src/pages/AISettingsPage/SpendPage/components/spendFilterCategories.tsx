import { AppWindowIcon, ServerIcon, SparklesIcon } from "lucide-react";
import { API } from "#/api/api";
import type {
	FilterCategory,
	FilterOption,
} from "#/components/Filter/FilterCombobox/types";
import { AIBridgeClientIcon } from "#/pages/AIBridgePage/icons/AIBridgeClientIcon";
import { AIBridgeModelIcon } from "#/pages/AIBridgePage/icons/AIBridgeModelIcon";
import { AIBridgeProviderIcon } from "#/pages/AIBridgePage/icons/AIBridgeProviderIcon";

const OPTIONS_LIMIT = 25;

const matches = (query: string, ...fields: readonly string[]): boolean => {
	const normalized = query.trim().toLowerCase();
	if (normalized.length === 0) {
		return true;
	}
	return fields.some((field) => field.toLowerCase().includes(normalized));
};

// Provider/client/model categories for the spend filter combobox. The chip keys
// (provider_name, client, model) match the spend API and the AI Sessions filter
// so a drill-in's filters hand off to the sessions link unchanged.
export const spendFilterCategories: readonly FilterCategory[] = [
	{
		key: "provider_name",
		label: "Provider",
		aliases: ["provider"],
		icon: <ServerIcon />,
		getOptions: async (query): Promise<FilterOption[]> => {
			const providers = await API.experimental.listAIProviders();
			return providers
				.filter((provider) =>
					matches(query, provider.display_name || provider.name, provider.name),
				)
				.map((provider) => ({
					label: provider.display_name || provider.name,
					value: provider.name,
					startIcon: (
						<AIBridgeProviderIcon
							provider={provider.type}
							className="size-icon-sm"
						/>
					),
				}));
		},
	},
	{
		key: "client",
		label: "Client",
		icon: <AppWindowIcon />,
		getOptions: async (query): Promise<FilterOption[]> => {
			const clients = await API.getAIBridgeClients({
				q: query,
				limit: OPTIONS_LIMIT,
			});
			return clients.map((client) => ({
				label: client,
				value: client,
				startIcon: (
					<AIBridgeClientIcon client={client} className="size-icon-sm" />
				),
			}));
		},
	},
	{
		key: "model",
		label: "Model",
		icon: <SparklesIcon />,
		getOptions: async (query): Promise<FilterOption[]> => {
			const models = await API.getAIBridgeModels({
				q: query,
				limit: OPTIONS_LIMIT,
			});
			return models.map((model) => ({
				label: model,
				value: model,
				startIcon: <AIBridgeModelIcon model={model} className="size-icon-sm" />,
			}));
		},
	},
];
