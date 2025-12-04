import {
	type UseFilterMenuOptions,
	useFilterMenu,
} from "components/Filter/menu";
import {
	SelectFilter,
	type SelectFilterOption,
} from "components/Filter/SelectFilter";
import type { FC } from "react";

const BOUNDARY_ACTIONS: SelectFilterOption[] = [
	{
		label: "Allow",
		value: "allow",
	},
	{
		label: "Deny",
		value: "deny",
	},
];

export const useActionFilterMenu = ({
	value,
	onChange,
	enabled,
}: Pick<UseFilterMenuOptions, "value" | "onChange" | "enabled">) => {
	return useFilterMenu({
		id: "action",
		getSelectedOption: async () =>
			BOUNDARY_ACTIONS.find((option) => option.value === value) ?? null,
		getOptions: async () => {
			return BOUNDARY_ACTIONS;
		},
		value,
		onChange,
		enabled,
	});
};

export type ActionFilterMenu = ReturnType<typeof useActionFilterMenu>;

interface ActionFilterProps {
	menu: ActionFilterMenu;
}

export const ActionFilter: FC<ActionFilterProps> = ({ menu }) => {
	return (
		<SelectFilter
			label={"Select action"}
			placeholder={"All actions"}
			emptyText="No actions found"
			options={menu.searchOptions}
			onSelect={(option) => menu.selectOption(option)}
			selectedOption={menu.selectedOption ?? undefined}
		/>
	);
};
