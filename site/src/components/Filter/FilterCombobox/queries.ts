import { queryOptions } from "react-query";
import type { FilterOption, SearchResult } from "./types";

const filterComboboxOptionsKey = (categoryKey: string, query: string) =>
	["filterCombobox", "options", categoryKey, query] as const;

const filterComboboxSearchResultsKey = (query: string) =>
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
) =>
	queryOptions({
		queryKey: filterComboboxOptionsKey(categoryKey, query),
		queryFn: async (): Promise<FilterOption[]> =>
			getOptions ? getOptions(query) : [],
		enabled,
	});

/** react-query options for the free-text resource preview at `query`. */
export const filterComboboxSearchResults = (
	getSearchResults: ((query: string) => Promise<SearchResult[]>) | undefined,
	query: string,
	enabled: boolean,
) =>
	queryOptions({
		queryKey: filterComboboxSearchResultsKey(query),
		queryFn: async (): Promise<SearchResult[]> =>
			getSearchResults ? getSearchResults(query) : [],
		enabled,
	});
