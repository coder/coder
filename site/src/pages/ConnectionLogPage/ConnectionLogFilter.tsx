import capitalize from "lodash/capitalize";
import {
	type ConnectionLogMethod,
	ConnectionLogMethods,
	type ConnectionLogStatus,
	ConnectionLogStatuses,
} from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import {
	Filter,
	MenuSkeleton,
	type useFilter,
} from "#/components/Filter/Filter";
import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "#/components/Filter/menu";
import {
	SelectFilter,
	type SelectFilterOption,
} from "#/components/Filter/SelectFilter";
import {
	DEFAULT_USER_FILTER_WIDTH,
	type UserFilterMenu,
	UserMenu,
} from "#/components/Filter/UserFilter";
import {
	type OrganizationsFilterMenu,
	OrganizationsMenu,
} from "#/modules/tableFiltering/options";
import { docs } from "#/utils/docs";
import { connectionLogMethodLabels } from "./connectionLogMethodLabels";

type ConnectionLogFilterValues = {
	status?: ConnectionLogStatus;
	method?: ConnectionLogMethod;
	workspace_owner?: string;
	organization?: string;
};

const buildConnectionLogFilterQuery = (
	v: ConnectionLogFilterValues,
): string => {
	const parts: string[] = [];
	if (v.status) parts.push(`status:${v.status}`);
	if (v.method) parts.push(`method:${v.method}`);
	if (v.workspace_owner) parts.push(`workspace_owner:${v.workspace_owner}`);
	if (v.organization) parts.push(`organization:${v.organization}`);
	return parts.join(" ");
};

const CONNECTION_LOG_PRESET_FILTERS = [
	{
		query: buildConnectionLogFilterQuery({ status: "ongoing", method: "ssh" }),
		name: "Active SSH connections",
	},
] satisfies { name: string; query: string }[];

type ConnectionLogFilterProps = {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
	menus: {
		user: UserFilterMenu;
		status: StatusFilterMenu;
		method: MethodFilterMenu;
		// The organization menu is only provided in a multi-org setup.
		organization?: OrganizationsFilterMenu;
	};
};

export const ConnectionLogFilter: React.FC<ConnectionLogFilterProps> = ({
	filter,
	error,
	menus,
}) => {
	const width = menus.organization ? DEFAULT_USER_FILTER_WIDTH : undefined;
	return (
		<>
			{/* Saved links and bookmarks can still carry the deprecated filter. */}
			{filter.values.type && (
				<Alert severity="warning" className="mb-4">
					The <code>type</code> filter is deprecated and will be removed in a
					future release. Use <code>method</code> and <code>app</code> instead.
				</Alert>
			)}
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
						<UserMenu
							placeholder="All owners"
							menu={menus.user}
							width={width}
						/>
						<StatusMenu menu={menus.status} width={width} />
						<MethodMenu menu={menus.method} width={width} />
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
		</>
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

type StatusMenuProps = {
	menu: StatusFilterMenu;
	width?: number;
};

const StatusMenu: React.FC<StatusMenuProps> = ({ menu, width }) => {
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

export const useMethodFilterMenu = ({
	value,
	onChange,
}: Pick<UseFilterMenuOptions, "value" | "onChange">) => {
	const methodOptions: SelectFilterOption[] = ConnectionLogMethods.map(
		(method) => ({
			value: method,
			label: connectionLogMethodLabels[method],
		}),
	);
	return useFilterMenu({
		onChange,
		value,
		id: "connection_method",
		getSelectedOption: async () =>
			methodOptions.find((option) => option.value === value) ?? null,
		getOptions: async () => methodOptions,
	});
};

type MethodFilterMenu = ReturnType<typeof useMethodFilterMenu>;

type MethodMenuProps = {
	menu: MethodFilterMenu;
	width?: number;
};

const MethodMenu: React.FC<MethodMenuProps> = ({ menu, width }) => {
	return (
		<SelectFilter
			label="Filter by connection method"
			placeholder="All methods"
			options={menu.searchOptions}
			onSelect={menu.selectOption}
			selectedOption={menu.selectedOption ?? undefined}
			width={width}
		/>
	);
};
