import { useEffect, useMemo, useRef, useState } from "react";
import { useQueries, useQuery } from "react-query";
import { useDebouncedFunction, useDebouncedValue } from "#/hooks/debounce";
import {
	chipsToValues,
	chipToken,
	collectValueSuggestions,
	composeFilterQuery,
	extractFreeText,
	matchCategories,
	parseChipToken,
	parseTypedCategoryPrefix,
	queryToChips,
} from "./filterQuery";
import type { FilterCategory, FilterOption, SearchResult } from "./types";
import { parseSearchResultToken, searchResultToken } from "./types";

const SEARCH_DEBOUNCE_MS = 300;

const filterComboboxSearchResultsKey = (query: string) =>
	["filterCombobox", "searchResults", query] as const;

const filterComboboxOptionsKey = (categoryKey: string, query: string) =>
	["filterCombobox", "options", categoryKey, query] as const;

type UseFilterComboboxOptions = {
	value: string;
	onChange: (query: string) => void;
	categories: readonly FilterCategory[];
	getSearchResults?: (query: string) => Promise<SearchResult[]>;
	onSearchResultSelect?: (result: SearchResult) => void;
};

/** Why the popup is open when no category is active. */
type FilterComboboxBrowseMode = "typeahead";

export const useFilterCombobox = ({
	value,
	onChange,
	categories,
	getSearchResults,
	onSearchResultSelect,
}: UseFilterComboboxOptions) => {
	const chipKeys = useMemo(
		() => categories.map((category) => category.key),
		[categories],
	);
	const [open, setOpen] = useState(false);
	const [browseMode, setBrowseMode] = useState<FilterComboboxBrowseMode | null>(
		null,
	);
	const [activeCategoryKey, setActiveCategoryKey] = useState<string | null>(
		null,
	);
	const [inputValue, setInputValue] = useState(() => extractFreeText(value));
	const [committedFreeText, setCommittedFreeText] = useState(() =>
		extractFreeText(value),
	);
	const committedFreeTextRef = useRef(committedFreeText);
	const facetModeRef = useRef(false);
	const browseModeRef = useRef<FilterComboboxBrowseMode | null>(null);
	const lastEmittedRef = useRef(value);
	const onChangeRef = useRef(onChange);
	onChangeRef.current = onChange;
	const getSearchResultsRef = useRef(getSearchResults);
	getSearchResultsRef.current = getSearchResults;
	const onSearchResultSelectRef = useRef(onSearchResultSelect);
	onSearchResultSelectRef.current = onSearchResultSelect;
	const storeInputRef = useRef<HTMLInputElement | null>(null);
	const hasSearchResults = Boolean(getSearchResults);
	const categoriesRef = useRef(categories);
	categoriesRef.current = categories;

	const { debounced: debouncedOnChange, cancelDebounce } = useDebouncedFunction(
		(query: string) => {
			lastEmittedRef.current = query;
			onChangeRef.current(query);
		},
		SEARCH_DEBOUNCE_MS,
	);

	const emitQuery = (query: string, immediate = false) => {
		if (immediate) {
			cancelDebounce();
			lastEmittedRef.current = query;
			onChangeRef.current(query);
			return;
		}
		debouncedOnChange(query);
	};

	const setBrowseModeSafe = (next: FilterComboboxBrowseMode | null) => {
		browseModeRef.current = next;
		setBrowseMode(next);
	};

	const setCommittedNameSearch = (nextValue: string) => {
		const next = nextValue.trim();
		committedFreeTextRef.current = next;
		setCommittedFreeText(next);
	};

	// Sync local free-text state when the controlled value changes externally.
	useEffect(() => {
		if (value === lastEmittedRef.current) {
			return;
		}
		lastEmittedRef.current = value;
		const freeText = extractFreeText(value);
		committedFreeTextRef.current = freeText;
		setCommittedFreeText(freeText);
		if (!facetModeRef.current) {
			setInputValue(freeText);
		}
	}, [value]);

	const openCategorySuggestions = () => {
		facetModeRef.current = false;
		setBrowseModeSafe("typeahead");
		setOpen(true);
	};

	/** Opens or closes the category list. Focuses the input when opening so
	 * keyboard list navigation via aria-activedescendant works. */
	const toggleFilterMenu = () => {
		if (open) {
			setOpen(false);
			setBrowseModeSafe(null);
			exitFacetMode();
			return;
		}
		openCategorySuggestions();
		setActiveCategoryKey(null);
		queueMicrotask(() => {
			storeInputRef.current?.focus();
		});
	};

	const activeCategory = categories.find(
		(category) => category.key === activeCategoryKey,
	);
	const chipValues = useMemo(
		() => queryToChips(value, chipKeys),
		[chipKeys, value],
	);

	/** Categories shown in typeahead: all when empty, prefix matches while typing. */
	const listedCategories = useMemo(() => {
		if (activeCategoryKey !== null || browseMode !== "typeahead") {
			return [] as FilterCategory[];
		}
		if (inputValue.trim().length === 0) {
			return [...categories];
		}
		return matchCategories(inputValue, categories);
	}, [activeCategoryKey, browseMode, categories, inputValue]);
	const listedCategoriesRef = useRef(listedCategories);
	listedCategoriesRef.current = listedCategories;

	const activeOptionsQuerySource = activeCategoryKey !== null ? inputValue : "";
	const debouncedActiveOptionsQuery = useDebouncedValue(
		activeOptionsQuerySource,
		SEARCH_DEBOUNCE_MS,
	);
	const activeOptionsPending =
		activeCategoryKey !== null &&
		activeOptionsQuerySource !== debouncedActiveOptionsQuery;

	const activeOptionsQuery = useQuery({
		queryKey: filterComboboxOptionsKey(
			activeCategoryKey ?? "",
			debouncedActiveOptionsQuery,
		),
		queryFn: () => {
			const category = categoriesRef.current.find(
				(entry) => entry.key === activeCategoryKey,
			);
			if (!category) {
				return Promise.resolve([] as FilterOption[]);
			}
			return category.getOptions(debouncedActiveOptionsQuery);
		},
		enabled: activeCategoryKey !== null,
	});

	const activeOptions = activeOptionsQuery.data;
	const activeOptionsLoading =
		activeOptionsPending ||
		(activeCategoryKey !== null &&
			(activeOptionsQuery.isFetching || activeOptions === undefined));

	const suggestionQuerySource =
		activeCategoryKey === null && browseMode === "typeahead"
			? inputValue.trim()
			: "";
	const debouncedSuggestionQuery = useDebouncedValue(
		suggestionQuerySource,
		SEARCH_DEBOUNCE_MS,
	);
	const suggestionQueryPending =
		suggestionQuerySource.length > 0 &&
		suggestionQuerySource !== debouncedSuggestionQuery;

	const suggestionQueries = useQueries({
		queries: categories.map((category) => ({
			queryKey: filterComboboxOptionsKey(
				category.key,
				debouncedSuggestionQuery,
			),
			queryFn: () => category.getOptions(debouncedSuggestionQuery),
			enabled:
				debouncedSuggestionQuery.length > 0 &&
				activeCategoryKey === null &&
				browseMode === "typeahead",
		})),
	});

	const optionsByKey = useMemo(() => {
		const map = new Map<string, readonly FilterOption[]>();
		categories.forEach((category, index) => {
			const options = suggestionQueries[index]?.data;
			if (options) {
				map.set(category.key, options);
			}
		});
		return map;
	}, [categories, suggestionQueries]);

	const valueSuggestions = useMemo(() => {
		if (activeCategoryKey !== null || browseMode !== "typeahead") {
			return [];
		}
		return collectValueSuggestions(
			inputValue,
			categories,
			optionsByKey,
			chipValues,
		);
	}, [
		activeCategoryKey,
		browseMode,
		categories,
		chipValues,
		inputValue,
		optionsByKey,
	]);
	const valueSuggestionsRef = useRef(valueSuggestions);
	valueSuggestionsRef.current = valueSuggestions;

	const valueSuggestionsLoading =
		activeCategoryKey === null &&
		browseMode === "typeahead" &&
		inputValue.trim().length > 0 &&
		(suggestionQueryPending ||
			suggestionQueries.some(
				(query) => query.isFetching || query.data === undefined,
			));

	const previewQuerySource =
		activeCategoryKey === null && browseMode === "typeahead"
			? inputValue.trim()
			: "";
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
				return Promise.resolve([] as SearchResult[]);
			}
			return loader(debouncedPreviewQuery);
		},
		enabled:
			hasSearchResults &&
			debouncedPreviewQuery.length > 0 &&
			activeCategoryKey === null &&
			browseMode === "typeahead",
	});

	const searchResults = searchResultsQuery.data ?? [];
	const searchResultsLoading =
		previewQueryPending || searchResultsQuery.isFetching;
	const searchResultsRef = useRef(searchResults);
	searchResultsRef.current = searchResults;

	const optionItems = useMemo(() => {
		if (activeCategoryKey && activeOptions) {
			return activeOptions.map((option) =>
				chipToken(activeCategoryKey, option.value),
			);
		}
		if (activeCategoryKey === null && browseMode === "typeahead") {
			return [
				...listedCategories.map((category) => category.key),
				...valueSuggestions.map((suggestion) => suggestion.token),
				...searchResults.map((result) => searchResultToken(result.value)),
			];
		}
		return [] as string[];
	}, [
		activeCategoryKey,
		activeOptions,
		browseMode,
		listedCategories,
		searchResults,
		valueSuggestions,
	]);
	const optionItemsRef = useRef(optionItems);
	optionItemsRef.current = optionItems;

	const restoreFreeTextInput = () => {
		setInputValue(committedFreeTextRef.current);
	};

	const exitFacetMode = () => {
		facetModeRef.current = false;
		setActiveCategoryKey(null);
		restoreFreeTextInput();
	};

	const enterCategoryMode = (categoryKey: string, query = "") => {
		facetModeRef.current = true;
		setBrowseModeSafe(null);
		setActiveCategoryKey(categoryKey);
		setInputValue(query);
		setOpen(true);
	};

	const updateFromChips = (tokens: string[], freeText?: string) => {
		const nextFreeText =
			freeText === undefined ? committedFreeTextRef.current : freeText;
		if (freeText !== undefined) {
			setCommittedNameSearch(freeText);
		}
		emitQuery(
			composeFilterQuery(
				chipsToValues(tokens, chipKeys),
				chipKeys,
				nextFreeText,
			),
			true,
		);
	};

	const selectCategory = (
		categoryKey: string,
		options?: Readonly<{ clearMatchedQuery?: boolean }>,
	) => {
		if (options?.clearMatchedQuery) {
			setCommittedNameSearch("");
			emitQuery(
				composeFilterQuery(chipsToValues(chipValues, chipKeys), chipKeys, ""),
				true,
			);
		} else if (activeCategoryKey === null) {
			setCommittedNameSearch(inputValue);
			emitQuery(
				composeFilterQuery(
					chipsToValues(chipValues, chipKeys),
					chipKeys,
					inputValue,
				),
				true,
			);
		}
		enterCategoryMode(categoryKey);
	};

	const selectValueSuggestion = (token: string) => {
		updateFromChips([...chipValues, token], "");
		facetModeRef.current = false;
		setBrowseModeSafe(null);
		setActiveCategoryKey(null);
		setInputValue("");
		setOpen(false);
	};

	const selectSearchResult = (result: SearchResult) => {
		onSearchResultSelectRef.current?.(result);
		facetModeRef.current = false;
		setBrowseModeSafe(null);
		setActiveCategoryKey(null);
		setOpen(false);
	};

	const handleInputFocus = () => {
		if (activeCategoryKey !== null || facetModeRef.current) {
			return;
		}
		openCategorySuggestions();
	};

	const handleInputValueChange = (
		nextValue: string,
		eventDetails: { reason: string },
	) => {
		if (eventDetails.reason === "input-clear") {
			if (activeCategoryKey === null) {
				restoreFreeTextInput();
				return;
			}
			setInputValue("");
			return;
		}

		const typedCategory = parseTypedCategoryPrefix(nextValue, categories);
		if (typedCategory) {
			setCommittedNameSearch(typedCategory.freeText);
			emitQuery(
				composeFilterQuery(
					chipsToValues(chipValues, chipKeys),
					chipKeys,
					typedCategory.freeText,
				),
				true,
			);
			enterCategoryMode(typedCategory.categoryKey, typedCategory.query);
			return;
		}

		if (activeCategory) {
			setInputValue(nextValue);
			facetModeRef.current = true;
			setBrowseModeSafe(null);
			setOpen(true);
			return;
		}

		setCommittedNameSearch(nextValue);
		setInputValue(nextValue);
		emitQuery(
			composeFilterQuery(
				chipsToValues(chipValues, chipKeys),
				chipKeys,
				nextValue,
			),
			false,
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
			setActiveCategoryKey(null);
		}
	};

	const handleValueChange = (nextTokens: string[]) => {
		const added = nextTokens.find((token) => !chipValues.includes(token));
		if (added) {
			const searchValue = parseSearchResultToken(added);
			if (searchValue) {
				const result = searchResultsRef.current.find(
					(entry) => entry.value === searchValue,
				);
				if (result) {
					selectSearchResult(result);
				}
				return;
			}

			const matchedCategory = categories.find(
				(category) => category.key === added,
			);
			if (activeCategoryKey === null && matchedCategory) {
				selectCategory(matchedCategory.key, { clearMatchedQuery: true });
				return;
			}

			if (
				activeCategoryKey === null &&
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
		setActiveCategoryKey(null);
		setInputValue(committedFreeTextRef.current);
		setOpen(false);
	};

	const exitActiveCategory = () => {
		facetModeRef.current = false;
		setBrowseModeSafe(null);
		setActiveCategoryKey(null);
		restoreFreeTextInput();
		setOpen(false);
	};

	const handleInputKeyDown = (event: {
		key: string;
		shiftKey?: boolean;
		preventDefault: () => void;
		preventBaseUIHandler?: () => void;
	}) => {
		if (event.key === "Backspace" && inputValue === "" && activeCategory) {
			event.preventDefault();
			event.preventBaseUIHandler?.();
			exitActiveCategory();
			return;
		}

		const isTabComplete = event.key === "Tab" && !event.shiftKey;
		const isCompleteKey = event.key === "Enter" || isTabComplete;
		if (!isCompleteKey || browseModeRef.current !== "typeahead") {
			return;
		}

		const topItem = optionItemsRef.current[0];
		if (!topItem) {
			return;
		}

		event.preventDefault();
		event.preventBaseUIHandler?.();

		const searchValue = parseSearchResultToken(topItem);
		if (searchValue) {
			const result = searchResultsRef.current.find(
				(entry) => entry.value === searchValue,
			);
			if (result) {
				selectSearchResult(result);
			}
			return;
		}

		const topCategory = listedCategoriesRef.current.find(
			(category) => category.key === topItem,
		);
		if (topCategory) {
			selectCategory(topCategory.key, { clearMatchedQuery: true });
			return;
		}

		if (parseChipToken(topItem, chipKeys)) {
			selectValueSuggestion(topItem);
		}
	};

	const setInputRef = (node: HTMLInputElement | null) => {
		storeInputRef.current = node;
	};

	return {
		open,
		browseMode,
		inputValue,
		committedFreeText,
		activeCategoryKey,
		activeCategory,
		activeOptions,
		activeOptionsLoading,
		listedCategories,
		valueSuggestions,
		valueSuggestionsLoading,
		searchResults,
		searchResultsLoading,
		chipValues,
		optionItems,
		selectCategory,
		exitActiveCategory,
		toggleFilterMenu,
		setInputRef,
		handleInputFocus,
		handleInputKeyDown,
		handleInputValueChange,
		handleOpenChange,
		handleValueChange,
	};
};
