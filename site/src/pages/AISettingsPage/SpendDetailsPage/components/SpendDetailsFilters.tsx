import type { FC } from "react";
import { API } from "#/api/api";
import type { Organization } from "#/api/typesGenerated";
import { ComboboxInput } from "#/components/Combobox/Combobox";
import {
	DateRangePicker,
	type DateRangeValue,
} from "#/components/DateRangePicker/DateRangePicker";
import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "#/components/Filter/menu";
import {
	SelectFilter,
	type SelectFilterOption,
} from "#/components/Filter/SelectFilter";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import {
	ModelFilter,
	type ModelFilterMenu,
	useModelFilterMenu,
} from "#/pages/AIBridgePage/filters/ModelFilter";
import {
	ProviderFilter,
	type ProviderFilterMenu,
	useProviderFilterMenu,
} from "#/pages/AIBridgePage/filters/ProviderFilter";

const FILTER_WIDTH = 170;
const MAX_SPEND_PERIOD_DAYS = 31;

type OrganizationFilterMenu = ReturnType<typeof useFilterMenu>;

const fallbackOption = (value: string): SelectFilterOption => ({
	label: value,
	value,
});

const useOrganizationMemberFilterMenu = ({
	organization,
	value,
	onChange,
	enabled,
}: Pick<UseFilterMenuOptions, "value" | "onChange" | "enabled"> & {
	organization: string;
}) =>
	useFilterMenu({
		id: `organization:${organization}:spend-details:user`,
		value,
		onChange,
		enabled,
		getSelectedOption: async () => {
			if (!value) return null;
			const members = await API.getOrganizationPaginatedMembers(organization, {
				q: value,
				limit: 1,
			});
			const member = members.members.find(
				(candidate) => candidate.user_id === value,
			);
			return member
				? { label: member.name || member.username, value: member.user_id }
				: fallbackOption(value);
		},
		getOptions: async (query) => {
			const members = await API.getOrganizationPaginatedMembers(organization, {
				q: query,
				limit: 25,
			});
			return members.members.map((member) => ({
				label: member.name || member.username,
				value: member.user_id,
			}));
		},
	});

const useOrganizationGroupFilterMenu = ({
	organization,
	value,
	onChange,
	enabled,
}: Pick<UseFilterMenuOptions, "value" | "onChange" | "enabled"> & {
	organization: string;
}) =>
	useFilterMenu({
		id: `organization:${organization}:spend-details:group`,
		value,
		onChange,
		enabled,
		getSelectedOption: async () => {
			if (!value) return null;
			const groups = await API.getOrganizationPaginatedGroups(organization, {
				q: value,
				limit: 1,
			});
			const group = groups.groups.find((candidate) => candidate.id === value);
			return group
				? { label: group.display_name || group.name, value: group.id }
				: fallbackOption(value);
		},
		getOptions: async (query) => {
			const groups = await API.getOrganizationPaginatedGroups(organization, {
				q: query,
				limit: 25,
			});
			return groups.groups.map((group) => ({
				label: group.display_name || group.name,
				value: group.id,
			}));
		},
	});

export interface SpendDetailsFilterMenus {
	user: OrganizationFilterMenu;
	group: OrganizationFilterMenu;
	provider: ProviderFilterMenu;
	model: ModelFilterMenu;
}

interface SpendDetailsFiltersProps {
	organizations: readonly Organization[];
	organization: Organization;
	onOrganizationChange: (organization: Organization) => void;
	dateRange: DateRangeValue | undefined;
	now: Date;
	minDate: Date | undefined;
	onDateRangeChange: (value: DateRangeValue) => void;
	menus: SpendDetailsFilterMenus;
}

export const SpendDetailsFilters: FC<SpendDetailsFiltersProps> = ({
	organizations,
	organization,
	onOrganizationChange,
	dateRange,
	now,
	minDate,
	onDateRangeChange,
	menus,
}) => (
	<div className="flex flex-wrap items-center gap-2">
		<OrganizationSelectFilter
			label="Select user"
			placeholder="All users"
			menu={menus.user}
		/>
		<OrganizationSelectFilter
			label="Select group"
			placeholder="All groups"
			menu={menus.group}
		/>
		<ProviderFilter menu={menus.provider} width={FILTER_WIDTH} />
		<ModelFilter menu={menus.model} width={FILTER_WIDTH} />
		{dateRange && (
			<DateRangePicker
				value={dateRange}
				now={now}
				onChange={onDateRangeChange}
				maxDays={MAX_SPEND_PERIOD_DAYS}
				minDate={minDate}
				size="lg"
			/>
		)}
		{organizations.length > 1 && (
			<OrganizationAutocomplete
				value={organization}
				ariaLabel={`Organization ${getOrganizationLabel(organization, organizations)}`}
				options={organizations}
				triggerClassName="w-60 sm:ml-auto"
				optionsTabbable
				onChange={(next) => next && onOrganizationChange(next)}
			/>
		)}
	</div>
);

const OrganizationSelectFilter: FC<{
	label: string;
	placeholder: string;
	menu: OrganizationFilterMenu;
}> = ({ label, placeholder, menu }) => (
	<SelectFilter
		label={label}
		placeholder={placeholder}
		emptyText={`No ${placeholder.slice(4).toLowerCase()} found`}
		options={menu.searchOptions}
		onSelect={menu.selectOption}
		selectedOption={menu.selectedOption ?? undefined}
		width={FILTER_WIDTH}
		selectFilterSearch={
			<ComboboxInput
				placeholder={`Search ${placeholder.slice(4, -1).toLowerCase()}...`}
				value={menu.query}
				onValueChange={menu.setQuery}
				aria-label={`Search ${placeholder.slice(4, -1).toLowerCase()}`}
			/>
		}
	/>
);

export const useSpendDetailsFilterMenus = ({
	organization,
	values,
	onChange,
	enabled,
}: {
	organization: string;
	values: {
		user_id?: string;
		group_id?: string;
		provider_name?: string;
		model?: string;
	};
	onChange: (
		key: "user_id" | "group_id" | "provider_name" | "model",
		value: string | undefined,
	) => void;
	enabled: boolean;
}): SpendDetailsFilterMenus => ({
	user: useOrganizationMemberFilterMenu({
		organization,
		value: values.user_id,
		onChange: (option) => onChange("user_id", option?.value),
		enabled,
	}),
	group: useOrganizationGroupFilterMenu({
		organization,
		value: values.group_id,
		onChange: (option) => onChange("group_id", option?.value),
		enabled,
	}),
	provider: useProviderFilterMenu({
		value: values.provider_name,
		onChange: (option) => onChange("provider_name", option?.value),
		enabled,
	}),
	model: useModelFilterMenu({
		value: values.model,
		onChange: (option) => onChange("model", option?.value),
		enabled,
	}),
});
