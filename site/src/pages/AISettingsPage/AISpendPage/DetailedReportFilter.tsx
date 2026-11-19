import { BoxIcon, CloudCogIcon, SquareTerminalIcon } from "lucide-react";
import { useMemo } from "react";
import { useQueryClient } from "react-query";
import { API } from "#/api/api";
import { aiBridgeProviders } from "#/api/queries/aiBridge";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import type {
	FilterCategory,
	FilterOption,
} from "#/components/Filter/FilterCombobox/types";
import { ProviderIcon } from "#/modules/aiModels/ProviderIcon";
import { AIBridgeClientIcon } from "#/pages/AIBridgePage/icons/AIBridgeClientIcon";
import { AIBridgeModelIcon } from "#/pages/AIBridgePage/icons/AIBridgeModelIcon";
import type { OptionsQueryClient } from "#/utils/filters";

const OPTIONS_LIMIT = 25;

const getProviderOptions = async (
	query: string,
	queryClient: OptionsQueryClient,
): Promise<FilterOption[]> => {
	const providers = await queryClient.fetchQuery(aiBridgeProviders());
	const normalized = query.trim().toLowerCase();
	return providers
		.filter(
			(provider) =>
				provider.name.toLowerCase().includes(normalized) ||
				provider.display_name.toLowerCase().includes(normalized),
		)
		.map((provider) => ({
			label: provider.name,
			value: provider.name,
			startIcon: <ProviderIcon provider={provider.type} icon={provider.icon} />,
		}));
};

const getModelOptions = async (query: string): Promise<FilterOption[]> => {
	const models = await API.getAIBridgeModels({
		model: query,
		limit: OPTIONS_LIMIT,
	});
	return models.map((model) => ({
		label: model,
		value: model,
		startIcon: <AIBridgeModelIcon model={model} className="size-icon-sm" />,
	}));
};

const getClientOptions = async (query: string): Promise<FilterOption[]> => {
	const clients = await API.getAIBridgeClients({
		q: query,
		limit: OPTIONS_LIMIT,
	});
	return clients.map((client) => ({
		label: client,
		value: client,
		startIcon: <AIBridgeClientIcon client={client} className="size-icon-sm" />,
	}));
};

type DetailedReportFilterProps = {
	value: string;
	onChange: (query: string) => void;
};

export const DetailedReportFilter: React.FC<DetailedReportFilterProps> = ({
	value,
	onChange,
}) => {
	const queryClient = useQueryClient();
	const categories = useMemo<FilterCategory[]>(
		() => [
			{
				key: "provider",
				optionsKey: "aiSpendReport.provider",
				label: "Provider",
				icon: <CloudCogIcon />,
				singleSelect: true,
				getOptions: (query) => getProviderOptions(query, queryClient),
			},
			{
				key: "client",
				optionsKey: "aiSpendReport.client",
				label: "Client",
				icon: <SquareTerminalIcon />,
				singleSelect: true,
				getOptions: getClientOptions,
			},
			{
				key: "model",
				optionsKey: "aiSpendReport.model",
				label: "Model",
				icon: <BoxIcon />,
				singleSelect: true,
				getOptions: getModelOptions,
			},
		],
		[queryClient],
	);

	return (
		<div className="flex w-full min-w-0 max-w-full flex-col sm:w-auto">
			<FilterCombobox
				value={value}
				onChange={onChange}
				categories={categories}
				placeholder="Search and filter…"
				className="w-full min-w-0 self-start sm:w-auto sm:min-w-sm sm:max-w-full"
			/>
		</div>
	);
};
