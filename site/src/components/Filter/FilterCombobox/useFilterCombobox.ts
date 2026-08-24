import {
	type KeyboardEvent as ReactKeyboardEvent,
	useEffect,
	useMemo,
	useReducer,
	useRef,
} from "react";
import { useQueries, useQuery } from "react-query";
import { useDebouncedFunction, useDebouncedValue } from "#/hooks/debounce";
import {
	collectValueSuggestions,
	composeFilterQuery,
	dedupeChips,
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

/**
 * The popup has three mutually exclusive modes. `closed` hides it; `browsing`
 * lists categories plus free-text typeahead suggestions; `category` narrows to
 * one category's options. `open` and `isBrowsing` are derived from `mode`.
 */
type Mode = "closed" | "browsing" | "category";

type State = {
	mode: Mode;
	activeCategoryKey: string | null;
	inputValue: string;
	committedFreeText: string;
};

type Action =
	| { type: "openBrowsing" }
	| {
			type: "enterCategory";
			categoryKey: string;
			query: string;
			committedFreeText: string;
	  }
	| { type: "typeInCategory"; value: string }
	| { type: "typeFreeText"; value: string }
	| { type: "setCommittedFreeText"; value: string }
	| { type: "close"; input: "restore" | "clear" | "keep" }
	| { type: "reconcile"; freeText: string };

const closeState = (
	state: State,
	input: "restore" | "clear" | "keep",
): State => ({
	mode: "closed",
	activeCategoryKey: null,
	committedFreeText: state.committedFreeText,
	inputValue:
		input === "restore"
			? state.committedFreeText
			: input === "clear"
				? ""
				: state.inputValue,
});

const reducer = (state: State, action: Action): State => {
	switch (action.type) {
		case "openBrowsing":
			return { ...state, mode: "browsing", activeCategoryKey: null };
		case "enterCategory":
			return {
				mode: "category",
				activeCategoryKey: action.categoryKey,
				inputValue: action.query,
				committedFreeText: action.committedFreeText.trim(),
			};
		case "typeInCategory":
			return { ...state, mode: "category", inputValue: action.value };
		case "typeFreeText":
			return {
				mode: "browsing",
				activeCategoryKey: null,
				inputValue: action.value,
				committedFreeText: action.value.trim(),
			};
		case "setCommittedFreeText":
			return { ...state, committedFreeText: action.value.trim() };
		case "close":
			return closeState(state, action.input);
		case "reconcile":
			return {
				...state,
				committedFreeText: action.freeText,
				// A pending category selection owns the input, so only the free-text
				// view mirrors an external value.
				inputValue:
					state.mode === "category" ? state.inputValue : action.freeText,
			};
	}
};

type UseFilterComboboxOptions = {
	value: string;
	onChange: (query: string) => void;
	categories: readonly FilterCategory[];
	getSearchResults?: (query: string) => Promise<SearchResult[]>;
	onSearchResultSelect?: (result: SearchResult) => void;
};

/**
 * Drives the unified workspace filter combobox: a `mode` state machine for the
 * popup, debounced query emission back to the caller, and the react-query
 * lookups for category options, cross-category suggestions, and search results.
 * Local state is reconciled against the caller-owned `value` so an external
 * update wins over any in-flight local edit.
 */
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

	const [state, dispatch] = useReducer(reducer, chipKeys, (keys): State => {
		const freeText = extractFreeText(value, keys);
		return {
			mode: "closed",
			activeCategoryKey: null,
			inputValue: freeText,
			committedFreeText: freeText,
		};
	});
	const { mode, activeCategoryKey, inputValue, committedFreeText } = state;
	const open = mode !== "closed";
	const isBrowsing = mode === "browsing";

	const lastEmittedRef = useRef(value);
	const prevChipKeysRef = useRef(chipKeys);
	const highlightedItemRef = useRef<string | null>(null);
	const inputRef = useRef<HTMLInputElement | null>(null);
	const onChangeRef = useRef(onChange);
	onChangeRef.current = onChange;
	const onSearchResultSelectRef = useRef(onSearchResultSelect);
	onSearchResultSelectRef.current = onSearchResultSelect;
	const categoriesRef = useRef(categories);
	categoriesRef.current = categories;
	const getSearchResultsRef = useRef(getSearchResults);
	getSearchResultsRef.current = getSearchResults;
	const hasSearchResultsLoader = Boolean(getSearchResults);

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

	// Reconcile local state with the caller-owned `value`. Reparse whenever the
	// value changes externally or the chip categories change, so renaming or
	// adding a category recomputes the free text instead of being suppressed by
	// the self-emit guard.
	useEffect(() => {
		const chipKeysChanged = prevChipKeysRef.current !== chipKeys;
		prevChipKeysRef.current = chipKeys;
		const isExternal = value !== lastEmittedRef.current;
		if (!isExternal && !chipKeysChanged) {
			return;
		}
		if (isExternal) {
			// An authoritative external value must win, so drop any pending local
			// write before adopting it.
			cancelDebounce();
			lastEmittedRef.current = value;
		}
		dispatch({ type: "reconcile", freeText: extractFreeText(value, chipKeys) });
	}, [value, chipKeys, cancelDebounce]);

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
			hasSearchResultsLoader &&
			debouncedTypeaheadQuery.length > 0 &&
			activeCategoryKey === null &&
			isBrowsing,
	});

	const searchResults = searchResultsQuery.data ?? [];
	const searchResultsLoading =
		(hasSearchResultsLoader && typeaheadQueryPending) ||
		(searchResultsQuery.isFetching && !searchResultsQuery.isError);
	const previewError =
		activeCategoryKey === null &&
		isBrowsing &&
		hasSearchResultsLoader &&
		searchResultsQuery.isError;
	const typeaheadError = suggestionsError || previewError;

	const typeaheadActive = activeCategoryKey === null && isBrowsing;
	const hasTypeaheadQuery = typeaheadActive && inputValue.trim().length > 0;
	const showSearchResults = hasTypeaheadQuery && searchResults.length > 0;
	// Spin while either source is still fetching without rows for its own
	// section, so an in-flight query never leaves an empty gap. The other
	// section's rows keep rendering above the spinner.
	const typeaheadLoading =
		hasTypeaheadQuery &&
		((valueSuggestionsLoading && valueSuggestions.length === 0) ||
			(hasSearchResultsLoader &&
				searchResultsLoading &&
				searchResults.length === 0));

	const typeaheadEmpty =
		hasTypeaheadQuery &&
		!typeaheadLoading &&
		!typeaheadError &&
		listedCategories.length === 0 &&
		valueSuggestions.length === 0 &&
		searchResults.length === 0;

	// Name the failing source so the copy points at the right endpoint instead
	// of blaming suggestions for a preview outage.
	const typeaheadErrorLabel =
		suggestionsError && previewError
			? "Couldn't load results."
			: previewError
				? "Couldn't load workspace previews."
				: suggestionsError
					? "Couldn't load suggestions."
					: "";

	const activeOptionsEmpty =
		activeCategoryKey !== null &&
		!activeOptionsLoading &&
		!activeOptionsError &&
		activeOptions !== undefined &&
		activeOptions.length === 0;

	// Announce a live-region message for each terminal state so screen readers
	// hear loading, failures, and empty results rather than silence.
	let statusMessage = "";
	if (activeCategory) {
		statusMessage = activeOptionsLoading
			? `Loading ${activeCategory.label} options`
			: activeOptionsError
				? `Couldn't load ${activeCategory.label} options`
				: activeOptionsEmpty
					? `No ${activeCategory.label} matches`
					: `Filtering by ${activeCategory.label}`;
	} else if (typeaheadLoading) {
		statusMessage = "Loading suggestions";
	} else if (typeaheadError) {
		statusMessage = typeaheadErrorLabel;
	} else if (typeaheadEmpty) {
		statusMessage = "No filters found";
	}

	// Refetch both typeahead sources so one retry covers a failed suggestion
	// lookup and a failed workspace preview.
	const retryTypeahead = () => {
		for (const query of suggestionQueries) {
			void query.refetch();
		}
		void searchResultsQuery.refetch();
	};

	const updateFromChips = (tokens: string[], freeText?: string) => {
		const nextFreeText = freeText ?? committedFreeText;
		if (freeText !== undefined) {
			dispatch({ type: "setCommittedFreeText", value: freeText });
		}
		emitQuery(composeFilterQuery(tokens, chipKeys, nextFreeText), true);
	};

	const selectCategory = (categoryKey: string) => {
		emitQuery(composeFilterQuery(chipValues, chipKeys, ""), true);
		dispatch({
			type: "enterCategory",
			categoryKey,
			query: "",
			committedFreeText: "",
		});
	};

	const selectValueSuggestion = (token: string) => {
		updateFromChips([...chipValues, token], "");
		dispatch({ type: "close", input: "clear" });
	};

	// In category mode, choosing an option commits its chip while preserving the
	// free-text name search that preceded the category prefix.
	const selectCategoryOption = (token: string) => {
		updateFromChips([...chipValues, token]);
		dispatch({ type: "close", input: "restore" });
	};

	const selectSearchResult = (result: SearchResult) => {
		onSearchResultSelectRef.current?.(result);
		dispatch({ type: "close", input: "keep" });
	};

	const toggleFilterMenu = () => {
		if (open) {
			dispatch({ type: "close", input: "restore" });
			return;
		}
		dispatch({ type: "openBrowsing" });
		queueMicrotask(() => {
			inputRef.current?.focus();
		});
	};

	const handleInputFocus = () => {
		if (mode === "category") {
			return;
		}
		dispatch({ type: "openBrowsing" });
	};

	const handleInputValueChange = (nextValue: string) => {
		const typedCategory = parseTypedCategoryPrefix(nextValue, categories);
		if (typedCategory) {
			// Promote any chip tokens sitting in the prefix's free text (e.g. a
			// pasted `owner:me template:docker`) instead of re-emitting them as
			// free text, which would duplicate the token on the next commit.
			const priorChips = queryToChips(typedCategory.freeText, chipKeys);
			const mergedChips = dedupeChips([...chipValues, ...priorChips], chipKeys);
			const cleanedFreeText = extractFreeText(typedCategory.freeText, chipKeys);
			emitQuery(
				composeFilterQuery(mergedChips, chipKeys, cleanedFreeText),
				true,
			);
			dispatch({
				type: "enterCategory",
				categoryKey: typedCategory.categoryKey,
				query: typedCategory.query,
				committedFreeText: cleanedFreeText,
			});
			return;
		}

		if (mode === "category") {
			dispatch({ type: "typeInCategory", value: nextValue });
			return;
		}

		// Promote chip tokens the user has finished (a trailing space marks the
		// last token complete). A chip-shaped token still being typed stays in the
		// input and is withheld from the emitted query so it does not commit a
		// half-typed chip such as `dormant:t`.
		const endsWithSpace = /\s$/.test(nextValue);
		const fragments = nextValue.split(/\s+/).filter(Boolean);
		const inProgress = endsWithSpace ? "" : (fragments.pop() ?? "");
		const settledText = fragments.join(" ");
		const settledChips = queryToChips(settledText, chipKeys);
		const inProgressIsPartialChip =
			inProgress.length > 0 && queryToChips(inProgress, chipKeys).length > 0;

		if (settledChips.length > 0 || inProgressIsPartialChip) {
			const mergedChips = dedupeChips(
				[...chipValues, ...settledChips],
				chipKeys,
			);
			const settledFreeText = extractFreeText(settledText, chipKeys);
			const inputFreeText = inProgressIsPartialChip
				? [settledFreeText, inProgress].filter(Boolean).join(" ")
				: settledFreeText;
			emitQuery(
				composeFilterQuery(mergedChips, chipKeys, settledFreeText),
				true,
			);
			dispatch({ type: "typeFreeText", value: inputFreeText });
			return;
		}

		emitQuery(composeFilterQuery(chipValues, chipKeys, nextValue), false);
		dispatch({ type: "typeFreeText", value: nextValue });
	};

	// Radix only originates close requests (escape / outside press); opens flow
	// from the caller, so a dismissal simply restores the free-text input.
	const handleDismiss = () => {
		dispatch({ type: "close", input: "restore" });
	};

	// Chip removal mirrors Backspace: drop the token and keep the current popup
	// and input state untouched.
	const handleRemoveChip = (token: string) => {
		updateFromChips(chipValues.filter((entry) => entry !== token));
	};

	const handleInputKeyDown = (event: ReactKeyboardEvent<HTMLInputElement>) => {
		const isBackspaceOrDelete =
			event.key === "Backspace" || event.key === "Delete";

		if (isBackspaceOrDelete && inputValue === "" && mode === "category") {
			event.preventDefault();
			dispatch({ type: "close", input: "restore" });
			return;
		}

		if (
			isBackspaceOrDelete &&
			inputValue === "" &&
			mode !== "category" &&
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
		if (!isTabComplete || mode !== "browsing") {
			return;
		}

		const highlighted = highlightedItemRef.current;
		if (!highlighted) {
			return;
		}

		const category = listedCategories.find(
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
		inputValue,
		committedFreeText,
		activeCategoryKey,
		activeCategory,
		activeOptions,
		activeOptionsLoading,
		activeOptionsError,
		statusMessage,
		listedCategories,
		valueSuggestions,
		searchResults,
		chipValues,
		// Derived typeahead view-model, so the view renders flags instead of
		// recomputing loading/visibility from raw query state.
		typeahead: {
			active: typeaheadActive,
			loading: typeaheadLoading,
			error: typeaheadError,
			errorLabel: typeaheadErrorLabel,
			showSearchResults,
		},
		actions: {
			setInputRef: (node: HTMLInputElement | null) => {
				inputRef.current = node;
			},
			toggleMenu: toggleFilterMenu,
			dismiss: handleDismiss,
			removeChip: handleRemoveChip,
			retryActiveOptions,
			retryTypeahead,
			selectCategory,
			selectCategoryOption,
			selectValueSuggestion,
			selectSearchResult,
			onInputFocus: handleInputFocus,
			onInputKeyDown: handleInputKeyDown,
			onInputValueChange: handleInputValueChange,
			onItemHighlighted: (highlightedValue: string | undefined) => {
				highlightedItemRef.current = highlightedValue ?? null;
			},
		},
	};
};
