import {
	AppWindowIcon,
	BoxIcon,
	Building2Icon,
	CloudIcon,
	DownloadIcon,
	UserIcon,
	UsersIcon,
} from "lucide-react";
import { type FC, useMemo } from "react";
import { type QueryClient, useQueryClient } from "react-query";
import {
	aiBridgeClients,
	aiBridgeModels,
	aiBridgeProviders,
} from "#/api/queries/aiBridge";
import { groupsByOrganization } from "#/api/queries/groups";
import { users } from "#/api/queries/users";
import { MaxAISpendPeriodDays, type Organization } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import { DateTimeRangePicker } from "#/components/DateTimeRangePicker/DateTimeRangePicker";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import type {
	FilterCategory,
	FilterOption,
} from "#/components/Filter/FilterCombobox/types";
import { getOrganizationLabel } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { Spinner } from "#/components/Spinner/Spinner";
import { ProviderIcon } from "#/modules/aiModels/ProviderIcon";
import { AIBridgeClientIcon } from "#/pages/AIBridgePage/icons/AIBridgeClientIcon";
import { AIBridgeModelIcon } from "#/pages/AIBridgePage/icons/AIBridgeModelIcon";
import { spendQuickPresets } from "../spendPeriod";

const OPTIONS_LIMIT = 25;

type SpendFiltersProps = {
	organizations: readonly Organization[];
	organization: Organization;
	filterQuery: string;
	onFilterQueryChange: (query: string) => void;
	canFilterDimensions: boolean;
	now: Date | undefined;
	period: DateTimeRangeValue;
	minDate: Date | undefined;
	onPeriodChange: (value: DateTimeRangeValue) => void;
	onExportCSV: () => void;
	isExportingCSV: boolean;
};

export const SpendFilters: FC<SpendFiltersProps> = ({
	organizations,
	organization,
	filterQuery,
	onFilterQueryChange,
	canFilterDimensions,
	now,
	period,
	minDate,
	onPeriodChange,
	onExportCSV,
	isExportingCSV,
}) => {
	const queryClient = useQueryClient();
	const categories = useMemo(
		() =>
			buildSpendFilterCategories({
				organizations,
				organizationName: organization.name,
				canFilterDimensions,
				queryClient,
			}),
		[organizations, organization.name, canFilterDimensions, queryClient],
	);

	return (
		<div className="flex flex-wrap items-start gap-2">
			<div className="flex w-full min-w-0 max-w-full flex-col sm:w-auto">
				<FilterCombobox
					value={filterQuery}
					onChange={onFilterQueryChange}
					categories={categories}
					placeholder="Search and filter users…"
					className="w-full min-w-0 self-start sm:w-auto sm:min-w-lg sm:max-w-full"
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
			<Button
				variant="outline"
				size="lg"
				className="shrink-0"
				disabled={isExportingCSV}
				onClick={onExportCSV}
			>
				<Spinner loading={isExportingCSV}>
					<DownloadIcon />
				</Spinner>
				Export CSV
			</Button>
		</div>
	);
};

const matchesQuery = (query: string, ...fields: readonly string[]) => {
	const normalized = query.trim().toLowerCase();
	return (
		normalized.length === 0 ||
		fields.some((field) => field.toLowerCase().includes(normalized))
	);
};

const unconfiguredPricingOption: FilterOption = {
	label: "Uses models with unconfigured pricing",
	value: "unconfigured",
};

type BuildSpendFilterCategoriesOptions = Readonly<{
	organizations: readonly Organization[];
	organizationName: string;
	canFilterDimensions: boolean;
	queryClient: Pick<QueryClient, "fetchQuery">;
}>;

const buildSpendFilterCategories = ({
	organizations,
	organizationName,
	canFilterDimensions,
	queryClient,
}: BuildSpendFilterCategoriesOptions): readonly FilterCategory[] => {
	const user: FilterCategory = {
		key: "user",
		label: "User",
		icon: <UserIcon />,
		getOptions: async (query) => {
			const usersRes = await queryClient.fetchQuery(
				users({ q: query, limit: OPTIONS_LIMIT }),
			);
			return usersRes.users.map((user) => ({
				label: user.username,
				value: user.username,
				startIcon: (
					<Avatar fallback={user.username} src={user.avatar_url} size="sm" />
				),
			}));
		},
	};

	const group: FilterCategory = {
		key: "group",
		label: "Group",
		icon: <UsersIcon />,
		getOptions: async (query) => {
			const groups = await queryClient.fetchQuery(
				groupsByOrganization(organizationName),
			);
			return groups
				.filter((group) => matchesQuery(query, group.name, group.display_name))
				.map((group) => ({
					label: group.display_name || group.name,
					value: group.name,
					startIcon: (
						<Avatar
							fallback={group.display_name || group.name}
							src={group.avatar_url}
							size="sm"
						/>
					),
				}));
		},
	};

	// Provider, model, and client options come from deployment-wide AI Gateway
	// endpoints, so only viewers of every session get those categories.
	const dimensions: FilterCategory[] = canFilterDimensions
		? [
				{
					key: "provider",
					label: "Provider",
					icon: <CloudIcon />,
					getOptions: async (query) => {
						const providers = await queryClient.fetchQuery(aiBridgeProviders());
						return providers
							.filter((provider) =>
								matchesQuery(query, provider.name, provider.display_name),
							)
							.map((provider) => ({
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
					icon: <BoxIcon />,
					getOptions: async (query) => {
						const models = await queryClient.fetchQuery(
							aiBridgeModels({ model: query, limit: OPTIONS_LIMIT }),
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
					icon: <AppWindowIcon />,
					getOptions: async (query) => {
						const clients = await queryClient.fetchQuery(
							aiBridgeClients({ q: query, limit: OPTIONS_LIMIT }),
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
			]
		: [];

	const organization: FilterCategory = {
		key: "org",
		label: "Organization",
		icon: <Building2Icon />,
		hideWhenSingleOption: true,
		getOptions: async (query) =>
			organizations
				.filter((organization) =>
					matchesQuery(query, organization.name, organization.display_name),
				)
				.map((organization) => ({
					label: getOrganizationLabel(organization, organizations),
					value: organization.name,
					startIcon: (
						<Avatar
							fallback={organization.display_name || organization.name}
							src={organization.icon}
							size="sm"
						/>
					),
				})),
	};

	const pricing: FilterCategory = {
		key: "pricing",
		label: "Pricing",
		inlineOptions: true,
		inlineOptionsLabel: "",
		// Icon-less rows otherwise always render in the primary color; this keeps
		// the row secondary until it is selected.
		inlineOptionIcons: true,
		getOptions: async (query) =>
			matchesQuery(query, unconfiguredPricingOption.label)
				? [unconfiguredPricingOption]
				: [],
	};

	return [user, group, ...dimensions, organization, pricing];
};
