import { BoxIcon, BrainIcon, MonitorIcon, UserIcon } from "lucide-react";
import { type FC, useMemo } from "react";
import { type QueryClient, useQueryClient } from "react-query";
import { API } from "#/api/api";
import { organizationMembers } from "#/api/queries/organizations";
import { MaxAISpendPeriodDays, type Organization } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { DateTimeRangePicker } from "#/components/DateTimeRangePicker/DateTimeRangePicker";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import type {
	FilterCategory,
	FilterOption,
} from "#/components/Filter/FilterCombobox/types";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { ProviderIcon } from "#/modules/aiModels/ProviderIcon";
import { AIBridgeClientIcon } from "#/pages/AIBridgePage/icons/AIBridgeClientIcon";
import { AIBridgeModelIcon } from "#/pages/AIBridgePage/icons/AIBridgeModelIcon";
import { spendQuickPresets } from "../spendPeriod";

const OPTIONS_LIMIT = 25;

type SpendFiltersProps = {
	organizations: readonly Organization[];
	organization: Organization;
	onOrganizationChange: (organization: Organization) => void;
	filterQuery: string;
	onFilterQueryChange: (query: string) => void;
	showDimensionFilters: boolean;
	now: Date | undefined;
	period: DateTimeRangeValue;
	minDate: Date | undefined;
	onPeriodChange: (value: DateTimeRangeValue) => void;
};

export const SpendFilters: FC<SpendFiltersProps> = ({
	organizations,
	organization,
	onOrganizationChange,
	filterQuery,
	onFilterQueryChange,
	showDimensionFilters,
	now,
	period,
	minDate,
	onPeriodChange,
}) => {
	const queryClient = useQueryClient();
	const categories = useMemo(
		() =>
			spendFilterCategories({
				organizationId: organization.id,
				showDimensionFilters,
				queryClient,
			}),
		[organization.id, showDimensionFilters, queryClient],
	);

	return (
		<div className="flex flex-col gap-2">
			<div className="flex flex-wrap gap-2">
				{organizations.length > 1 && (
					<OrganizationAutocomplete
						value={organization}
						ariaLabel={`Organization ${getOrganizationLabel(
							organization,
							organizations,
						)}`}
						options={organizations}
						triggerClassName="basis-[150px] grow"
						optionsTabbable
						onChange={(next) => {
							if (next) {
								onOrganizationChange(next);
							}
						}}
					/>
				)}
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
			<FilterCombobox
				value={filterQuery}
				onChange={onFilterQueryChange}
				categories={categories}
				placeholder="Search and filter users…"
				className="max-w-lg"
				allowFreeText={false}
			/>
		</div>
	);
};

type SpendFilterCategoriesOptions = {
	organizationId: string;
	showDimensionFilters: boolean;
	queryClient: QueryClient;
};

const spendFilterCategories = ({
	organizationId,
	showDimensionFilters,
	queryClient,
}: SpendFilterCategoriesOptions): FilterCategory[] => {
	const userCategory: FilterCategory = {
		key: "user",
		label: "User",
		icon: <UserIcon />,
		getOptions: async (query) => {
			const response = await queryClient.fetchQuery(
				organizationMembers(organizationId, {
					q: query,
					limit: OPTIONS_LIMIT,
				}),
			);
			return response.members.map(
				(member): FilterOption => ({
					value: member.username,
					label: member.username,
					subtitle: member.name || undefined,
					startIcon: (
						<Avatar
							fallback={member.username}
							src={member.avatar_url}
							size="md"
						/>
					),
				}),
			);
		},
	};
	if (!showDimensionFilters) {
		return [userCategory];
	}

	return [
		userCategory,
		{
			key: "provider",
			label: "Provider",
			icon: <BoxIcon />,
			getOptions: async () => {
				const providers = await API.getAIBridgeProviders();
				return providers.map(
					(provider): FilterOption => ({
						value: provider.name,
						label: provider.display_name || provider.name,
						startIcon: (
							<ProviderIcon
								provider={provider.type}
								icon={provider.icon}
								className="size-icon-sm"
							/>
						),
					}),
				);
			},
		},
		{
			key: "model",
			label: "Model",
			icon: <BrainIcon />,
			getOptions: async (query) => {
				const models = await API.getAIBridgeModels({
					model: query,
					limit: OPTIONS_LIMIT,
				});
				return models.map(
					(model): FilterOption => ({
						value: model,
						label: model,
						startIcon: (
							<AIBridgeModelIcon model={model} className="size-icon-sm" />
						),
					}),
				);
			},
		},
		{
			key: "client",
			label: "Client",
			icon: <MonitorIcon />,
			getOptions: async (query) => {
				const clients = await API.getAIBridgeClients({
					q: query,
					limit: OPTIONS_LIMIT,
				});
				return clients.map(
					(client): FilterOption => ({
						value: client,
						label: client,
						startIcon: (
							<AIBridgeClientIcon client={client} className="size-icon-sm" />
						),
					}),
				);
			},
		},
	];
};
