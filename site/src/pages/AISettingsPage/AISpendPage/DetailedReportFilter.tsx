import {
	BoxIcon,
	CloudCogIcon,
	SquareTerminalIcon,
	UserIcon,
	UsersIcon,
} from "lucide-react";
import { useMemo } from "react";
import { useQueryClient } from "react-query";
import {
	type AISpendReportFilter,
	aiBridgeClients,
	aiBridgeModels,
} from "#/api/queries/aiBridge";
import { aiProvidersIncludingDeleted } from "#/api/queries/aiProviders";
import { groupsByOrganization } from "#/api/queries/groups";
import { organizationMembers } from "#/api/queries/organizations";
import { Avatar } from "#/components/Avatar/Avatar";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import type {
	FilterCategory,
	FilterOption,
} from "#/components/Filter/FilterCombobox/types";
import { parseFilterQuery } from "#/components/Filter/filterQuery";
import { ProviderIcon } from "#/modules/aiModels/ProviderIcon";
import { AIBridgeClientIcon } from "#/pages/AIBridgePage/icons/AIBridgeClientIcon";
import { AIBridgeModelIcon } from "#/pages/AIBridgePage/icons/AIBridgeModelIcon";
import type { OptionsQueryClient } from "#/utils/filters";

// The limit of options a filter category search loads.
const SEARCH_SUGGESTIONS_LIMIT = 25;

/** Parses a filter query into the report filter. Free text is ignored. */
export const toReportFilter = (query: string): AISpendReportFilter => {
	const values = parseFilterQuery(query);
	return {
		username: values.user,
		group_name: values.group,
		provider_name: values.provider,
		model: values.model,
		client: values.client,
	};
};

const getUserOptions = async (
	query: string,
	organizationId: string,
	queryClient: OptionsQueryClient,
): Promise<FilterOption[]> => {
	const { members } = await queryClient.fetchQuery(
		organizationMembers(organizationId, {
			q: query,
			limit: SEARCH_SUGGESTIONS_LIMIT,
		}),
	);
	return members.map((member) => ({
		label: member.username,
		value: member.username,
		startIcon: (
			<Avatar fallback={member.username} src={member.avatar_url} size="sm" />
		),
	}));
};

const getGroupOptions = async (
	query: string,
	organizationId: string,
	queryClient: OptionsQueryClient,
): Promise<FilterOption[]> => {
	const groups = await queryClient.fetchQuery(
		groupsByOrganization(organizationId),
	);
	const normalized = query.trim().toLowerCase();
	return groups
		.filter(
			(group) =>
				group.name.toLowerCase().includes(normalized) ||
				group.display_name.toLowerCase().includes(normalized),
		)
		.map((group) => ({
			label: group.name,
			value: group.name,
			startIcon: (
				<Avatar
					fallback={group.display_name || group.name}
					src={group.avatar_url}
					size="sm"
				/>
			),
		}));
};

const getProviderOptions = async (
	query: string,
	queryClient: OptionsQueryClient,
): Promise<FilterOption[]> => {
	const providers = await queryClient.fetchQuery(aiProvidersIncludingDeleted());
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

const getModelOptions = async (
	query: string,
	queryClient: OptionsQueryClient,
): Promise<FilterOption[]> => {
	const models = await queryClient.fetchQuery(
		aiBridgeModels({ model: query, limit: SEARCH_SUGGESTIONS_LIMIT }),
	);
	return models.map((model) => ({
		label: model,
		value: model,
		startIcon: <AIBridgeModelIcon model={model} className="size-icon-sm" />,
	}));
};

const getClientOptions = async (
	query: string,
	queryClient: OptionsQueryClient,
): Promise<FilterOption[]> => {
	const clients = await queryClient.fetchQuery(
		aiBridgeClients({ q: query, limit: SEARCH_SUGGESTIONS_LIMIT }),
	);
	return clients.map((client) => ({
		label: client,
		value: client,
		startIcon: <AIBridgeClientIcon client={client} className="size-icon-sm" />,
	}));
};

type DetailedReportFilterProps = {
	organizationId: string;
	value: string;
	onChange: (query: string) => void;
};

export const DetailedReportFilter: React.FC<DetailedReportFilterProps> = ({
	organizationId,
	value,
	onChange,
}) => {
	const queryClient = useQueryClient();
	const categories = useMemo<FilterCategory[]>(
		() => [
			{
				key: "user",
				optionsKey: `aiSpendReport.user.${organizationId}`,
				label: "User",
				icon: <UserIcon />,
				singleSelect: true,
				getOptions: (query) =>
					getUserOptions(query, organizationId, queryClient),
			},
			{
				key: "group",
				optionsKey: `aiSpendReport.group.${organizationId}`,
				label: "Group",
				icon: <UsersIcon />,
				singleSelect: true,
				getOptions: (query) =>
					getGroupOptions(query, organizationId, queryClient),
			},
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
				getOptions: (query) => getClientOptions(query, queryClient),
			},
			{
				key: "model",
				optionsKey: "aiSpendReport.model",
				label: "Model",
				icon: <BoxIcon />,
				singleSelect: true,
				getOptions: (query) => getModelOptions(query, queryClient),
			},
		],
		[organizationId, queryClient],
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
