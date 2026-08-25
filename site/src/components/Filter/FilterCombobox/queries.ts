import type { FilterOption, SearchResult } from "./types";

export const filterComboboxOptionsKey = (categoryKey: string, query: string) =>
	["filterCombobox", "options", categoryKey, query] as const;

export const filterComboboxSearchResultsKey = (query: string) =>
	["filterCombobox", "searchResults", query] as const;

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
) => ({
	queryKey: filterComboboxOptionsKey(categoryKey, query),
	queryFn: () =>
		getOptions ? getOptions(query) : Promise.resolve<FilterOption[]>([]),
	enabled,
});

/** react-query options for the free-text resource preview at `query`. */
export const filterComboboxSearchResults = (
	getSearchResults: ((query: string) => Promise<SearchResult[]>) | undefined,
	query: string,
	enabled: boolean,
) => ({
	queryKey: filterComboboxSearchResultsKey(query),
	queryFn: () =>
		getSearchResults
			? getSearchResults(query)
			: Promise.resolve<SearchResult[]>([]),
	enabled,
});
