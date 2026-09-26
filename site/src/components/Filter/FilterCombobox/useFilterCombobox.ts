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
import { categoryChipKeys } from "./types";

// Shorter prefixes would list the category for ordinary searches such as `sh`.
const SCOPE_MATCH_MIN_QUERY_LENGTH = 3;

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
	/**
	 * Scope set by a typed prefix for the open scope-toggle category:
	 * `widenedKey` sets true, and the category key or an alias sets false. Null
	 * when no prefix was typed, so the applied chip decides the scope instead.
	 */
	typedScopeWidened: boolean | null;
};

type Action =
	| { type: "openBrowsing" }
	| { type: "showAllFilters" }
	| {
			type: "enterCategory";
			categoryKey: string;
			query: string;
			typedFreeText: string;
			typedScopeWidened: boolean | null;
	  }
	| { type: "resetTypedScope" }
	| { type: "typeInCategory"; value: string }
	| { type: "typeFilterSearch"; value: string }
	| { type: "typeFreeText"; value: string }
	| { type: "setTypedFreeText"; value: string }
	| { type: "leaveCategory" }
	| { type: "close" }
	| { type: "clear" }
	| { type: "reconcile"; freeText: string };

const closeState = (state: State): State => ({
	mode: "closed",
	browseAll: false,
	activeCategoryKey: null,
	typedFreeText: state.typedFreeText,
	inputValue: state.typedFreeText,
	typedScopeWidened: null,
});

