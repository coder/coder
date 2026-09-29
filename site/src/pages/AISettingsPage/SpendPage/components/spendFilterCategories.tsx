import { AppWindowIcon, ServerIcon, SparklesIcon } from "lucide-react";
import { API } from "#/api/api";
import type {
	FilterCategory,
	FilterOption,
} from "#/components/Filter/FilterCombobox/types";
import { ProviderIcon } from "#/modules/aiModels/ProviderIcon";
import { AIBridgeClientIcon } from "#/pages/AIBridgePage/icons/AIBridgeClientIcon";
import { AIBridgeModelIcon } from "#/pages/AIBridgePage/icons/AIBridgeModelIcon";

const OPTIONS_LIMIT = 25;

const matches = (query: string, ...fields: readonly string[]): boolean => {
	const normalized = query.trim().toLowerCase();
	if (normalized.length === 0) {
		return true;
	}
	return fields.some((field) => field.toLowerCase().includes(normalized));
};

/** Dimension options loaded by the combobox's React Query integration. */
export const spendFilterCategories: readonly FilterCategory[] = [
	{
		key: "provider_name",
		label: "Provider",
		aliases: ["provider"],
		icon: <ServerIcon />,
		getOptions: async (query): Promise<FilterOption[]> => {
			const providers = await API.getAIBridgeProviders();
			return providers
				.filter((provider) =>
					matches(query, provider.display_name || provider.name, provider.name),
				)
				.map((provider) => ({
					label: provider.display_name || provider.name,
					value: provider.name,
					startIcon: (
						<ProviderIcon
							provider={provider.type}
							icon={provider.icon}
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
				model: query,
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
