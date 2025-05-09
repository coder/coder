import { Filter, MenuSkeleton, type useFilter } from "components/Filter/Filter";
import { type UserFilterMenu, UserMenu } from "components/Filter/UserFilter";
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
];

// Defined outside component so that the array doesn't get reconstructed each render
const PRESETS_WITH_DORMANT: FilterPreset[] = [
	...PRESET_FILTERS,
	{
		query: workspaceFilterQuery.dormant,
		name: "Dormant workspaces",
	},
];

export type WorkspaceFilterProps = {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
	menus: {
		user?: UserFilterMenu;
		template: TemplateFilterMenu;
		status: StatusFilterMenu;
		organizations?: OrganizationsFilterMenu;
	};
};

export const WorkspacesFilter: FC<WorkspaceFilterProps> = ({
	filter,
	error,
	menus,
}) => {
	const { entitlements, showOrganizations } = useDashboard();
	const width = showOrganizations ? 175 : undefined;
	const presets = entitlements.features.advanced_template_scheduling.enabled
		? PRESETS_WITH_DORMANT
		: PRESET_FILTERS;

	return (
		<Filter
			presets={presets}
			isLoading={menus.status.isInitializing}
			filter={filter}
			error={error}
			learnMoreLink={docs(
				"/user-guides/workspace-management#workspace-filtering",
			)}
			options={
				<>
					{menus.user && <UserMenu width={width} menu={menus.user} />}
					<TemplateMenu width={width} menu={menus.template} />
					<StatusMenu width={width} menu={menus.status} />
					{showOrganizations && menus.organizations !== undefined && (
						<OrganizationsMenu width={width} menu={menus.organizations} />
					)}
				</>
			}
			optionsSkeleton={
				<>
					{menus.user && <MenuSkeleton />}
					<MenuSkeleton />
					<MenuSkeleton />
					{showOrganizations && <MenuSkeleton />}
				</>
			}
		/>
	);
};
