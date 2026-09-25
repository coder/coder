import {
	type KeyboardEvent as ReactKeyboardEvent,
	useCallback,
	useEffect,
	useMemo,
	useReducer,
	useRef,
	useState,
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
	 * chip token still being typed. While it could still be a filter, the
	 * typed-text lookup withholds it from the emitted query until
	 * `applyTypedSearch` runs; when it ends with a chip token still being typed,
	 * the text before that token has already been emitted, and the token waits
	 * until it is committed as a chip.
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
	| { type: "clear" }
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
	typeaheadLoading: boolean;
	typeaheadError: boolean;
	typeaheadEmpty: boolean;
};

/**
 * Longest wait for option lookups before typed text that matched no loaded
 * filter is applied as a free-text search anyway.
 */
export const TYPED_TEXT_LOOKUP_TIMEOUT_MS = 1000;

/** Shown and announced when the typeahead suggestion queries fail. */
export const SUGGESTIONS_ERROR_MESSAGE = "Couldn’t load suggestions.";

// Live-region text for each state so screen readers hear loading, failures,
// and empty results rather than silence. Typeahead loading shows no spinner,
// so this is its only announcement.
const deriveStatusMessage = ({
	activeCategoryLabel,
	activeOptionsLoading,
	activeOptionsError,
	activeOptionsEmpty,
	categoryListLoading,
	typeaheadLoading,
	typeaheadError,
	typeaheadEmpty,
}: StatusMessageInput): string => {
	if (activeCategoryLabel !== undefined) {
		if (activeOptionsLoading) {
			return `Loading ${activeCategoryLabel} options`;
		}
		if (activeOptionsError) {
			return `Couldn't load ${activeCategoryLabel} options`;
		}
		if (activeOptionsEmpty) {
			return `No ${activeCategoryLabel} matches`;
		}
		return `Filtering by ${activeCategoryLabel}`;
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
	// A category's first applied chip decides its scope toggle; with no chip
	// the toggle is on. A typed `owner:` or `user:` prefix decides it instead
	// while its category is open, and reaches the query only with the option
	// picked.
	const [typedScopeWidened, setTypedScopeWidened] = useState<boolean | null>(
		null,
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
				const { key, hideWhenSingleOption } = categories[index];
				if (result.data) {
					optionsByKey.set(key, result.data);
				} else if (result.isError) {
					erroredKeys.add(key);
				}
				// A retry puts a failed query back to pending, so
				// `errorUpdateCount` tells a retry from the first load.
				if (!result.data && result.errorUpdateCount > 0) {
					failedOrRetryingKeys.add(key);
				} else if (hideWhenSingleOption && result.isPending) {
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
	// A hideable category stays in the menu while its options first load, so
	// typed text can match it; while it has an applied chip, so the chip can be
	// changed. After a failed lookup, including while its retry runs, it stays
	// so its flyout can offer a retry, until a retry returns at most one option.
	const isInMenu = (category: FilterCategory, chips = chipValues) =>
		!category.hideWhenSingleOption ||
		isHideableFirstLoad(category) ||
		unfilteredOptions.failedOrRetryingKeys.has(category.key) ||
		(unfilteredOptions.optionsByKey.get(category.key)?.length ?? 0) > 1 ||
		chips.some((token) => categoryForChip(token) === category);
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

	const activeOptionsQuerySource = activeCategoryKey !== null ? inputValue : "";
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

	// Until a category's results for the typed text arrive, its unfiltered
	// options are filtered locally so matching rows stay visible. A category
	// whose query failed shows no rows; the retry row replaces them.
	const typeaheadOptionsByKey = new Map<string, readonly FilterOption[]>(
		typeaheadQuerySource.length === 0 || typeaheadQueryPending
			? []
			: suggestionOptions.optionsByKey,
	);
	if (typeaheadQuerySource.length > 0) {
		for (const category of optionLookupCategories) {
			const options = unfilteredOptions.optionsByKey.get(category.key);
			const settled =
				!typeaheadQueryPending &&
				(typeaheadOptionsByKey.has(category.key) ||
					suggestionOptions.erroredKeys.has(category.key));
			if (options && !settled) {
				typeaheadOptionsByKey.set(
					category.key,
					filterOptionsByText(options, typeaheadQuerySource),
				);
			}
		}
	}
	const inlineOptionsSource =
		typeaheadQuerySource.length === 0
			? unfilteredOptions.optionsByKey
			: typeaheadOptionsByKey;
	const inlineOptionRowsFor = (
		optionsByCategory: ReadonlyMap<string, readonly FilterOption[]>,
	) =>
		categories.flatMap((category) => {
			if (!category.inlineOptions) {
				return [];
			}
			return (optionsByCategory.get(category.key) ?? []).map((option) => {
				const token = optionToken(category.key, option);
				return {
					categoryKey: category.key,
					categoryLabel: category.inlineOptionsLabel ?? `${category.label} is…`,
					token,
					selected: chipValues.includes(token),
					showIcon: category.inlineOptionsIcons ?? false,
					option,
				};
			});
		});
	const inlineOptionRows = open
		? inlineOptionRowsFor(inlineOptionsSource).filter(
				(row) =>
					typedInlinePrefix === null ||
					row.categoryKey === typedInlinePrefix.categoryKey,
			)
		: [];

	const valueSuggestions =
		!typeaheadActive || typedInlinePrefix !== null
			? []
			: collectValueSuggestions(
					inputValue,
					menuCategories.map((category) => ({
						...category,
						chipKey: optionChipKey(category),
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
		inlineOptionRows.length === 0 &&
		valueSuggestions.length === 0;

	const activeOptionsEmpty =
		activeCategoryKey !== null &&
		!activeOptionsLoading &&
		!activeOptionsError &&
		activeOptions !== undefined &&
		activeOptions.length === 0;

	const statusMessage = deriveStatusMessage({
		activeCategoryLabel: activeCategory?.label,
		activeOptionsLoading,
		activeOptionsError,
		activeOptionsEmpty,
		categoryListLoading: placeholdersShown,
		typeaheadLoading,
		typeaheadError,
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
		setTypedScopeWidened(null);
		emitQuery(composeFilterQuery(chipValues, chipKeys, freeText));
		dispatch({
			type: "enterCategory",
			categoryKey,
			query: "",
			typedFreeText: freeText,
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
		setTypedScopeWidened(null);
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
		dispatch({ type: "close" });
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
			dispatch({ type: "close" });
		} else {
			toggleValueSuggestion(token);
		}
		return true;
	};

	const focusAndLeaveCategory = () => {
		inputRef.current?.focus();
		returnToCategories();
	};

	// True when text matches the name of any category, including one left out
	// of the menu, or a loaded option or an option getOptions returns within
	// TYPED_TEXT_LOOKUP_TIMEOUT_MS of a category in the menu or an inline
	// category. Failed and pending lookups count as no match. Callers hold such
	// text back from the search so results do not empty out mid-word.
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
			dispatch({ type: "close" });
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
			setTypedScopeWidened(
				scopeToggle ? typedCategory.typedKey === scopeToggle.widenedKey : null,
			);
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
		dispatch({ type: "close" });
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
			const candidate = chipToken(
				optionChipKey(activeCategory),
				inputValue.trim(),
			);
			const hasHighlightedOption = activeOptions?.some(
				(option) =>
					optionToken(optionChipKey(activeCategory), option) === highlighted,
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
			...inlineOptionRows.map((row) => row.token),
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
			dispatch({ type: "close" });
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
		activeOptionsError,
		statusMessage,
		listedCategories,
		categoriesNarrowedByText: categoryQuery.length > 0,
		// Category whose scopeToggle.searchPhrase starts with the typed text.
		scopeMatchKey: scopeMatchedCategory?.key ?? null,
		categoryPlaceholderCount,
		autoHighlight: !typingFreeText && !placeholdersShown,
		unfilteredOptionsByKey: unfilteredOptions.optionsByKey,
		unfilteredOptionsErroredKeys: unfilteredOptions.erroredKeys,
		valueSuggestions,
		inlineOptionRows,
		chipValues,
		highlightRef,
		// Switch state for a scope category, undefined for other categories.
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
		optionChipKey: (categoryKey: string) => {
			const category = categories.find((entry) => entry.key === categoryKey);
			return category ? optionChipKey(category) : categoryKey;
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
		},
	};
};
