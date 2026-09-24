import { type FC, useMemo } from "react";
import { type QueryClient, useQueryClient } from "react-query";
import {
	aiBridgeClients,
	aiBridgeModels,
	aiBridgeProviders,
} from "#/api/queries/aiBridge";
import { users } from "#/api/queries/users";
import { MaxAISpendPeriodDays, type Organization } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { DateTimeRangePicker } from "#/components/DateTimeRangePicker/DateTimeRangePicker";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import type { FilterCategory } from "#/components/Filter/FilterCombobox/types";
import { getOrganizationLabel } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { ProviderIcon } from "#/modules/aiModels/ProviderIcon";
import { AIBridgeClientIcon } from "#/pages/AIBridgePage/icons/AIBridgeClientIcon";
import { AIBridgeModelIcon } from "#/pages/AIBridgePage/icons/AIBridgeModelIcon";
import { spendQuickPresets } from "../spendPeriod";

type SpendFiltersProps = {
	organizations: readonly Organization[];
	filterQuery: string;
	onFilterQueryChange: (query: string) => void;
	canFilterDimensions: boolean;
	now: Date | undefined;
	period: DateTimeRangeValue;
	minDate: Date | undefined;
	onPeriodChange: (value: DateTimeRangeValue) => void;
};

export const SpendFilters: FC<SpendFiltersProps> = ({
	organizations,
	filterQuery,
	onFilterQueryChange,
	canFilterDimensions,
	now,
	period,
	minDate,
	onPeriodChange,
}) => {
	const queryClient = useQueryClient();
	const categories = useMemo(
		() =>
			buildSpendFilterCategories(
				organizations,
				canFilterDimensions,
				queryClient,
			),
		[organizations, canFilterDimensions, queryClient],
	);

	return (
		<div className="flex flex-wrap gap-2 lg:flex-nowrap">
			<div className="w-full min-w-0">
				<FilterCombobox
					value={filterQuery}
					onChange={onFilterQueryChange}
					categories={categories}
					placeholder="Search and filter users…"
				/>
			</div>
			<div className="shrink-0">
				<DateTimeRangePicker
					now={now}
					value={period}
					onChange={onPeriodChange}
					presets={spendQuickPresets}
					maxDays={MaxAISpendPeriodDays}
					minDate={minDate}
					size="lg"
				/>
			</div>
		</div>
	);
};

const buildSpendFilterCategories = (
	organizations: readonly Organization[],
	canFilterDimensions: boolean,
	queryClient: Pick<QueryClient, "fetchQuery">,
): readonly FilterCategory[] => {
	const categories: FilterCategory[] = [
		{
			key: "org",
			label: "Organization",
			getOptions: async (query) => {
				const normalizedQuery = query.trim().toLowerCase();
				return organizations
					.filter((organization) => {
						if (!normalizedQuery) {
							return true;
						}
						return [organization.name, organization.display_name]
							.join(" ")
							.toLowerCase()
							.includes(normalizedQuery);
					})
					.map((organization) => ({
						label: getOrganizationLabel(organization, organizations),
						value: organization.name,
					}));
			},
		},
		{
			key: "user",
			label: "User",
			getOptions: async (query) => {
				const usersRes = await queryClient.fetchQuery(
					users({ q: query, limit: 25 }),
				);
				return usersRes.users.map((user) => ({
					label: user.username,
					value: user.username,
					startIcon: (
						<Avatar fallback={user.username} src={user.avatar_url} size="sm" />
					),
					subtitle: user.name,
				}));
			},
		},
		{
			key: "pricing",
			label: "Pricing",
			getOptions: async (query) => {
				const option = {
					label: "Models with unconfigured pricing",
					value: "unconfigured",
					subtitle: "Users with usage excluded from spend",
				};
				return option.label.toLowerCase().includes(query.trim().toLowerCase())
					? [option]
					: [];
			},
		},
	];

	if (!canFilterDimensions) {
		return categories;
	}

	return [
		...categories,
		{
			key: "provider",
			label: "Provider",
			getOptions: async () => {
				const providers = await queryClient.fetchQuery(aiBridgeProviders());
				return providers.map((provider) => ({
					value: provider.name,
					label: provider.display_name || provider.name,
					startIcon: (
						<ProviderIcon provider={provider.type} icon={provider.icon} />
					),
				}));
			},
		},
		{
			key: "model",
			label: "Model",
			getOptions: async (query) => {
				const models = await queryClient.fetchQuery(
					aiBridgeModels({ model: query, limit: 25 }),
				);
				return models.map((model) => ({
					label: model,
					value: model,
					startIcon: (
						<AIBridgeModelIcon model={model} className="size-icon-sm" />
					),
				}));
			},
		},
		{
			key: "client",
			label: "Client",
			getOptions: async (query) => {
				const clients = await queryClient.fetchQuery(
					aiBridgeClients({ q: query, limit: 25 }),
				);
				return clients.map((client) => ({
					label: client,
					value: client,
					startIcon: (
						<AIBridgeClientIcon client={client} className="size-icon-sm" />
					),
				}));
			},
		},
	];
};