const reducer = (state: State, action: Action): State => {
	switch (action.type) {
		case "openBrowsing":
			return {
				...state,
				mode: "browsing",
				browseAll: false,
				activeCategoryKey: null,
				typedScopeWidened: null,
			};
		case "showAllFilters":
			return {
				...state,
				mode: "browsing",
				browseAll: true,
				activeCategoryKey: null,
				typedScopeWidened: null,
			};
		case "enterCategory":
			return {
				mode: "category",
				browseAll: false,
				activeCategoryKey: action.categoryKey,
				inputValue: action.query,
				typedFreeText: action.typedFreeText.trim(),
				typedScopeWidened: action.typedScopeWidened,
			};
		case "resetTypedScope":
			return { ...state, typedScopeWidened: null };
		case "typeInCategory":
			return { ...state, mode: "category", inputValue: action.value };
		case "typeFilterSearch":
			return {
				...state,
				mode: "browsing",
				browseAll: false,
				activeCategoryKey: null,
				inputValue: action.value,
				typedScopeWidened: null,
			};
		case "typeFreeText":
			return {
				mode: "browsing",
				browseAll: false,
				activeCategoryKey: null,
				inputValue: action.value,
				typedFreeText: action.value.trim(),
				typedScopeWidened: null,
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
				typedScopeWidened: null,
			};
		case "close":
			return closeState(state);
		case "clear":
			return { ...state, inputValue: "", typedFreeText: "" };
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
	categoryListLoading: boolean;
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
	categoryListLoading,
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
	if (categoryListLoading) {
		return "Loading filters";
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
	categories: categoriesProp,
}: UseFilterComboboxOptions) => {
	// Typed text matches a scope category's widened key too, such as `user`.
	const categories = useMemo(
		() =>
			categoriesProp.map((category) =>
				category.scopeToggle
					? {
							...category,
							aliases: [
								...(category.aliases ?? []),
								category.scopeToggle.widenedKey,
							],
						}
					: category,
			),
		[categoriesProp],
	);
	const chipKeys = useMemo(
		() => categories.flatMap(categoryChipKeys),
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
			typedScopeWidened: null,
		};
	});
	const {
		mode,
		browseAll,
		activeCategoryKey,
		inputValue,
		typedFreeText,
		typedScopeWidened,
	} = state;
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
	// A row the query drops from the menu while it is closed must not stay
	// highlighted.
	const emitQueryKeepingLookup = (query: string) => {
		highlightCategoryListRow(
			queryToChips(query, chipKeys),
			getHighlightedValue(),
		);
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
	const chipKeyOf = (token: string) => parseChipToken(token, chipKeys)?.key;
	const scopeChipsOf = (category: FilterCategory) =>
		category.scopeToggle
			? chipValues.filter((token) => {
					const key = chipKeyOf(token);
					return key !== undefined && categoryChipKeys(category).includes(key);
				})
			: [];
	// The switch and pill act on a scope category's only chip. With a chip
	// under each key, such as a bookmarked `user:me owner:carol`, rewriting one
	// would collide with the other, and the query would silently lose a filter.
	const isScopeToggleDisabled = (category: FilterCategory) =>
		scopeChipsOf(category).length !== 1;
	// A category's first applied chip decides its scope toggle; with no chip
	// the toggle is on. While its category is open, a typed prefix decides it
	// instead. The typed key reaches the query only with the option picked.
	const isScopeWidened = (category: FilterCategory) => {
		const toggle = category.scopeToggle;
		if (!toggle) {
			return false;
		}
		if (activeCategoryKey === category.key && typedScopeWidened !== null) {
			return typedScopeWidened;
		}
		const [scopeChip] = scopeChipsOf(category);
		return (
			scopeChip === undefined || chipKeyOf(scopeChip) === toggle.widenedKey
		);
	};
	// Follows the applied chip, not a typed `owner:` or `user:` prefix.
	const scopePillCategoryKey = (token: string) => {
		const category = categoryForChip(token);
		return category?.scopeToggle &&
			!isScopeToggleDisabled(category) &&
			chipKeyOf(token) === category.scopeToggle.widenedKey
			? category.key
			: undefined;
	};
	const optionChipKey = (category: FilterCategory) =>
		category.scopeToggle && isScopeWidened(category)
			? category.scopeToggle.widenedKey
			: category.key;
	// The applied chip holding a value, ignoring letter case, unless a typed
	// prefix sets the key. An option maps to it, so its row shows as applied
	// and choosing it removes it; typed Enter with no option highlighted
	// commits it, which keeps it.
	const scopeChipHolding = (category: FilterCategory, value: string) => {
		if (activeCategoryKey === category.key && typedScopeWidened !== null) {
			return undefined;
		}
		const folded = value.toLowerCase();
		return scopeChipsOf(category).find(
			(chip) => parseChipToken(chip, chipKeys)?.value.toLowerCase() === folded,
		);
	};
	const optionTokenFor = (
		category: FilterCategory,
		option: Pick<FilterOption, "token" | "value">,
	) =>
		scopeChipHolding(category, option.value) ??
		optionToken(optionChipKey(category), option);
	const categoryForChip = (token: string) => {
		const key = parseChipToken(token, chipKeys)?.key;
		return key === undefined
			? undefined
			: categories.find((category) => categoryChipKeys(category).includes(key));
	};

	// Categories with a flyout or drill-in list, including those
	// hideWhenSingleOption leaves out of the menu. Inline categories render their
	// options directly in the main panel and never enter category mode.
	const allSubmenuCategories = categories.filter(
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

	// Empty-query options load while browsing the category list. Inline
	// categories, whose chips take their labels from them, and categories with
	// `hideWhenSingleOption` load when the filter renders. Shares its cache key
	// with the category view.
	const unfilteredOptionsEnabled = isBrowsing && activeCategoryKey === null;
	const unfilteredOptions = useQueries({
		queries: categories.map((category) =>
			filterComboboxOptions(
				category.key,
				category.getOptions,
				"",
				unfilteredOptionsEnabled ||
					Boolean(category.inlineOptions || category.hideWhenSingleOption),
			),
		),
		combine: (results) => {
			const optionsByKey = new Map<string, readonly FilterOption[]>();
			const erroredKeys = new Set<string>();
			const failedOrRetryingKeys = new Set<string>();
			const hideableFirstLoadKeys = new Set<string>();
			results.forEach((result, index) => {
				const { key, hideWhenSingleOption, inlineOptions } = categories[index];
				if (result.data) {
					optionsByKey.set(key, result.data);
				} else if (result.isError) {
					erroredKeys.add(key);
				}
				// A retry puts a failed query back to pending, so
				// `errorUpdateCount` tells a retry from the first load.
				if (!result.data && result.errorUpdateCount > 0) {
					failedOrRetryingKeys.add(key);
				} else if (hideWhenSingleOption && !inlineOptions && result.isPending) {
					hideableFirstLoadKeys.add(key);
				}
			});
			return {
				optionsByKey,
				erroredKeys,
				failedOrRetryingKeys,
				hideableFirstLoadKeys,
				refetch: (categoryKey: string) => {
					const index = categories.findIndex(
						(category) => category.key === categoryKey,
					);
					void results[index]?.refetch();
				},
			};
		},
	});
	const isHideableFirstLoad = (category: FilterCategory) =>
		unfilteredOptions.hideableFirstLoadKeys.has(category.key);
	const hasSeveralOptionsOrChip = (
		category: FilterCategory,
		options: readonly FilterOption[] | undefined,
		chips: readonly string[],
	) =>
		(options?.length ?? 0) > 1 ||
		chips.some((token) => categoryForChip(token) === category);
	// A hideable category stays in the menu while its options first load, so
	// typed text can match it; while it has an applied chip, so the chip can be
	// changed. After a failed lookup, including while its retry runs, it stays
	// so its flyout can offer a retry, until a retry returns at most one option.
	const isInMenu = (category: FilterCategory, chips = chipValues) =>
		!category.hideWhenSingleOption ||
		isHideableFirstLoad(category) ||
		unfilteredOptions.failedOrRetryingKeys.has(category.key) ||
		hasSeveralOptionsOrChip(
			category,
			unfilteredOptions.optionsByKey.get(category.key),
			chips,
		);
	const menuCategories = allSubmenuCategories.filter((category) =>
		isInMenu(category),
	);
	const hideableFirstLoadPending =
		allSubmenuCategories.some(isHideableFirstLoad);

	const categoryQuery =
		activeCategoryKey !== null || browseAll ? "" : inputValue.trim();
	// Until every hideable category finishes its first load, successfully or
	// not, the full list is unknown, so the unnarrowed category list shows one
	// placeholder row per submenu category. The list that replaces them can be
	// shorter.
	const categoryPlaceholderCount =
		open &&
		activeCategoryKey === null &&
		typedInlinePrefix === null &&
		categoryQuery.length === 0 &&
		hideableFirstLoadPending
			? allSubmenuCategories.length
			: 0;
	// Placeholder rows are not cmdk items, so automatic highlight is off while
	// they show; otherwise cmdk would highlight the first inline row and Enter
	// would apply it.
	const placeholdersShown = categoryPlaceholderCount > 0;
	const findScopeMatch = (query: string) => {
		const scopeQuery = query.trim().toLowerCase();
		if (scopeQuery.length < SCOPE_MATCH_MIN_QUERY_LENGTH) {
			return undefined;
		}
		return menuCategories.find((category) => {
			return category.scopeToggle?.searchPhrase
				.toLowerCase()
				.startsWith(scopeQuery);
		});
	};
	const scopeMatchedCategory =
		typedInlinePrefix !== null ? undefined : findScopeMatch(categoryQuery);
	const matchedCategories = matchCategories(categoryQuery, menuCategories);
	const listedOnlyByScopeMatch =
		scopeMatchedCategory !== undefined &&
		!matchedCategories.includes(scopeMatchedCategory);
	const listedCategories =
		!open || typedInlinePrefix !== null || placeholdersShown
			? []
			: categoryQuery.length === 0
				? menuCategories
				: listedOnlyByScopeMatch
					? [...matchedCategories, scopeMatchedCategory]
					: matchedCategories;

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

	const optionLookupCategories = [...menuCategories, ...inlineCategories];
	const suggestionOptions = useQueries({
		queries: optionLookupCategories.map((category) =>
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
					optionsByKey.set(optionLookupCategories[index].key, result.data);
				} else if (result.isError) {
					erroredKeys.add(optionLookupCategories[index].key);
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
					menuCategories.map((category) => ({
						...category,
						optionTokenFor: (option: FilterOption) =>
							optionTokenFor(category, option),
					})),
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
		categoryListLoading: placeholdersShown,
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
		const category = categoryForChip(token);
		const kept = category?.inlineOptionsExclusive
			? chipValues.filter((chip) => categoryForChip(chip) !== category)
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
			typedScopeWidened: null,
		});
	};

	// A category search is dropped after a filter pick or an explicit switch
	// toggle; text already applied as a search stays.
	const hasCategorySearchText =
		mode === "browsing" && !browseAll && inputValue.trim().length > 0;
	const appliedFreeText = () =>
		extractFreeText(lastEmittedRef.current, chipKeys);

	const toggleScope = (
		categoryKey: string,
		{ clearCategorySearch = false }: { clearCategorySearch?: boolean } = {},
	) => {
		const category = categories.find((entry) => entry.key === categoryKey);
		const toggle = category?.scopeToggle;
		if (!category || !toggle || isScopeToggleDisabled(category)) {
			return;
		}
		const widened = isScopeWidened(category);
		dispatch({ type: "resetTypedScope" });
		rewriteScopeKey(
			widened ? toggle.widenedKey : category.key,
			widened ? category.key : toggle.widenedKey,
			{ clearCategorySearch },
		);
	};

	// Unlike the switch, which a typed `owner:` or `user:` prefix may override,
	// the pill always narrows the applied chip.
	const removeScopePill = (categoryKey: string) => {
		const category = categories.find((entry) => entry.key === categoryKey);
		const toggle = category?.scopeToggle;
		if (!category || !toggle || isScopeToggleDisabled(category)) {
			return;
		}
		rewriteScopeKey(toggle.widenedKey, category.key);
	};

	const rewriteScopeKey = (
		fromKey: string,
		toKey: string,
		{ clearCategorySearch = false }: { clearCategorySearch?: boolean } = {},
	) => {
		const rewritten = chipValues.map((token) => {
			const parsed = parseChipToken(token, chipKeys);
			return parsed?.key === fromKey ? chipToken(toKey, parsed.value) : token;
		});
		if (clearCategorySearch && hasCategorySearchText) {
			const freeText = appliedFreeText();
			updateFromChips(rewritten, freeText);
			dispatch({ type: "typeFreeText", value: freeText });
		} else if (rewritten.some((token, index) => token !== chipValues[index])) {
			emitQueryKeepingLookup(
				composeFilterQuery(rewritten, chipKeys, appliedFreeText()),
			);
		}
	};

	// A pick in a scope category replaces the chip under the picked key, or the
	// category's first chip when none uses that key, such as after a typed
	// `owner:` or `user:` prefix.
	const withCategoryOption = (token: string) => {
		const category = categoryForChip(token);
		const scopeChips = category ? scopeChipsOf(category) : [];
		const replaced =
			scopeChips.find((chip) => chipKeyOf(chip) === chipKeyOf(token)) ??
			scopeChips[0];
		return replaced === undefined
			? [...chipValues, token]
			: chipValues.map((chip) => (chip === replaced ? token : chip));
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

	// Highlights the category row `rowKey` in the menu that `nextChips`
	// produce. When that row is not in it, because the picked option hid it, a
	// removed chip no longer lists it, or the category was entered by typing
	// its hidden key, the first row in the menu is highlighted instead; cmdk
	// keeps a highlight that names no row. Leaves the highlight unchanged when
	// `rowKey` names no category row.
	const highlightCategoryListRow = (
		nextChips: string[],
		rowKey = activeCategoryKey,
	) => {
		const row = allSubmenuCategories.find(
			(category) => category.key === rowKey,
		);
		if (row === undefined) {
			return;
		}
		const staysInMenu = (category: FilterCategory) =>
			isInMenu(category, nextChips);
		const nextHighlight = staysInMenu(row)
			? row.key
			: allSubmenuCategories.find(staysInMenu)?.key;
		setHighlightedValue(nextHighlight ?? "");
	};
	// Highlights the row that was open, so the keyboard position is never lost
	// when the option rows unmount.
	const returnToCategories = (nextChips = chipValues) => {
		highlightCategoryListRow(nextChips);
		dispatch({ type: "leaveCategory" });
	};

	const applyCategoryChips = (nextChips: string[]) => {
		updateFromChips(
			nextChips,
			hasCategorySearchText ? appliedFreeText() : typedFreeText,
		);
		returnToCategories(nextChips);
	};

	const commitCategoryOption = (token: string) => {
		applyCategoryChips(withCategoryOption(token));
	};

	const toggleCategoryOption = (token: string) => {
		applyCategoryChips(toggledChips(token, () => withCategoryOption(token)));
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

	// A hideable submenu category's empty-query options, which decide whether it
	// stays in the menu. Awaits a pending first load or Retry. A failed load
	// that is not retrying keeps the category listed; it returns undefined here
	// because `ensureQueryData` would refetch it.
	const unfilteredForMenuCheck = (
		category: FilterCategory,
	): Promise<readonly FilterOption[] | undefined> | undefined => {
		if (!category.hideWhenSingleOption || category.inlineOptions) {
			return undefined;
		}
		const options = filterComboboxOptions(
			category.key,
			category.getOptions,
			"",
			true,
		);
		const state = queryClient.getQueryState(options.queryKey);
		if (state?.status === "error" && state.data === undefined) {
			return undefined;
		}
		return queryClient.ensureQueryData(options).catch(() => undefined);
	};

	// True when text matches the name of any category, including one left out
	// of the menu, or a loaded option or an option getOptions returns within
	// TYPED_TEXT_LOOKUP_TIMEOUT_MS of an inline category or a category that is
	// still in the menu once the lookup settles, judged by its settled
	// empty-query options and the last sent chips. Failed and pending lookups
	// count as no match. Callers hold such text back from the search so results
	// do not empty out mid-word.
	const couldBeFilterSearch = (text: string): Promise<boolean> => {
		if (text.length === 0) {
			return Promise.resolve(false);
		}
		const matchesLoadedOption = optionLookupCategories.some(
			(category) =>
				filterOptionsByText(
					unfilteredOptions.optionsByKey.get(category.key) ?? [],
					text,
				).length > 0,
		);
		const matchesCategory =
			matchCategories(text, categories).length > 0 ||
			findScopeMatch(text) !== undefined;
		if (matchesCategory || matchesLoadedOption) {
			return Promise.resolve(true);
		}
		const matches = optionLookupCategories.map(async (category) => {
			const [unfiltered, filtered] = await Promise.all([
				unfilteredForMenuCheck(category),
				queryClient.fetchQuery(
					filterComboboxOptions(category.key, category.getOptions, text, true),
				),
			]);
			// Chips may change while the lookup runs, so read the last sent query.
			if (
				unfiltered &&
				!hasSeveralOptionsOrChip(
					category,
					unfiltered,
					queryToChips(lastEmittedRef.current, chipKeys),
				)
			) {
				throw new Error("Category leaves the menu");
			}
			if (filterOptionsByText(filtered, text).length === 0) {
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
		// A `value` change from the caller can drop the highlighted row.
		// `handleInputFocus` repairs it, but `focus()` fires no focus event when
		// the input already has focus.
		highlightCategoryListRow(chipValues, getHighlightedValue());
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
		// A `value` change from the caller can drop the highlighted row.
		highlightCategoryListRow(chipValues, getHighlightedValue());
		dispatch({ type: "openBrowsing" });
	};

	const handleInputValueChange = (nextValue: string) => {
		cancelTypedTextLookup();
		const typedCategory = parseTypedCategoryPrefix(
			nextValue,
			allSubmenuCategories,
		);
		if (typedCategory) {
			const scopeToggle = allSubmenuCategories.find(
				(entry) => entry.key === typedCategory.categoryKey,
			)?.scopeToggle;
			dispatch({
				type: "enterCategory",
				categoryKey: typedCategory.categoryKey,
				query: typedCategory.query,
				typedFreeText: commitTypedText(typedCategory.freeText),
				typedScopeWidened: scopeToggle
					? typedCategory.typedKey === scopeToggle.widenedKey
					: null,
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

	// Empties the query, chips and search text alike. Unlike chip removal, it
	// cancels a pending typed-text lookup, and an open category returns to the
	// full list while the popup stays open.
	const clearAll = () => {
		if (activeCategoryKey !== null) {
			returnToCategories([]);
		}
		dispatch({ type: "clear" });
		emitQuery("");
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
			// A widened chip is followed by its scope pill, so the pill goes first.
			const pillCategoryKey = scopePillCategoryKey(
				chipValues[chipValues.length - 1],
			);
			if (pillCategoryKey) {
				removeScopePill(pillCategoryKey);
				return;
			}
			updateFromChips(chipValues.slice(0, -1), "");
			return;
		}

		// Let cmdk commit a currently highlighted category option. Otherwise
		// Enter commits the typed value: the applied chip holding it, a listed
		// option matching it ignoring letter case, or a new chip, so valid
		// backend values do not have to appear in the suggestion list.
		if (
			event.key === "Enter" &&
			mode === "category" &&
			activeCategory &&
			inputValue.trim().length > 0
		) {
			const highlighted = getHighlightedValue();
			const typedOption = { value: inputValue.trim() };
			const listedOption = activeOptions?.find(
				(option) =>
					option.value.toLowerCase() === typedOption.value.toLowerCase(),
			);
			// See `scopeToggle`: unlisted values avoid the widened key, which a
			// backend can reject (#29961 for Workspaces `user:`).
			const candidate =
				scopeChipHolding(activeCategory, typedOption.value) ??
				(listedOption
					? optionToken(optionChipKey(activeCategory), listedOption)
					: chipToken(
							typedScopeWidened === true
								? optionChipKey(activeCategory)
								: activeCategory.key,
							typedOption.value,
						));
			const hasHighlightedOption = activeOptions?.some(
				(option) => optionTokenFor(activeCategory, option) === highlighted,
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
		// Completing a row listed only because the text matched its scope
		// phrase applies the text as a search instead of entering the category.
		if (
			isComplete &&
			listedOnlyByScopeMatch &&
			highlighted === scopeMatchedCategory?.key
		) {
			applyTypedSearch();
			dispatch({ type: "showAllFilters" });
			event.preventDefault();
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
		menuCategories,
		listedCategories,
		categoriesNarrowedByText: categoryQuery.length > 0,
		// Category whose scopeToggle.searchPhrase starts with the typed text.
		scopeMatchKey: scopeMatchedCategory?.key ?? null,
		categoryPlaceholderCount,
		autoHighlight: !typingFreeText && !placeholdersShown,
		unfilteredOptionsByKey: unfilteredOptions.optionsByKey,
		unfilteredOptionsErroredKeys: unfilteredOptions.erroredKeys,
		valueSuggestions,
		inlineSections,
		chipValues,
		highlightRef,
		scopeState: (categoryKey: string) => {
			const category = categories.find((entry) => entry.key === categoryKey);
			const toggle = category?.scopeToggle;
			if (!category || !toggle) {
				return undefined;
			}
			const [scopeChip] = scopeChipsOf(category);
			return {
				widened: isScopeWidened(category),
				label: toggle.label(
					scopeChip === undefined
						? undefined
						: parseChipToken(scopeChip, chipKeys)?.value,
				),
				disabled: isScopeToggleDisabled(category),
			};
		},
		// Key of the category whose scope pill follows this chip.
		scopePillCategoryKey,
		// While a scope category has a chip under each key, its chips show their
		// own query keys instead of the category key.
		showsOwnQueryKey: (token: string) => {
			const category = categoryForChip(token);
			return category !== undefined && scopeChipsOf(category).length > 1;
		},
		optionTokenFor: (categoryKey: string, option: FilterOption) => {
			const category = categories.find((entry) => entry.key === categoryKey);
			return category
				? optionTokenFor(category, option)
				: optionToken(categoryKey, option);
		},
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
			clearAll,
			retryActiveOptions,
			retryTypeahead,
			retryUnfilteredOptions: unfilteredOptions.refetch,
			selectCategory,
			toggleCategoryOption,
			toggleInlineOption,
			toggleScope,
			removeScopePill,
			toggleValueSuggestion,
			onInputFocus: handleInputFocus,
			onInputKeyDown: handleInputKeyDown,
			onInputValueChange: handleInputValueChange,
			setHighlightedValue,
			onHighlightedValueChange: handleHighlightedValueChange,
		},
	};
};
