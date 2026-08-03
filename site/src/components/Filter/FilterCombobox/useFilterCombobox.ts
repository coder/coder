import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "react-query";
import type { UseFilterResult } from "#/components/Filter/Filter";
import type { SelectFilterOption } from "#/components/Filter/SelectFilter";
import { useDebouncedValue } from "#/hooks/debounce";
import {
	chipsToValues,
	chipToken,
	collectValueSuggestions,
	composeFilterQuery,
	extractFreeText,
	filterValuesToChips,
	matchFacets,
	parseChipToken,
	parseTypedFacetPrefix,
} from "./filterQuery";
import {
	type FilterFacet,
	type FilterSearchResult,
	parseSearchResultToken,
	searchResultToken,
} from "./types";

const SEARCH_DEBOUNCE_MS = 300;

export const filterComboboxSearchResultsKey = (query: string) =>
	["filterCombobox", "searchResults", query] as const;

type UseFilterComboboxOptions<Id extends string> = {
	filter: UseFilterResult;
	facets: readonly FilterFacet<Id>[];
	/** Stable chip key order for serialization. Defaults to facet ids. */
	chipKeys?: readonly Id[];
	getSearchResults?: (query: string) => Promise<FilterSearchResult[]>;
	onSearchResultSelect?: (result: FilterSearchResult) => void;
};

/** Why the popup is open when no facet is active. */
export type FilterComboboxBrowseMode = "typeahead";

