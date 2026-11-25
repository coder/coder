import { action } from "storybook/actions";
import type { UseFilterResult } from "./Filter";

export const getDefaultFilterProps = <TFilterProps>({
	query = "",
	values,
	used = false,
}: {
	query?: string;
	values: Record<string, string | undefined>;
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
	}) as TFilterProps;
