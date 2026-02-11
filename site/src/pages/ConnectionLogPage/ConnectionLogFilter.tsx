import { Filter, MenuSkeleton, type useFilter } from "components/Filter/Filter";
import {
	DEFAULT_USER_FILTER_WIDTH,
	type UserFilterMenu,
	UserMenu,
} from "components/Filter/UserFilter";
import {
	type OrganizationsFilterMenu,
	OrganizationsMenu,
} from "modules/tableFiltering/options";
import type { FC } from "react";
import { docs } from "utils/docs";

type ConnectionLogFilterValues = {
	workspace_owner?: string;
	organization?: string;
};

const buildConnectionLogFilterQuery = (
	v: ConnectionLogFilterValues,
): string => {
	const parts: string[] = [];
	if (v.workspace_owner) parts.push(`workspace_owner:${v.workspace_owner}`);
	if (v.organization) parts.push(`organization:${v.organization}`);
	return parts.join(" ");
};

const CONNECTION_LOG_PRESET_FILTERS = [
	{
		query: buildConnectionLogFilterQuery({ workspace_owner: "me" }),
		name: "My sessions",
	},
] satisfies { name: string; query: string }[];

interface ConnectionLogFilterProps {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
	menus: {
		user: UserFilterMenu;
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
					{menus.organization && (
						<OrganizationsMenu menu={menus.organization} width={width} />
					)}
				</>
			}
			optionsSkeleton={
				<>
					<MenuSkeleton />
					{menus.organization && <MenuSkeleton />}
				</>
			}
		/>
	);
};
