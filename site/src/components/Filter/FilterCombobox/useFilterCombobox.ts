import {
	type KeyboardEvent as ReactKeyboardEvent,
	useCallback,
	useEffect,
	useLayoutEffect,
	useMemo,
	useReducer,
	useRef,
} from "react";
import { useQueries, useQuery, useQueryClient } from "react-query";
import { useDebouncedFunction, useDebouncedValue } from "#/hooks/debounce";
import {
	chipToken,
	collectValueSuggestions,
	composeFilterQuery,
	extractFreeText,
	filterOptionsByText,
	matchCategories,
	optionToken,
	parseChipToken,
	parseTypedCategoryPrefix,
	queryToChips,
} from "./filterQuery";
import type { FilterComboboxHighlight } from "./primitives";
import { filterComboboxOptions, SEARCH_DEBOUNCE_MS } from "./queries";
import type { FilterCategory, FilterOption } from "./types";

/**
 * The popup has three mutually exclusive modes. `closed` hides it; `browsing`
 * lists categories, inline option rows, and typeahead suggestions for typed
 * text (`browseAll` below changes how typed text is treated); `category`
 * narrows to one category's options. `open` and `isBrowsing` are derived from
 * `mode`.
 */
type Mode = "closed" | "browsing" | "category";

type State = {
	mode: Mode;
	/**
	 * True when the menu was opened from the filter button or by stepping back
	 * out of a category. The full category list is shown, and free text typed
	 * before that stays a free-text search instead of narrowing filters.
	 * Typing leaves browse-all.
	 */
	browseAll: boolean;
	activeCategoryKey: string | null;
	inputValue: string;
	/**
	 * Text typed outside chips and category prefixes; its last token can be a
	 * chip token still being typed. Plain typed text is withheld from the
	 * emitted query while the typed-text lookup says it could be a filter, and
	 * is emitted when the lookup finds no match or `applyTypedSearch` runs.
	 * Text typed with a chip token or before a category prefix is emitted at
	 * once, and a chip token still being typed waits until it is committed as a
	 * chip.
	 */
	typedFreeText: string;
};

type Action =
	| { type: "openBrowsing" }
	| { type: "showAllFilters" }
	| {
			type: "enterCategory";
			categoryKey: string;
			query: string;
			typedFreeText: string;
	  }
	| { type: "typeInCategory"; value: string }
	| { type: "typeFilterSearch"; value: string }
	| { type: "typeFreeText"; value: string }
	| { type: "setTypedFreeText"; value: string }
	| { type: "leaveCategory" }
	| { type: "close" }
	| { type: "reconcile"; freeText: string };

const closeState = (state: State): State => ({
	mode: "closed",
	browseAll: false,
	activeCategoryKey: null,
	typedFreeText: state.typedFreeText,
	inputValue: state.typedFreeText,
});

const reducer = (state: State, action: Action): State => {
	switch (action.type) {
		case "openBrowsing":
			return {
				...state,
				mode: "browsing",
				browseAll: false,
				activeCategoryKey: null,
			};
		case "showAllFilters":
			return {
				...state,
				mode: "browsing",
				browseAll: true,
				activeCategoryKey: null,
			};
		case "enterCategory":
			return {
				mode: "category",
				browseAll: false,
				activeCategoryKey: action.categoryKey,
				inputValue: action.query,
				typedFreeText: action.typedFreeText.trim(),
			};
		case "typeInCategory":
			return { ...state, mode: "category", inputValue: action.value };
		case "typeFilterSearch":
			return {
				...state,
				mode: "browsing",
				browseAll: false,
				activeCategoryKey: null,
				inputValue: action.value,
			};
		case "typeFreeText":
			return {
				mode: "browsing",
				browseAll: false,
				activeCategoryKey: null,
				inputValue: action.value,
				typedFreeText: action.value.trim(),
			};
		case "setTypedFreeText":
			return { ...state, typedFreeText: action.value.trim() };
		case "leaveCategory":
			return {
				...state,
				mode: "browsing",
				browseAll: true,
				activeCategoryKey: null,
				inputValue: state.typedFreeText,
			};
		case "close":
			return closeState(state);
		case "reconcile":
			return {
				...state,
				browseAll: false,
				typedFreeText: action.freeText,
				// A pending category selection owns the input, so only the free-text
				// view mirrors an external value.
				inputValue:
					state.mode === "category" ? state.inputValue : action.freeText,
			};
	}
};

type StatusMessageInput = {
	activeCategoryLabel: string | undefined;
	activeOptionsLoading: boolean;
	activeOptionsError: boolean;
	activeOptionsEmpty: boolean;
	activeOptionsSearched: boolean;
	typeaheadLoading: boolean;
	typeaheadError: boolean;
	typeaheadEmpty: boolean;
	inlineLoadMessages: readonly string[];
};

// cmdk value of an inline category's loading or Retry row. Assumes no
// category key starts with "__load__".
const INLINE_LOAD_ROW_VALUE_PREFIX = "__load__:";

