import { Filter, MenuSkeleton, type useFilter } from "components/Filter/Filter";
import { type UserFilterMenu, UserMenu } from "components/Filter/UserFilter";
import { OrganizationsFilter, type OrganizationsFilterMenu } from "modules/tableFiltering/options";
import type { FC } from "react";
import { ActionFilter, type ActionFilterMenu } from "./filter";

interface BoundaryLogsFilterProps {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
	menus: {
		user: UserFilterMenu;
		action: ActionFilterMenu;
		organization?: OrganizationsFilterMenu;
	};
}

export const BoundaryLogsFilter: FC<BoundaryLogsFilterProps> = ({
	filter,
	error,
	menus,
}) => {
	return (
		<Filter
			filter={filter}
			optionsSkeleton={<MenuSkeleton />}
			isLoading={menus.user.isInitializing}
			presets={[
				{
					name: "All logs",
					query: "",
				},
				{
					name: "My workspaces",
					query: "workspace_owner:me",
				},
				{
					name: "Denied requests",
					query: "action:deny",
				},
			]}
			error={error}
			options={
				<>
					<UserMenu menu={menus.user} />
					<ActionFilter menu={menus.action} />
					{menus.organization && (
						<OrganizationsFilter menu={menus.organization} />
					)}
				</>
			}
		/>
	);
};
