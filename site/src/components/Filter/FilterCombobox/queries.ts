import type { UseQueryOptions } from "react-query";
import type { FilterOption } from "./types";

/**
 * Delay after the last keystroke before typed text is sent to `onChange` and
 * to a category's `getOptions`.
 */
export const SEARCH_DEBOUNCE_MS = 300;

const filterComboboxOptionsKey = (categoryKey: string, query: string) =>
	["filterCombobox", "options", categoryKey, query] as const;

/**
 * react-query options for one category's options at `query`. `getOptions` is
 * optional so an active-category key that no longer resolves to a category
 * degrades to an empty result instead of throwing.
 */
export const filterComboboxOptions = (
	categoryKey: string,
	getOptions: ((query: string) => Promise<FilterOption[]>) | undefined,
	query: string,
	enabled: boolean,
) => {
	return {
		queryKey: filterComboboxOptionsKey(categoryKey, query),
		queryFn: async (): Promise<FilterOption[]> =>
			getOptions ? getOptions(query) : [],
		enabled,
	} satisfies UseQueryOptions<FilterOption[]>;
};