/** Shown and announced when a category's options fail to load. */
export const optionsLoadErrorMessage = (label: string) =>
	`Couldn’t load ${label} options.`;

/** Announced while a category's options load. */
export const optionsLoadingMessage = (label: string) =>
	`Loading ${label} options.`;

/** Announced when a category shows no options, with or without a search. */
const optionsEmptyMessage = (label: string, searched: boolean) =>
	searched ? `No ${label} matches.` : `No ${label} options.`;

/** Shown in an options panel with no options, with or without a search. */
export const optionsEmptyText = (searched: boolean) =>
	searched ? "No matching options" : "No options";

/**
 * Rows an options panel shows for trimmed `text`, given the category's
 * unfiltered options and `results`, which are `getOptions(text)`'s results
 * or undefined until they arrive. `getOptions` may return only the first page
 * for an empty query, so until then the unfiltered options are filtered
 * locally. `loading` is set while there is nothing to show, or while `text`
 * has no results and no local match, unless `failed` is set.
 */
export const shownOptions = ({
	unfiltered,
	results,
	text,
	failed,
}: {
	unfiltered: readonly FilterOption[] | undefined;
	results: readonly FilterOption[] | undefined;
	text: string;
	failed: boolean;
}) => {
	const searched = text.length > 0;
	const options = !searched
		? (unfiltered ?? results)
		: (results ?? (unfiltered && filterOptionsByText(unfiltered, text)));
	return {
		options,
		loading:
			!failed &&
			(options === undefined ||
				(searched && results === undefined && options.length === 0)),
	};
};

/**
 * Announces an options panel's state: loading, then a failed load, then no
 * options. Undefined when none of `loading`, `failed`, or `empty` is set.
 */
export const optionsStatusMessage = ({
	label,
	loading,
	failed,
	empty,
	searched,
}: {
	label: string;
	loading: boolean;
	failed: boolean;
	empty: boolean;
	searched: boolean;
}) => {
	if (loading) {
		return optionsLoadingMessage(label);
	}
	if (failed) {
		return optionsLoadErrorMessage(label);
	}
	return empty ? optionsEmptyMessage(label, searched) : undefined;
};

/**
 * Longest wait for option lookups before typed text that matched no loaded
 * filter is applied as a free-text search anyway.
 */
export const TYPED_TEXT_LOOKUP_TIMEOUT_MS = 1000;

/** Shown and announced when the typeahead suggestion queries fail. */
export const SUGGESTIONS_ERROR_MESSAGE = "Couldn’t load suggestions.";

// Live-region text for the hook's states so screen readers hear loading,
// failures, and empty results rather than silence. Typeahead loading shows no
// spinner, so this is its only announcement. FilterCombobox adds the open
// flyout's state, which is view state.
const deriveStatusMessage = ({
	activeCategoryLabel,
	activeOptionsLoading,
	activeOptionsError,
	activeOptionsEmpty,
	activeOptionsSearched,
	typeaheadLoading,
	typeaheadError,
	typeaheadEmpty,
	inlineLoadMessages,
}: StatusMessageInput): string => {
	if (activeCategoryLabel !== undefined) {
		return (
			optionsStatusMessage({
				label: activeCategoryLabel,
				loading: activeOptionsLoading,
				failed: activeOptionsError,
				empty: activeOptionsEmpty,
				searched: activeOptionsSearched,
			}) ?? `Filtering by ${activeCategoryLabel}`
		);
	}
	if (typeaheadLoading) {
		return "Loading suggestions";
	}
	if (typeaheadError) {
		return SUGGESTIONS_ERROR_MESSAGE;
	}
	if (typeaheadEmpty) {
		return "No filters found";
	}
	if (inlineLoadMessages.length > 0) {
		return inlineLoadMessages.join(" ");
	}
	return "";
};

type UseFilterComboboxOptions = {
	value: string;
	onChange: (query: string) => void;
	categories: readonly FilterCategory[];
};

/**
 * Drives `FilterCombobox`: a `mode` state machine for the popup, free-text
 * emission that waits for option lookups and withholds text matching a
 * filter, and the react-query lookups for category options and
 * cross-category suggestions.
 * Local state is reconciled against the caller-owned `value` so an external
 * update wins over any in-flight local edit.
 */
