import {
	type ConnectionLogStatus,
	ConnectionLogStatuses,
	type ConnectionType,
	ConnectionTypes,
} from "api/typesGenerated";
import { Filter, MenuSkeleton, type useFilter } from "components/Filter/Filter";
import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "components/Filter/menu";
import {
	SelectFilter,
	type SelectFilterOption,
} from "components/Filter/SelectFilter";
import {
	DEFAULT_USER_FILTER_WIDTH,
	type UserFilterMenu,
	UserMenu,
} from "components/Filter/UserFilter";
import capitalize from "lodash/capitalize";
import {
	type OrganizationsFilterMenu,
	OrganizationsMenu,
} from "modules/tableFiltering/options";
import type { FC } from "react";
import { connectionTypeToFriendlyName } from "utils/connection";
import { docs } from "utils/docs";

type ConnectionLogFilterValues = {
	status?: ConnectionLogStatus;
	type?: ConnectionType;
	workspace_owner?: string;
	organization?: string;
};

const buildConnectionLogFilterQuery = (
	v: ConnectionLogFilterValues,
): string => {
	const parts: string[] = [];
	if (v.status) parts.push(`status:${v.status}`);
	if (v.type) parts.push(`type:${v.type}`);
	if (v.workspace_owner) parts.push(`workspace_owner:${v.workspace_owner}`);
	if (v.organization) parts.push(`organization:${v.organization}`);
	return parts.join(" ");
};

const CONNECTION_LOG_PRESET_FILTERS = [
	{
		query: buildConnectionLogFilterQuery({ status: "ongoing", type: "ssh" }),
		name: "Active SSH connections",
	},
] satisfies { name: string; query: string }[];

interface ConnectionLogFilterProps {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
	menus: {
		user: UserFilterMenu;
		status: StatusFilterMenu;
		type: TypeFilterMenu;
		// The organization menu is only provided in a multi-org setup.
		organization?: OrganizationsFilterMenu;
	};
}

export const ConnectionLogFilter: FC<ConnectionLogFilterProps> = ({
	filter,
	error,
	menus,
}) => {
	const width = menus.organization ? DEFAULT_USER_FILTER_WIDTH : undefined;
	return (
		<Filter
			learnMoreLink={docs(
				"/admin/monitoring/connection-logs#how-to-filter-connection-logs",
			)}
			presets={CONNECTION_LOG_PRESET_FILTERS}
			isLoading={menus.user.isInitializing}
			filter={filter}
			error={error}
			options={
				<>
					<UserMenu placeholder="All owners" menu={menus.user} width={width} />
					<StatusMenu menu={menus.status} width={width} />
					<TypeMenu menu={menus.type} width={width} />
					{menus.organization && (
						<OrganizationsMenu menu={menus.organization} width={width} />
					)}
				</>
			}
			optionsSkeleton={
				<>
					<MenuSkeleton />
					<MenuSkeleton />
					<MenuSkeleton />
					{menus.organization && <MenuSkeleton />}
				</>
			}
		/>
	);
};

export const useStatusFilterMenu = ({
	value,
	onChange,
}: Pick<UseFilterMenuOptions, "value" | "onChange">) => {
	const statusOptions: SelectFilterOption[] = ConnectionLogStatuses.map(
		(status) => ({
			value: status,
			label: capitalize(status),
		}),
	);
	return useFilterMenu({
		onChange,
		value,
		id: "status",
		getSelectedOption: async () =>
			statusOptions.find((option) => option.value === value) ?? null,
		getOptions: async () => statusOptions,
	});
};

type StatusFilterMenu = ReturnType<typeof useStatusFilterMenu>;

interface StatusMenuProps {
	menu: StatusFilterMenu;
	width?: number;
}

const StatusMenu: FC<StatusMenuProps> = ({ menu, width }) => {
	return (
		<SelectFilter
			label="Filter by session status"
			placeholder="All sessions"
			options={menu.searchOptions}
			onSelect={menu.selectOption}
			selectedOption={menu.selectedOption ?? undefined}
			width={width}
		/>
	);
};

export const useTypeFilterMenu = ({
	value,
	onChange,
}: Pick<UseFilterMenuOptions, "value" | "onChange">) => {
	const typeOptions: SelectFilterOption[] = ConnectionTypes.map((type) => {
		const label: string = connectionTypeToFriendlyName(type);
		return {
			value: type,
			label,
		};
	});
	return useFilterMenu({
		onChange,
		value,
		id: "connection_type",
		getSelectedOption: async () =>
			typeOptions.find((option) => option.value === value) ?? null,
		getOptions: async () => typeOptions,
	});
};

type TypeFilterMenu = ReturnType<typeof useTypeFilterMenu>;

interface TypeMenuProps {
	menu: TypeFilterMenu;
	width?: number;
}

const TypeMenu: FC<TypeMenuProps> = ({ menu, width }) => {
	return (
		<SelectFilter
			label="Filter by connection type"
			placeholder="All types"
			options={menu.searchOptions}
			onSelect={menu.selectOption}
			selectedOption={menu.selectedOption ?? undefined}
			width={width}
		/>
	);
};
