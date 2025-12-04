import { Filter, MenuSkeleton, type useFilter } from "components/Filter/Filter";
import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "components/Filter/menu";
import {
	SelectFilter,
	type SelectFilterOption,
} from "components/Filter/SelectFilter";
import { type UserFilterMenu, UserMenu } from "components/Filter/UserFilter";
import {
	OrganizationsMenu,
	type OrganizationsFilterMenu,
} from "modules/tableFiltering/options";
import type { FC } from "react";

const BOUNDARY_DECISIONS: SelectFilterOption[] = [
	{
		label: "Allow",
		value: "allow",
	},
	{
		label: "Deny",
		value: "deny",
	},
];

export const useDecisionFilterMenu = ({
	value,
	onChange,
	enabled,
}: Pick<UseFilterMenuOptions, "value" | "onChange" | "enabled">) => {
	return useFilterMenu({
		id: "decision",
		getSelectedOption: async () =>
			BOUNDARY_DECISIONS.find((option) => option.value === value) ?? null,
		getOptions: async () => {
			return BOUNDARY_DECISIONS;
		},
		value,
		onChange,
		enabled,
	});
};

export type DecisionFilterMenu = ReturnType<typeof useDecisionFilterMenu>;

interface DecisionFilterProps {
	menu: DecisionFilterMenu;
}

export const DecisionFilter: FC<DecisionFilterProps> = ({ menu }) => {
	return (
		<SelectFilter
			label={"Select decision"}
			placeholder={"All decisions"}
			emptyText="No decisions found"
			options={menu.searchOptions}
			onSelect={(option) => menu.selectOption(option)}
			selectedOption={menu.selectedOption ?? undefined}
		/>
	);
};

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
