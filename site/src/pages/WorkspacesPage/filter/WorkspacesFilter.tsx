import {
	Filter,
	MenuSkeleton,
	type UseFilterResult,
} from "components/Filter/Filter";
import {
	DEFAULT_USER_FILTER_WIDTH,
	type UserFilterMenu,
	UserMenu,
} from "components/Filter/UserFilter";
import { useDashboard } from "modules/dashboard/useDashboard";
import {
	type OrganizationsFilterMenu,
	OrganizationsMenu,
} from "modules/tableFiltering/options";
import type { FC } from "react";
import { docs } from "utils/docs";
import {
	type StatusFilterMenu,
	StatusMenu,
	type TemplateFilterMenu,
	TemplateMenu,
} from "./menus";

const workspaceFilterQuery = {
	me: "owner:me",
	all: "",
	running: "status:running",
	failed: "status:failed",
	dormant: "dormant:true",
	outdated: "outdated:true",
	shared: "shared:true",
};

type FilterPreset = {
	query: string;
	name: string;
};

// Can't use as const declarations to make arrays deep readonly because that
// interferes with the type contracts for Filter
const PRESET_FILTERS: FilterPreset[] = [
	{
		query: workspaceFilterQuery.me,
		name: "My workspaces",
	},
	{
		query: workspaceFilterQuery.all,
		name: "All workspaces",
	},
	{
		query: workspaceFilterQuery.running,
		name: "Running workspaces",
	},
	{
		query: workspaceFilterQuery.failed,
		name: "Failed workspaces",
	},
	{
		query: workspaceFilterQuery.outdated,
		name: "Outdated workspaces",
	},
	{
		query: workspaceFilterQuery.shared,
		name: "Shared workspaces",
	},
];

// Defined outside component so that the array doesn't get reconstructed each render
const PRESETS_WITH_DORMANT: FilterPreset[] = [
	...PRESET_FILTERS,
	{
		query: workspaceFilterQuery.dormant,
		name: "Dormant workspaces",
	},
];

export type WorkspaceFilterState = {
	filter: UseFilterResult;
	error?: unknown;
	menus: {
		user?: UserFilterMenu;
		template: TemplateFilterMenu;
		status: StatusFilterMenu;
		organizations?: OrganizationsFilterMenu;
	};
};

type WorkspaceFilterProps = Readonly<{
	filter: UseFilterResult;
	error: unknown;
	templateMenu: TemplateFilterMenu;
	statusMenu: StatusFilterMenu;

	userMenu?: UserFilterMenu;
	organizationsMenu?: OrganizationsFilterMenu;
}>;

export const WorkspacesFilter: FC<WorkspaceFilterProps> = ({
	filter,
	error,
	templateMenu,
	statusMenu,
	userMenu,
	organizationsMenu,
}) => {
	const { entitlements, showOrganizations } = useDashboard();
	const width = showOrganizations ? DEFAULT_USER_FILTER_WIDTH : undefined;
	const presets = entitlements.features.advanced_template_scheduling.enabled
		? PRESETS_WITH_DORMANT
		: PRESET_FILTERS;
	const organizationsActive =
		showOrganizations && organizationsMenu !== undefined;

	return (
		<Filter
			presets={presets}
			isLoading={statusMenu.isInitializing}
			filter={filter}
			error={error}
			learnMoreLink={docs(
				"/user-guides/workspace-management#workspace-filtering",
			)}
			options={
				<>
					{userMenu && <UserMenu width={width} menu={userMenu} />}
					<TemplateMenu width={width} menu={templateMenu} />
					<StatusMenu width={width} menu={statusMenu} />
					{organizationsActive && (
						<OrganizationsMenu width={width} menu={organizationsMenu} />
					)}
				</>
			}
			optionsSkeleton={
				<>
					{userMenu && <MenuSkeleton />}
					<MenuSkeleton />
					<MenuSkeleton />
					{organizationsActive && <MenuSkeleton />}
				</>
			}
		/>
	);
};
