import { action } from "@storybook/addon-actions";
import type { UseFilterResult } from "./Filter";
import type { UseFilterMenuResult } from "./menu";

export const MockMenu: UseFilterMenuResult = {
	initialOption: undefined,
	isInitializing: false,
	isSearching: false,
	query: "",
	searchOptions: [],
	selectedOption: undefined,
	selectOption: action("selectOption"),
	setQuery: action("updateQuery"),
};

export const getDefaultFilterProps = <TFilterProps>({
	query = "",
	values,
	menus,
	used = false,
}: {
	query?: string;
	values: Record<string, string | undefined>;
	menus: Record<string, UseFilterMenuResult>;
	used?: boolean;
}) =>
	({
		filter: {
			query,
			update: () => action("update"),
			debounceUpdate: action("debounce") as UseFilterResult["debounceUpdate"],
			used: used,
			values,
		},
		menus,
	}) as TFilterProps;
