import { useEffect, useMemo, useRef, useState } from "react";
import { useQueries, useQuery } from "react-query";
import { useDebouncedFunction, useDebouncedValue } from "#/hooks/debounce";
import {
	collectValueSuggestions,
	composeFilterQuery,
	extractFreeText,
	matchCategories,
	parseChipToken,
	parseTypedCategoryPrefix,
	queryToChips,
} from "./filterQuery";
import type { FilterCategory, FilterOption, SearchResult } from "./types";

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

export const useFilterCombobox = ({
	value,
	onChange,
	categories,
	getSearchResults,
	onSearchResultSelect,
}: UseFilterComboboxOptions) => {
	const chipKeys = useMemo(
		() => categories.flatMap((category) => category.chipKeys ?? [category.key]),
		[categories],
	);
	const [open, setOpen] = useState(false);
	const [isBrowsing, setIsBrowsing] = useState(false);
	const [activeCategoryKey, setActiveCategoryKey] = useState<string | null>(
		null,
	);
	const [inputValue, setInputValue] = useState(() =>
		extractFreeText(value, chipKeys),
	);
	const [committedFreeText, setCommittedFreeText] = useState(() =>
		extractFreeText(value, chipKeys),
	);
	const committedFreeTextRef = useRef(committedFreeText);
	const facetModeRef = useRef(false);
	const isBrowsingRef = useRef(false);
	const lastEmittedRef = useRef(value);
	const highlightedItemRef = useRef<string | null>(null);
	const storeInputRef = useRef<HTMLInputElement | null>(null);
	const onChangeRef = useRef(onChange);
	onChangeRef.current = onChange;
	const onSearchResultSelectRef = useRef(onSearchResultSelect);
	onSearchResultSelectRef.current = onSearchResultSelect;
	const categoriesRef = useRef(categories);
	categoriesRef.current = categories;
	const getSearchResultsRef = useRef(getSearchResults);
	getSearchResultsRef.current = getSearchResults;
	const hasSearchResults = Boolean(getSearchResults);

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

	const setBrowsing = (next: boolean) => {
		isBrowsingRef.current = next;
		setIsBrowsing(next);
	};

	const setCommittedNameSearch = (nextValue: string) => {
		const next = nextValue.trim();
		committedFreeTextRef.current = next;
		setCommittedFreeText(next);
	};

	const restoreFreeTextInput = () => {
		setInputValue(committedFreeTextRef.current);
	};

	const resetPopup = (input: "restore" | "clear" | "keep") => {
		facetModeRef.current = false;
		setBrowsing(false);
		setActiveCategoryKey(null);
		setOpen(false);
		if (input === "restore") {
			restoreFreeTextInput();
		} else if (input === "clear") {
			setInputValue("");
		}
	};

	const enterBrowsing = () => {
		facetModeRef.current = false;
		setBrowsing(true);
		setActiveCategoryKey(null);
		setOpen(true);
	};

	const enterCategoryMode = (categoryKey: string, query = "") => {
		facetModeRef.current = true;
		setBrowsing(false);
		setActiveCategoryKey(categoryKey);
		setInputValue(query);
		setOpen(true);
	};

	useEffect(() => {
		if (value === lastEmittedRef.current) {
			return;
		}
		lastEmittedRef.current = value;
		const freeText = extractFreeText(value, chipKeys);
		committedFreeTextRef.current = freeText;
		setCommittedFreeText(freeText);
		if (!facetModeRef.current) {
			setInputValue(freeText);
		}
	}, [value, chipKeys]);

	const toggleFilterMenu = () => {
		if (open) {
			resetPopup("restore");
			return;
		}
		enterBrowsing();
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

	const listedCategories = useMemo(() => {
		if (activeCategoryKey !== null || !isBrowsing) {
			return [] as FilterCategory[];
		}
		if (inputValue.trim().length === 0) {
			return [...categories];
		}
		return matchCategories(inputValue, categories);
	}, [activeCategoryKey, isBrowsing, categories, inputValue]);
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
	const activeOptionsError =
		activeCategoryKey !== null && activeOptionsQuery.isError;
	const activeOptionsLoading =
		activeOptionsPending ||
		(activeCategoryKey !== null &&
			!activeOptionsError &&
			(activeOptionsQuery.isFetching || activeOptions === undefined));
	const retryActiveOptions = () => {
		void activeOptionsQuery.refetch();
	};

	const typeaheadQuerySource =
		activeCategoryKey === null && isBrowsing ? inputValue.trim() : "";
	const debouncedTypeaheadQuery = useDebouncedValue(
		typeaheadQuerySource,
		SEARCH_DEBOUNCE_MS,
	);
	const typeaheadQueryPending =
		typeaheadQuerySource.length > 0 &&
		typeaheadQuerySource !== debouncedTypeaheadQuery;

	const suggestionQueries = useQueries({
		queries: categories.map((category) => ({
			queryKey: filterComboboxOptionsKey(category.key, debouncedTypeaheadQuery),
			queryFn: () => category.getOptions(debouncedTypeaheadQuery),
			enabled:
				debouncedTypeaheadQuery.length > 0 &&
				activeCategoryKey === null &&
				isBrowsing,
		})),
	});

	// react-query keeps each query's `data` reference stable between renders,
	// but useQueries returns a fresh array wrapper every render, so memoizing
	// on it never hits. The map is tiny (a handful of categories), so build it
	// directly instead of pretending to memoize.
	const optionsByKey = new Map<string, readonly FilterOption[]>();
	for (const [index, category] of categories.entries()) {
		const options = suggestionQueries[index]?.data;
		if (options) {
			optionsByKey.set(category.key, options);
		}
	}

	const valueSuggestions =
		activeCategoryKey !== null || !isBrowsing
			? []
			: collectValueSuggestions(
					inputValue,
					categories,
					optionsByKey,
					chipValues,
				);

	// A rejected suggestion query must not leave the popup spinning forever;
	// treat an error as "done loading" and surface it instead.
	const suggestionsError =
		activeCategoryKey === null &&
		isBrowsing &&
		suggestionQueries.some((query) => query.isError);
	const valueSuggestionsLoading =
		activeCategoryKey === null &&
		isBrowsing &&
		inputValue.trim().length > 0 &&
		!suggestionsError &&
		(typeaheadQueryPending ||
			suggestionQueries.some(
				(query) =>
					query.isFetching || (!query.isError && query.data === undefined),
			));

	const searchResultsQuery = useQuery({
		queryKey: filterComboboxSearchResultsKey(debouncedTypeaheadQuery),
		queryFn: () => {
			const loader = getSearchResultsRef.current;
			if (!loader) {
				return Promise.resolve([] as SearchResult[]);
			}
			return loader(debouncedTypeaheadQuery);
		},
		enabled:
			hasSearchResults &&
			debouncedTypeaheadQuery.length > 0 &&
			activeCategoryKey === null &&
			isBrowsing,
	});

	const searchResults = searchResultsQuery.data ?? [];
	const searchResultsLoading =
		(hasSearchResults && typeaheadQueryPending) ||
		(searchResultsQuery.isFetching && !searchResultsQuery.isError);
	const typeaheadError =
		activeCategoryKey === null &&
		isBrowsing &&
		(suggestionsError || (hasSearchResults && searchResultsQuery.isError));

	const updateFromChips = (tokens: string[], freeText?: string) => {
		const nextFreeText =
			freeText === undefined ? committedFreeTextRef.current : freeText;
		if (freeText !== undefined) {
			setCommittedNameSearch(freeText);
		}
		emitQuery(composeFilterQuery(tokens, chipKeys, nextFreeText), true);
	};

	const selectCategory = (categoryKey: string) => {
		setCommittedNameSearch("");
		emitQuery(composeFilterQuery(chipValues, chipKeys, ""), true);
		enterCategoryMode(categoryKey);
	};

	const selectValueSuggestion = (token: string) => {
		updateFromChips([...chipValues, token], "");
		resetPopup("clear");
	};

	// In category mode, choosing an option commits its chip while preserving the
	// free-text name search that preceded the category prefix.
	const selectCategoryOption = (token: string) => {
		updateFromChips([...chipValues, token]);
		resetPopup("restore");
	};

	const selectSearchResult = (result: SearchResult) => {
		onSearchResultSelectRef.current?.(result);
		resetPopup("keep");
	};

	const handleInputFocus = () => {
		if (activeCategoryKey !== null || facetModeRef.current) {
			return;
		}
		enterBrowsing();
	};

	const handleInputValueChange = (nextValue: string) => {
		const typedCategory = parseTypedCategoryPrefix(nextValue, categories);
		if (typedCategory) {
			setCommittedNameSearch(typedCategory.freeText);
			emitQuery(
				composeFilterQuery(chipValues, chipKeys, typedCategory.freeText),
				true,
			);
			enterCategoryMode(typedCategory.categoryKey, typedCategory.query);
			return;
		}

		if (activeCategory) {
			setInputValue(nextValue);
			facetModeRef.current = true;
			setBrowsing(false);
			setOpen(true);
			return;
		}

		setCommittedNameSearch(nextValue);
		setInputValue(nextValue);
		emitQuery(composeFilterQuery(chipValues, chipKeys, nextValue), false);
		enterBrowsing();
	};

	// Radix only originates close requests (escape / outside press); opens flow
	// from the caller, so a dismissal simply restores the free-text input.
	const handleDismiss = () => {
		resetPopup("restore");
	};

	const handleRemoveChip = (token: string) => {
		updateFromChips(chipValues.filter((entry) => entry !== token));
		resetPopup("restore");
	};

	const handleInputKeyDown = (event: {
		key: string;
		shiftKey?: boolean;
		preventDefault: () => void;
	}) => {
		if (event.key === "Backspace" && inputValue === "" && activeCategory) {
			event.preventDefault();
			resetPopup("restore");
			return;
		}

		if (
			(event.key === "Backspace" || event.key === "Delete") &&
			inputValue === "" &&
			!activeCategory &&
			chipValues.length > 0
		) {
			event.preventDefault();
			updateFromChips(chipValues.slice(0, -1));
			return;
		}

		// Enter is committed by cmdk through the highlighted item's `onSelect`.
		// Tab completes the highlighted filter only. A highlighted resource
		// preview has no chip token, so Tab falls through to the default focus
		// move rather than navigating.
		const isTabComplete = event.key === "Tab" && !event.shiftKey;
		if (!isTabComplete || !isBrowsingRef.current) {
			return;
		}

		const highlighted = highlightedItemRef.current;
		if (!highlighted) {
			return;
		}

		const category = listedCategoriesRef.current.find(
			(entry) => entry.key === highlighted,
		);
		if (category) {
			event.preventDefault();
			selectCategory(category.key);
			return;
		}

		if (parseChipToken(highlighted, chipKeys)) {
			event.preventDefault();
			selectValueSuggestion(highlighted);
		}
	};

	return {
		open,
		isBrowsing,
		inputValue,
		committedFreeText,
		activeCategoryKey,
		activeCategory,
		activeOptions,
		activeOptionsLoading,
		activeOptionsError,
		retryActiveOptions,
		listedCategories,
		valueSuggestions,
		valueSuggestionsLoading,
		typeaheadError,
		searchResults,
		searchResultsLoading,
		chipValues,
		toggleFilterMenu,
		selectCategory,
		selectCategoryOption,
		selectValueSuggestion,
		selectSearchResult,
		handleRemoveChip,
		setInputRef: (node: HTMLInputElement | null) => {
			storeInputRef.current = node;
		},
		handleInputFocus,
		handleInputKeyDown,
		handleItemHighlighted: (highlightedValue: string | undefined) => {
			highlightedItemRef.current = highlightedValue ?? null;
		},
		handleInputValueChange,
		handleDismiss,
	};
};
