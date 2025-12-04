import { Filter, MenuSkeleton, type useFilter } from "components/Filter/Filter";
import { type UserFilterMenu, UserMenu } from "components/Filter/UserFilter";
import { OrganizationsMenu, type OrganizationsFilterMenu } from "modules/tableFiltering/options";
import type { FC } from "react";
import { DecisionFilter, type DecisionFilterMenu } from "./filter";

interface BoundaryLogsFilterProps {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
	menus: {
		user: UserFilterMenu;
		decision: DecisionFilterMenu;
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
					query: "decision:deny",
				},
			]}
			error={error}
			options={
				<>
					<UserMenu menu={menus.user} />
					<DecisionFilter menu={menus.decision} />
					{menus.organization && (
						<OrganizationsMenu menu={menus.organization} />
					)}
				</>
			}
		/>
	);
};