export const useFilterCombobox = ({
	value,
	onChange,
	categories,
}: UseFilterComboboxOptions) => {
	const chipKeys = useMemo(
		() => categories.flatMap((category) => category.chipKeys ?? [category.key]),
		[categories],
	);

	const [state, dispatch] = useReducer(reducer, chipKeys, (keys): State => {
		const freeText = extractFreeText(value, keys);
		return {
			mode: "closed",
			browseAll: false,
			activeCategoryKey: null,
			inputValue: freeText,
			typedFreeText: freeText,
		};
	});
	const { mode, browseAll, activeCategoryKey, inputValue, typedFreeText } =
		state;
	const open = mode !== "closed";
	const isBrowsing = mode === "browsing";

	const lastEmittedRef = useRef(value);
	const prevChipKeysRef = useRef(chipKeys);
	const highlightRef = useRef<FilterComboboxHighlight | null>(null);
	const getHighlightedValue = () => highlightRef.current?.get() ?? "";
	const setHighlightedValue = (value: string) => {
		highlightRef.current?.set(value);
	};
	// A load row's highlight hand-off applies only while the menu stays open,
	// so a load that finishes while it is closed does not move a reopened
	// menu's highlight.
	const closeMenu = () => {
		if (getHighlightedValue().startsWith(INLINE_LOAD_ROW_VALUE_PREFIX)) {
			setHighlightedValue("");
		}
		dispatch({ type: "close" });
	};
	const inputRef = useRef<HTMLInputElement | null>(null);

	const queryClient = useQueryClient();
	// cancelTypedTextLookup bumps this, so a typed-text lookup that resolves
	// after any cancel is dropped. emitQueryKeepingLookup does not cancel.
	const typedTextLookupGenerationRef = useRef(0);
	const {
		debounced: scheduleTypedTextLookup,
		cancelDebounce: cancelTypedTextLookupTimer,
	} = useDebouncedFunction(
		(text: string, generation: number) =>
			applyTypedTextUnlessFilter(text, generation),
		SEARCH_DEBOUNCE_MS,
	);
	const cancelTypedTextLookup = useCallback(() => {
		cancelTypedTextLookupTimer();
		typedTextLookupGenerationRef.current += 1;
	}, [cancelTypedTextLookupTimer]);
	useEffect(() => cancelTypedTextLookup, [cancelTypedTextLookup]);

	// A pending typed-text lookup reads the chips of the last sent query when
	// it resolves.
	const emitQueryKeepingLookup = (query: string) => {
		lastEmittedRef.current = query;
		onChange(query);
	};
	const emitQuery = (query: string) => {
		cancelTypedTextLookup();
		emitQueryKeepingLookup(query);
	};

	// Reconcile local state with the caller-owned `value`. Reparse whenever the
	// value changes externally or the chip categories change, so renaming or
	// adding a category recomputes the free text instead of being suppressed by
	// the self-emit guard. It runs during the commit, so a key handled before
	// passive effects flush already reads the caller's value.
	useLayoutEffect(() => {
		const chipKeysChanged = prevChipKeysRef.current !== chipKeys;
		prevChipKeysRef.current = chipKeys;
		const isExternal = value !== lastEmittedRef.current;
		if (!isExternal && !chipKeysChanged) {
			return;
		}
		if (isExternal) {
			// An authoritative external value must win, so drop any pending local
			// write before adopting it.
			cancelTypedTextLookup();
			lastEmittedRef.current = value;
		}
		dispatch({ type: "reconcile", freeText: extractFreeText(value, chipKeys) });
	}, [value, chipKeys, cancelTypedTextLookup]);

	const activeCategory = categories.find(
		(category) => category.key === activeCategoryKey,
	);
	const chipValues = useMemo(
		() => queryToChips(value, chipKeys),
		[chipKeys, value],
	);

	// Categories with a flyout or drill-in list. Inline categories render their
	// options directly in the main panel and never enter category mode.
	const submenuCategories = categories.filter(
		(category) => !category.inlineOptions,
	);
	const inlineCategories = categories.filter(
		(category) => category.inlineOptions,
	);
	const typeaheadActive =
		activeCategoryKey === null && isBrowsing && !browseAll;
	// A typed `status:` style prefix for an inline category narrows the main
	// panel to that category's options; the text after the colon is the query.
	const typedInlinePrefix = typeaheadActive
		? parseTypedCategoryPrefix(inputValue, inlineCategories)
		: null;

	// Each category's unfiltered options, fetched while browsing for the pointer
	// flyouts and always for inline categories, whose chips take their labels
	// from them. Shares its cache key with the category view.
	const unfilteredOptionsEnabled = isBrowsing && activeCategoryKey === null;
	const unfilteredOptions = useQueries({
		queries: categories.map((category) =>
			filterComboboxOptions(
				category.key,
				category.getOptions,
				"",
				unfilteredOptionsEnabled || Boolean(category.inlineOptions),
			),
		),
		combine: (results) => {
			const optionsByKey = new Map<string, readonly FilterOption[]>();
			const erroredKeys = new Set<string>();
			results.forEach((result, index) => {
				const { key } = categories[index];
				if (result.data) {
					optionsByKey.set(key, result.data);
				} else if (result.isError) {
					erroredKeys.add(key);
				}
			});
			return {
				optionsByKey,
				erroredKeys,
				refetch: (categoryKey: string) => {
					const index = categories.findIndex(
						(category) => category.key === categoryKey,
					);
					void results[index]?.refetch();
				},
			};
		},
	});

	const categoryQuery =
		activeCategoryKey !== null || browseAll ? "" : inputValue.trim();
	const listedCategories =
		!open || typedInlinePrefix !== null
			? []
			: categoryQuery.length === 0
				? submenuCategories
				: matchCategories(categoryQuery, submenuCategories);

	const activeOptionsQuerySource =
		activeCategoryKey !== null ? inputValue.trim() : "";
	const debouncedActiveOptionsQuery = useDebouncedValue(
		activeOptionsQuerySource,
		SEARCH_DEBOUNCE_MS,
	);
	const activeOptionsPending =
		activeCategoryKey !== null &&
		activeOptionsQuerySource !== debouncedActiveOptionsQuery;

	const activeOptionsQuery = useQuery(
		filterComboboxOptions(
			activeCategoryKey ?? "",
			activeCategory?.getOptions,
			debouncedActiveOptionsQuery,
			activeCategoryKey !== null,
		),
	);

	// The query still holds the previous text while a search is pending, so
	// its results and failure count only once the text settles. Until then
	// the panel shows the unfiltered options filtered by the typed text, so
	// Enter picks only a row that matches it.
	const activeOptionsError =
		activeCategoryKey !== null &&
		!activeOptionsPending &&
		activeOptionsQuery.isError;
	const activeShown = shownOptions({
		unfiltered:
			activeCategoryKey === null
				? undefined
				: unfilteredOptions.optionsByKey.get(activeCategoryKey),
		results: activeOptionsPending ? undefined : activeOptionsQuery.data,
		text: activeOptionsQuerySource,
		failed: activeOptionsError,
	});
	const activeOptions =
		activeCategoryKey === null || activeOptionsError
			? undefined
			: activeShown.options;
	const activeOptionsLoading =
		activeCategoryKey !== null && activeShown.loading;
	const retryActiveOptions = () => {
		void activeOptionsQuery.refetch();
	};

	const typeaheadQuerySource = typeaheadActive
		? (typedInlinePrefix?.query ?? inputValue).trim()
		: "";
	const debouncedTypeaheadQuery = useDebouncedValue(
		typeaheadQuerySource,
		SEARCH_DEBOUNCE_MS,
	);
	const typeaheadQueryPending =
		typeaheadQuerySource.length > 0 &&
		typeaheadQuerySource !== debouncedTypeaheadQuery;

	const suggestionOptions = useQueries({
		queries: categories.map((category) =>
			filterComboboxOptions(
				category.key,
				category.getOptions,
				debouncedTypeaheadQuery,
				debouncedTypeaheadQuery.length > 0 &&
					typeaheadActive &&
					(typedInlinePrefix === null ||
						typedInlinePrefix.categoryKey === category.key),
			),
		),
		// Derive the per-category map and loading/error flags through `combine` so
		// react-query's structural sharing drives recomputation, instead of the
		// fresh array wrapper `useQueries` returns each render.
		combine: (results) => {
			const optionsByKey = new Map<string, readonly FilterOption[]>();
			const erroredKeys = new Set<string>();
			results.forEach((result, index) => {
				if (result.data) {
					optionsByKey.set(categories[index].key, result.data);
				} else if (result.isError) {
					erroredKeys.add(categories[index].key);
				}
			});
			return {
				optionsByKey,
				erroredKeys,
				isError: erroredKeys.size > 0,
				// A not-yet-loaded (but not errored) query counts as fetching so the
				// status does not announce an empty result early. Disabled queries
				// stay idle and never load, so they are skipped.
				isFetching: results.some(
					(result) =>
						result.isFetching ||
						(result.fetchStatus !== "idle" &&
							!result.isError &&
							result.data === undefined),
				),
				refetch: () => {
					for (const result of results) {
						void result.refetch();
					}
				},
			};
		},
	});

	// A category whose query failed shows no rows; the retry row replaces
	// them.
	const typeaheadOptionsByKey = new Map<string, readonly FilterOption[]>();
	if (typeaheadQuerySource.length > 0) {
		for (const { key } of categories) {
			if (!typeaheadQueryPending && suggestionOptions.erroredKeys.has(key)) {
				continue;
			}
			const { options } = shownOptions({
				unfiltered: unfilteredOptions.optionsByKey.get(key),
				results: typeaheadQueryPending
					? undefined
					: suggestionOptions.optionsByKey.get(key),
				text: typeaheadQuerySource,
				failed: false,
			});
			if (options) {
				typeaheadOptionsByKey.set(key, options);
			}
		}
	}
	const inlineOptionsSource =
		typeaheadQuerySource.length === 0
			? unfilteredOptions.optionsByKey
			: typeaheadOptionsByKey;
	const inlineOptionRowsFor = (category: FilterCategory) =>
		(inlineOptionsSource.get(category.key) ?? []).map((option) => {
			const token = optionToken(category.key, option);
			return {
				token,
				selected: chipValues.includes(token),
				showIcon: category.inlineOptionsIcons ?? false,
				option,
			};
		});
	// Inline categories have no flyout, so the main panel shows their loading
	// row, or a failed load's error and Retry. Typed text replaces the
	// unfiltered rows and their load state.
	const inlineLoadStatus = (
		category: FilterCategory,
	): "ready" | "loading" | "failed" => {
		if (typeaheadQuerySource.length > 0) {
			return "ready";
		}
		if (unfilteredOptions.erroredKeys.has(category.key)) {
			return "failed";
		}
		return unfilteredOptions.optionsByKey.has(category.key)
			? "ready"
			: "loading";
	};
	const inlineSections = open
		? categories.flatMap((category) => {
				if (
					!category.inlineOptions ||
					(typedInlinePrefix !== null &&
						category.key !== typedInlinePrefix.categoryKey)
				) {
					return [];
				}
				const rows = inlineOptionRowsFor(category);
				const status = inlineLoadStatus(category);
				if (status === "ready" && rows.length === 0) {
					return [];
				}
				return [
					{
						category,
						heading: category.inlineOptionsLabel ?? `${category.label} is…`,
						rows,
						status,
						loadRowValue: `${INLINE_LOAD_ROW_VALUE_PREFIX}${category.key}`,
					},
				];
			})
		: [];
	const inlineOptionRows = inlineSections.flatMap((section) => section.rows);
	// cmdk highlights the first row when the highlighted row unmounts. When a
	// highlighted inline load row gives way to loaded options, the highlight
	// moves to that category's first option instead.
	const handleHighlightedValueChange = (_value: string, previous: string) => {
		const loadedSection = inlineSections.find(
			(section) => section.loadRowValue === previous,
		);
		const [firstRow] = loadedSection?.rows ?? [];
		if (loadedSection?.status === "ready" && firstRow) {
			setHighlightedValue(firstRow.token);
		}
	};
	const inlineLoadMessages = inlineSections.flatMap(({ category, status }) => {
		if (status === "loading") {
			return [optionsLoadingMessage(category.label)];
		}
		return status === "failed" ? [optionsLoadErrorMessage(category.label)] : [];
	});

	const valueSuggestions =
		!typeaheadActive || typedInlinePrefix !== null
			? []
			: collectValueSuggestions(
					inputValue,
					submenuCategories,
					typeaheadOptionsByKey,
					chipValues,
				);

	// A failed category query for the current input ends loading even while
	// other category queries are still fetching, so the live region announces
	// the failure instead of "Loading suggestions".
	const typeaheadError =
		typeaheadQuerySource.length > 0 &&
		!typeaheadQueryPending &&
		suggestionOptions.isError;
	// Includes the debounce window so the live region does not announce an
	// empty result before the request starts.
	const valueSuggestionsLoading =
		typeaheadQuerySource.length > 0 &&
		!typeaheadError &&
		(typeaheadQueryPending || suggestionOptions.isFetching);

	const hasTypeaheadQuery = typeaheadActive && inputValue.trim().length > 0;
	const typeaheadLoading =
		hasTypeaheadQuery &&
		valueSuggestionsLoading &&
		valueSuggestions.length === 0;

	const typeaheadEmpty =
		hasTypeaheadQuery &&
		!typeaheadLoading &&
		!typeaheadError &&
		listedCategories.length === 0 &&
		inlineSections.length === 0 &&
		valueSuggestions.length === 0;

	const activeOptionsEmpty =
		activeCategoryKey !== null &&
		!activeOptionsLoading &&
		!activeOptionsError &&
		activeOptions !== undefined &&
		activeOptions.length === 0;
	const activeOptionsSearched = activeOptionsQuerySource.length > 0;

	const statusMessage = deriveStatusMessage({
		activeCategoryLabel: activeCategory?.label,
		activeOptionsLoading,
		activeOptionsError,
		activeOptionsEmpty,
		activeOptionsSearched,
		typeaheadLoading,
		typeaheadError,
		inlineLoadMessages,
		typeaheadEmpty,
	});

	const retryTypeahead = () => {
		suggestionOptions.refetch();
	};

	const updateFromChips = (tokens: string[], freeText: string) => {
		dispatch({ type: "setTypedFreeText", value: freeText });
		emitQuery(composeFilterQuery(tokens, chipKeys, freeText));
	};

	// Adding an option from an exclusive category replaces that category's
	// other applied options. Queries that already combine several of them, such
	// as a bookmarked URL, are left alone until one is picked.
	const withInlineOption = (token: string) => {
		const categoryFor = (chip: string) => {
			const key = parseChipToken(chip, chipKeys)?.key;
			return key === undefined
				? undefined
				: categories.find((category) =>
						(category.chipKeys ?? [category.key]).includes(key),
					);
		};
		const category = categoryFor(token);
		const kept = category?.inlineOptionsExclusive
			? chipValues.filter((chip) => categoryFor(chip) !== category)
			: chipValues;
		return [...kept, token];
	};

	const selectCategory = (categoryKey: string) => {
		const category = categories.find((entry) => entry.key === categoryKey);
		if (!category || category.inlineOptions) {
			return;
		}
		const freeText = browseAll ? typedFreeText : "";
		emitQuery(composeFilterQuery(chipValues, chipKeys, freeText));
		dispatch({
			type: "enterCategory",
			categoryKey,
			query: "",
			typedFreeText: freeText,
		});
	};

	const toggledChips = (
		token: string,
		add = () => [...chipValues, token],
	): string[] =>
		chipValues.includes(token)
			? chipValues.filter((chip) => chip !== token)
			: add();

	// The typed text only located the suggestion, so it is dropped either way.
	const toggleValueSuggestion = (token: string) => {
		updateFromChips(toggledChips(token), "");
		closeMenu();
	};

	// Returning to the category list highlights the row that was open, so the
	// keyboard position is never lost when the option rows unmount.
	const returnToCategories = () => {
		setHighlightedValue(activeCategoryKey ?? "");
		dispatch({ type: "leaveCategory" });
	};

	const commitCategoryOption = (token: string) => {
		updateFromChips([...chipValues, token], typedFreeText);
		returnToCategories();
	};

	const toggleCategoryOption = (token: string) => {
		updateFromChips(toggledChips(token), typedFreeText);
		returnToCategories();
	};

	// Typed filter text is dropped once an option is picked. Free-text search
	// text is kept: text typed ahead of a `status:` prefix, and all text in
	// browse-all mode.
	const applyInlineChips = (tokens: string[]) => {
		const freeText =
			browseAll || typedInlinePrefix !== null ? typedFreeText : "";
		updateFromChips(tokens, freeText);
		if (!browseAll) {
			dispatch({ type: "typeFreeText", value: freeText });
		}
	};

	const toggleInlineOption = (token: string) => {
		applyInlineChips(toggledChips(token, () => withInlineOption(token)));
	};

	// Enter or Tab on a highlighted inline option row or value suggestion. When
	// typed text located an applied option, keeps it and clears the text;
	// otherwise toggles it as a click does. Returns false for any other token.
	const completeHighlightedOption = (token: string) => {
		const isInlineRow = inlineOptionRows.some((row) => row.token === token);
		const isSuggestion = valueSuggestions.some(
			(suggestion) => suggestion.token === token,
		);
		if (!isInlineRow && !isSuggestion) {
			return false;
		}
		const keepApplied =
			typeaheadActive &&
			inputValue.trim().length > 0 &&
			chipValues.includes(token);
		if (isInlineRow) {
			if (keepApplied) {
				applyInlineChips(chipValues);
			} else {
				toggleInlineOption(token);
			}
		} else if (keepApplied) {
			updateFromChips(chipValues, "");
			closeMenu();
		} else {
			toggleValueSuggestion(token);
		}
		return true;
	};

	const focusAndLeaveCategory = () => {
		inputRef.current?.focus();
		returnToCategories();
	};

	// True when matchCategories matches text, text matches a loaded option, or
	// a category's getOptions returns a matching option within
	// TYPED_TEXT_LOOKUP_TIMEOUT_MS; a failed lookup counts as no match. Callers
	// hold such text back so results do not empty out mid-word.
	const couldBeFilterSearch = (text: string): Promise<boolean> => {
		if (text.length === 0) {
			return Promise.resolve(false);
		}
		const matchesLoadedOption = [
			...unfilteredOptions.optionsByKey.values(),
		].some((options) => filterOptionsByText(options, text).length > 0);
		if (matchCategories(text, categories).length > 0 || matchesLoadedOption) {
			return Promise.resolve(true);
		}
		const matches = categories.map(async (category) => {
			const options = await queryClient.fetchQuery(
				filterComboboxOptions(category.key, category.getOptions, text, true),
			);
			if (filterOptionsByText(options, text).length === 0) {
				throw new Error("No matching option");
			}
			return true;
		});
		return Promise.race([
			Promise.any(matches).catch(() => false),
			new Promise<boolean>((resolve) =>
				setTimeout(resolve, TYPED_TEXT_LOOKUP_TIMEOUT_MS, false),
			),
		]);
	};

	const applyTypedTextUnlessFilter = async (
		text: string,
		generation: number,
	) => {
		const couldBeFilter = await couldBeFilterSearch(text);
		if (generation !== typedTextLookupGenerationRef.current) {
			return;
		}
		const query = composeFilterQuery(
			queryToChips(lastEmittedRef.current, chipKeys),
			chipKeys,
			couldBeFilter ? "" : text,
		);
		if (query !== lastEmittedRef.current) {
			emitQuery(query);
		}
	};

	// Chip tokens in typed text become chips rather than search text that
	// would repeat them, whether the menu held one back while it was typed or
	// it was pasted (`owner:me template:docker`). Applied chips come from the
	// last sent query, which `value` can lag.
	const composeTypedQuery = (text: string) => {
		const freeText = extractFreeText(text, chipKeys);
		const query = composeFilterQuery(
			[
				...queryToChips(lastEmittedRef.current, chipKeys),
				...queryToChips(text, chipKeys),
			],
			chipKeys,
			freeText,
		);
		return { query, freeText };
	};
	// Commits text on the spot, such as text typed ahead of a `key:` prefix,
	// and returns the text left after its chip tokens.
	const commitTypedText = (text: string) => {
		const { query, freeText } = composeTypedQuery(text);
		emitQuery(query);
		return freeText;
	};

	// The input drops committed chip tokens, so a later pick or dismissal does
	// not send them again.
	const applyTypedSearch = () => {
		cancelTypedTextLookup();
		const { query, freeText } = composeTypedQuery(typedFreeText);
		if (query !== lastEmittedRef.current) {
			emitQuery(query);
		}
		if (queryToChips(typedFreeText, chipKeys).length > 0) {
			dispatch({ type: "typeFreeText", value: freeText });
		}
	};

	// Also applies typed text as a free-text search, or steps back out of an
	// open category.
	const showAllFilters = () => {
		inputRef.current?.focus();
		if (mode === "category") {
			returnToCategories();
			return;
		}
		applyTypedSearch();
		dispatch({ type: "showAllFilters" });
	};

	// From inside a category the toggle steps back to the category list rather
	// than closing, so an accidental click can be corrected without reopening the menu.
	// With typed text narrowing the menu, it shows all filters instead of closing.
	const toggleFilterMenu = () => {
		if (mode === "category") {
			focusAndLeaveCategory();
			return;
		}
		if (open) {
			if (!browseAll && inputValue.trim().length > 0) {
				showAllFilters();
				return;
			}
			closeMenu();
			return;
		}
		showAllFilters();
	};

	const handleInputFocus = () => {
		if (mode === "category") {
			return;
		}
		dispatch({ type: "openBrowsing" });
	};

	const handleInputValueChange = (nextValue: string) => {
		cancelTypedTextLookup();
		const typedCategory = parseTypedCategoryPrefix(
			nextValue,
			submenuCategories,
		);
		if (typedCategory) {
			dispatch({
				type: "enterCategory",
				categoryKey: typedCategory.categoryKey,
				query: typedCategory.query,
				typedFreeText: commitTypedText(typedCategory.freeText),
			});
			return;
		}

		if (mode === "category") {
			dispatch({ type: "typeInCategory", value: nextValue });
			return;
		}

		// Inline category prefixes are filter search, never free-text search, so
		// the prefix itself is withheld until an option is picked.
		const typedInline = parseTypedCategoryPrefix(nextValue, inlineCategories);
		if (typedInline) {
			dispatch({
				type: "setTypedFreeText",
				value: commitTypedText(typedInline.freeText),
			});
			dispatch({ type: "typeFilterSearch", value: nextValue });
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
			const settledFreeText = commitTypedText(settledText);
			const inputFreeText = inProgressIsPartialChip
				? [settledFreeText, inProgress].filter(Boolean).join(" ")
				: settledFreeText;
			dispatch({ type: "typeFreeText", value: inputFreeText });
			return;
		}

		dispatch({ type: "typeFreeText", value: nextValue });
		setHighlightedValue("");
		scheduleTypedTextLookup(
			nextValue.trim(),
			typedTextLookupGenerationRef.current,
		);
	};

	// Radix only originates close requests (escape / outside press); opens flow
	// from the caller, so a dismissal restores the free-text input and applies
	// it as a search.
	const handleDismiss = () => {
		if (mode !== "category") {
			applyTypedSearch();
		}
		closeMenu();
	};

	// Chip removal leaves the popup, the input, and a pending typed-text lookup
	// untouched.
	const handleRemoveChip = (token: string) => {
		emitQueryKeepingLookup(
			composeFilterQuery(
				chipValues.filter((entry) => entry !== token),
				chipKeys,
				extractFreeText(lastEmittedRef.current, chipKeys),
			),
		);
	};

	// Text typed in the main menu may be a free-text search, so no row is
	// highlighted until the user moves to one. Inside a category, in the
	// all-filters list, or after a typed `key:` prefix the text narrows filters, so
	// the first match stays highlighted.
	const typingFreeText = hasTypeaheadQuery && typedInlinePrefix === null;

	const handleInputKeyDown = (event: ReactKeyboardEvent<HTMLInputElement>) => {
		const isBackspaceOrDelete =
			event.key === "Backspace" || event.key === "Delete";

		if (event.key === "ArrowLeft" && mode === "category") {
			event.preventDefault();
			focusAndLeaveCategory();
			return;
		}

		if (isBackspaceOrDelete && inputValue === "" && mode === "category") {
			event.preventDefault();
			focusAndLeaveCategory();
			return;
		}

		if (
			isBackspaceOrDelete &&
			inputValue === "" &&
			mode !== "category" &&
			chipValues.length > 0
		) {
			event.preventDefault();
			updateFromChips(chipValues.slice(0, -1), "");
			return;
		}

		// Let cmdk commit a currently highlighted category option. If there is no
		// rendered option to select, Enter commits the typed value directly so
		// valid backend values do not have to appear in the suggestion list.
		if (
			event.key === "Enter" &&
			mode === "category" &&
			activeCategory &&
			inputValue.trim().length > 0
		) {
			const highlighted = getHighlightedValue();
			const candidate = chipToken(activeCategory.key, inputValue.trim());
			const hasHighlightedOption = activeOptions?.some(
				(option) => optionToken(activeCategory.key, option) === highlighted,
			);
			if (
				!activeOptionsLoading &&
				!hasHighlightedOption &&
				parseChipToken(candidate, chipKeys)
			) {
				event.preventDefault();
				commitCategoryOption(candidate);
				return;
			}
		}

		// Same for a typed inline prefix such as `status:starting`, whose value
		// may not be one of the suggested options.
		if (
			event.key === "Enter" &&
			mode === "browsing" &&
			typedInlinePrefix !== null &&
			typedInlinePrefix.query.trim().length > 0
		) {
			const { categoryKey } = typedInlinePrefix;
			const highlighted = getHighlightedValue();
			const hasHighlightedOption = inlineOptionRows.some(
				(row) => row.token === highlighted,
			);
			const candidate = chipToken(categoryKey, typedInlinePrefix.query.trim());
			if (!hasHighlightedOption && parseChipToken(candidate, chipKeys)) {
				event.preventDefault();
				applyInlineChips(
					chipValues.includes(candidate)
						? chipValues
						: withInlineOption(candidate),
				);
				return;
			}
		}

		if (mode !== "browsing") {
			return;
		}

		// Row values in the order MainPanel renders them, so the first is the
		// top row.
		const rowValues = [
			...listedCategories.map((category) => category.key),
			...inlineSections.flatMap((section) =>
				section.status === "ready"
					? section.rows.map((row) => row.token)
					: [section.loadRowValue],
			),
			...valueSuggestions.map((suggestion) => suggestion.token),
		];
		// A highlighted row can unmount without cmdk reporting a new highlight,
		// so only a value naming a rendered row counts.
		const highlightedValue = getHighlightedValue();
		const renderedHighlightedValue = rowValues.includes(highlightedValue)
			? highlightedValue
			: "";

		// With free-typed text and no row highlighted, Enter applies the text as
		// a free-text search and closes the menu.
		if (
			event.key === "Enter" &&
			typingFreeText &&
			renderedHighlightedValue === ""
		) {
			event.preventDefault();
			applyTypedSearch();
			closeMenu();
			return;
		}

		// Enter and Tab complete the highlighted row. While typing free text with
		// no row highlighted, Tab completes the top row; otherwise Tab moves
		// focus. ArrowRight only opens a highlighted category.
		const isComplete =
			event.key === "Enter" || (event.key === "Tab" && !event.shiftKey);
		if (!isComplete && event.key !== "ArrowRight") {
			return;
		}

		const highlighted =
			renderedHighlightedValue ||
			(event.key === "Tab" && typingFreeText ? (rowValues[0] ?? "") : "");
		const category = listedCategories.find(
			(entry) => entry.key === highlighted,
		);
		if (category) {
			event.preventDefault();
			selectCategory(category.key);
			return;
		}

		if (isComplete && completeHighlightedOption(highlighted)) {
			event.preventDefault();
		}
	};

	return {
		open,
		inputValue,
		typedFreeText,
		activeCategoryKey,
		activeCategory,
		activeOptions,
		activeOptionsLoading,
		activeOptionsError,
		activeOptionsEmptyText: activeOptionsEmpty
			? optionsEmptyText(activeOptionsSearched)
			: undefined,
		statusMessage,
		listedCategories,
		categoriesNarrowedByText: categoryQuery.length > 0,
		autoHighlight: !typingFreeText,
		unfilteredOptionsByKey: unfilteredOptions.optionsByKey,
		unfilteredOptionsErroredKeys: unfilteredOptions.erroredKeys,
		valueSuggestions,
		inlineSections,
		chipValues,
		highlightRef,
		typeaheadError,
		actions: {
			setInputRef: (node: HTMLInputElement | null) => {
				inputRef.current = node;
			},
			focusInput: () => inputRef.current?.focus(),
			toggleMenu: toggleFilterMenu,
			showAllFilters,
			dismiss: handleDismiss,
			removeChip: handleRemoveChip,
			retryActiveOptions,
			retryTypeahead,
			retryUnfilteredOptions: unfilteredOptions.refetch,
			selectCategory,
			toggleCategoryOption,
			toggleInlineOption,
			toggleValueSuggestion,
			onInputFocus: handleInputFocus,
			onInputKeyDown: handleInputKeyDown,
			onInputValueChange: handleInputValueChange,
			setHighlightedValue,
			onHighlightedValueChange: handleHighlightedValueChange,
		},
	};
};