export const useFilterCombobox = <Id extends string>({
	filter,
	facets,
	chipKeys: chipKeysProp,
	getSearchResults,
	onSearchResultSelect,
}: UseFilterComboboxOptions<Id>) => {
	const chipKeys = chipKeysProp ?? facets.map((facet) => facet.id);
	const [open, setOpen] = useState(false);
	const [browseMode, setBrowseMode] = useState<FilterComboboxBrowseMode | null>(
		null,
	);
	const [activeFacet, setActiveFacet] = useState<Id | null>(null);
	const [inputValue, setInputValue] = useState(() =>
		extractFreeText(filter.query),
	);
	const [committedFreeText, setCommittedFreeText] = useState(() =>
		extractFreeText(filter.query),
	);
	const committedFreeTextRef = useRef(committedFreeText);
	const facetModeRef = useRef(false);
	const browseModeRef = useRef<FilterComboboxBrowseMode | null>(null);
	const getSearchResultsRef = useRef(getSearchResults);
	getSearchResultsRef.current = getSearchResults;
	const onSearchResultSelectRef = useRef(onSearchResultSelect);
	onSearchResultSelectRef.current = onSearchResultSelect;
	const hasSearchResults = Boolean(getSearchResults);

	const setBrowseModeSafe = (next: FilterComboboxBrowseMode | null) => {
		browseModeRef.current = next;
		setBrowseMode(next);
	};

	const setCommittedNameSearch = (value: string) => {
		const next = value.trim();
		committedFreeTextRef.current = next;
		setCommittedFreeText(next);
	};

	const openCategorySuggestions = () => {
		facetModeRef.current = false;
		setBrowseModeSafe("typeahead");
		setOpen(true);
	};

	/** Opens or closes the category list without focusing the search input. */
	const toggleFilterMenu = () => {
		if (open) {
			setOpen(false);
			setBrowseModeSafe(null);
			exitFacetMode();
			return;
		}
		openCategorySuggestions();
		setActiveFacet(null);
	};

	const activeFacetMeta = facets.find((facet) => facet.id === activeFacet);
	const activeOptions = activeFacetMeta?.menu.searchOptions;
	const chipValues = filterValuesToChips(filter.values, chipKeys);

	/** Categories shown in typeahead: all when empty, prefix matches while typing. */
	const listedFacets = useMemo(() => {
		if (activeFacet !== null || browseMode !== "typeahead") {
			return [] as FilterFacet<Id>[];
		}
		if (inputValue.trim().length === 0) {
			return [...facets];
		}
		return matchFacets(inputValue, facets);
	}, [activeFacet, browseMode, facets, inputValue]);
	const listedFacetsRef = useRef(listedFacets);
	listedFacetsRef.current = listedFacets;

	// Drive every facet menu from free-text typeahead so options stay in sync.
	useEffect(() => {
		if (activeFacet !== null || browseMode !== "typeahead") {
			return;
		}
		const query = inputValue.trim();
		for (const facet of facets) {
			facet.menu.setQuery(query);
		}
	}, [activeFacet, browseMode, facets, inputValue]);

	const valueSuggestions = useMemo(() => {
		if (activeFacet !== null || browseMode !== "typeahead") {
			return [];
		}
		return collectValueSuggestions(inputValue, facets, chipValues);
	}, [activeFacet, browseMode, chipValues, facets, inputValue]);
	const valueSuggestionsRef = useRef(valueSuggestions);
	valueSuggestionsRef.current = valueSuggestions;

	const valueSuggestionsLoading =
		activeFacet === null &&
		browseMode === "typeahead" &&
		inputValue.trim().length > 0 &&
		facets.some(
			(facet) =>
				facet.menu.isSearching === true ||
				facet.menu.searchOptions === undefined,
		);

	const previewQuerySource =
		activeFacet === null && browseMode === "typeahead" ? inputValue.trim() : "";
	const debouncedPreviewQuery = useDebouncedValue(
		previewQuerySource,
		SEARCH_DEBOUNCE_MS,
	);
	const previewQueryPending =
		hasSearchResults &&
		previewQuerySource.length > 0 &&
		previewQuerySource !== debouncedPreviewQuery;

	const searchResultsQuery = useQuery({
		queryKey: filterComboboxSearchResultsKey(debouncedPreviewQuery),
		queryFn: () => {
			const loader = getSearchResultsRef.current;
			if (!loader) {
				return Promise.resolve([] as FilterSearchResult[]);
			}
			return loader(debouncedPreviewQuery);
		},
		enabled:
			hasSearchResults &&
			debouncedPreviewQuery.length > 0 &&
			activeFacet === null &&
			browseMode === "typeahead",
	});

	const searchResults = searchResultsQuery.data ?? [];
	const searchResultsLoading =
		previewQueryPending || searchResultsQuery.isFetching;
	const searchResultsRef = useRef(searchResults);
	searchResultsRef.current = searchResults;

	const optionItems = useMemo(() => {
		if (activeFacet && activeOptions) {
			return activeOptions.map((option) =>
				chipToken(activeFacet, option.value),
			);
		}
		if (activeFacet === null && browseMode === "typeahead") {
			return [
				...listedFacets.map((facet) => facet.id),
				...valueSuggestions.map((suggestion) => suggestion.token),
				...searchResults.map((result) => searchResultToken(result.id)),
			];
		}
		return [] as string[];
	}, [
		activeFacet,
		activeOptions,
		browseMode,
		listedFacets,
		searchResults,
		valueSuggestions,
	]);
	const optionItemsRef = useRef(optionItems);
	optionItemsRef.current = optionItems;

	const optionByToken = useMemo(() => {
		const map = new Map<string, SelectFilterOption>();
		if (!activeFacet || !activeOptions) {
			return map;
		}
		for (const option of activeOptions) {
			map.set(chipToken(activeFacet, option.value), option);
		}
		return map;
	}, [activeFacet, activeOptions]);

	const restoreFreeTextInput = () => {
		setInputValue(committedFreeTextRef.current);
	};

	const exitFacetMode = () => {
		facetModeRef.current = false;
		setActiveFacet(null);
		restoreFreeTextInput();
	};

	const enterFacetMode = (facetId: Id, query = "") => {
		facetModeRef.current = true;
		setBrowseModeSafe(null);
		setActiveFacet(facetId);
		setInputValue(query);
		setOpen(true);
		const facet = facets.find((entry) => entry.id === facetId);
		facet?.menu.setQuery(query);
	};

	const updateFromChips = (tokens: string[], freeText?: string) => {
		const nextFreeText =
			freeText === undefined ? committedFreeTextRef.current : freeText;
		if (freeText !== undefined) {
			setCommittedNameSearch(freeText);
		}
		filter.cancelDebounce();
		filter.update(
			composeFilterQuery(
				chipsToValues(tokens, chipKeys),
				chipKeys,
				nextFreeText,
			),
		);
	};

	const selectFacet = (
		facetId: Id,
		options?: Readonly<{ clearMatchedQuery?: boolean }>,
	) => {
		filter.cancelDebounce();
		if (options?.clearMatchedQuery) {
			// Typed text matched a category name; do not keep it as free-text search.
			setCommittedNameSearch("");
			filter.update(composeFilterQuery(filter.values, chipKeys, ""));
		} else if (activeFacet === null) {
			setCommittedNameSearch(inputValue);
			filter.update(composeFilterQuery(filter.values, chipKeys, inputValue));
		}
		enterFacetMode(facetId);
	};

	const selectValueSuggestion = (token: string) => {
		updateFromChips([...chipValues, token], "");
		facetModeRef.current = false;
		setBrowseModeSafe(null);
		setActiveFacet(null);
		setInputValue("");
		setOpen(false);
	};

	const selectSearchResult = (result: FilterSearchResult) => {
		onSearchResultSelectRef.current?.(result);
		facetModeRef.current = false;
		setBrowseModeSafe(null);
		setActiveFacet(null);
		setOpen(false);
	};

	const handleInputFocus = () => {
		if (activeFacet !== null || facetModeRef.current) {
			return;
		}
		openCategorySuggestions();
	};

	const handleInputValueChange = (
		nextValue: string,
		eventDetails: { reason: string },
	) => {
		if (eventDetails.reason === "input-clear") {
			if (activeFacet === null) {
				restoreFreeTextInput();
				return;
			}
			setInputValue("");
			return;
		}

		const typedFacet = parseTypedFacetPrefix(nextValue, facets);
		if (typedFacet) {
			filter.cancelDebounce();
			setCommittedNameSearch(typedFacet.freeText);
			filter.update(
				composeFilterQuery(filter.values, chipKeys, typedFacet.freeText),
			);
			enterFacetMode(typedFacet.facetId, typedFacet.query);
			return;
		}

		if (activeFacetMeta) {
			setInputValue(nextValue);
			activeFacetMeta.menu.setQuery(nextValue);
			facetModeRef.current = true;
			setBrowseModeSafe(null);
			setOpen(true);
			return;
		}

		setCommittedNameSearch(nextValue);
		setInputValue(nextValue);
		filter.debounceUpdate(
			composeFilterQuery(filter.values, chipKeys, nextValue),
		);
		openCategorySuggestions();
	};

	const handleOpenChange = (
		nextOpen: boolean,
		eventDetails: { reason: string },
	) => {
		if (
			nextOpen &&
			!facetModeRef.current &&
			browseModeRef.current !== "typeahead" &&
			eventDetails.reason !== "trigger-press"
		) {
			return;
		}

		setOpen(nextOpen);
		if (!nextOpen) {
			setBrowseModeSafe(null);
			exitFacetMode();
		} else if (eventDetails.reason === "trigger-press") {
			facetModeRef.current = false;
			setBrowseModeSafe("typeahead");
			setActiveFacet(null);
		}
	};

	const handleValueChange = (nextTokens: string[]) => {
		const added = nextTokens.find((token) => !chipValues.includes(token));
		if (added) {
			const searchId = parseSearchResultToken(added);
			if (searchId) {
				const result = searchResultsRef.current.find(
					(entry) => entry.id === searchId,
				);
				if (result) {
					selectSearchResult(result);
				}
				return;
			}

			const matchedFacet = facets.find((facet) => facet.id === added);
			if (activeFacet === null && matchedFacet) {
				selectFacet(matchedFacet.id, { clearMatchedQuery: true });
				return;
			}

			if (
				activeFacet === null &&
				browseModeRef.current === "typeahead" &&
				parseChipToken(added, chipKeys)
			) {
				selectValueSuggestion(added);
				return;
			}
		}

		updateFromChips(nextTokens);
		facetModeRef.current = false;
		setBrowseModeSafe(null);
		setActiveFacet(null);
		setInputValue(committedFreeTextRef.current);
		setOpen(false);
	};

	const exitActiveFacet = () => {
		activeFacetMeta?.menu.setQuery("");
		facetModeRef.current = false;
		setBrowseModeSafe(null);
		setActiveFacet(null);
		restoreFreeTextInput();
		setOpen(false);
	};

	const handleInputKeyDown = (event: {
		key: string;
		preventDefault: () => void;
		preventBaseUIHandler?: () => void;
	}) => {
		if (event.key === "Backspace" && inputValue === "" && activeFacetMeta) {
			event.preventDefault();
			event.preventBaseUIHandler?.();
			exitActiveFacet();
			return;
		}

		if (event.key !== "Enter" || browseModeRef.current !== "typeahead") {
			return;
		}

		const topItem = optionItemsRef.current[0];
		if (!topItem) {
			return;
		}

		event.preventDefault();
		event.preventBaseUIHandler?.();

		const searchId = parseSearchResultToken(topItem);
		if (searchId) {
			const result = searchResultsRef.current.find(
				(entry) => entry.id === searchId,
			);
			if (result) {
				selectSearchResult(result);
			}
			return;
		}

		const topFacet = listedFacetsRef.current.find(
			(facet) => facet.id === topItem,
		);
		if (topFacet) {
			selectFacet(topFacet.id, { clearMatchedQuery: true });
			return;
		}

		if (parseChipToken(topItem, chipKeys)) {
			selectValueSuggestion(topItem);
		}
	};

	return {
		open,
		browseMode,
		inputValue,
		committedFreeText,
		activeFacet,
		activeFacetMeta,
		activeOptions,
		listedFacets,
		valueSuggestions,
		valueSuggestionsLoading,
		searchResults,
		searchResultsLoading,
		chipValues,
		optionItems,
		optionByToken,
		selectFacet,
		exitActiveFacet,
		toggleFilterMenu,
		handleInputFocus,
		handleInputKeyDown,
		handleInputValueChange,
		handleOpenChange,
		handleValueChange,
	};
};
