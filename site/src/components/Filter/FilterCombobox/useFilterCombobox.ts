import {
	type KeyboardEvent as ReactKeyboardEvent,
	useEffect,
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
	dedupeChips,
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
	 * Free text typed outside chips and category prefixes. While it could still
	 * be a filter being searched for, it is withheld from the emitted query
	 * until `applyTypedSearch` runs.
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
	| { type: "close"; input: "restore" | "clear" }
	| { type: "reconcile"; freeText: string };

const closeState = (state: State, input: "restore" | "clear"): State => ({
	mode: "closed",
	browseAll: false,
	activeCategoryKey: null,
	typedFreeText: state.typedFreeText,
	inputValue: input === "restore" ? state.typedFreeText : "",
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
			return closeState(state, action.input);
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
	typeaheadLoading: boolean;
	typeaheadError: boolean;
	typeaheadEmpty: boolean;
};

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
 * Drives `FilterCombobox`: a `mode` state machine for the
 * popup, debounced query emission back to the caller, and the react-query
 * lookups for category options and cross-category suggestions.
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
	const inputRef = useRef<HTMLInputElement | null>(null);

	const queryClient = useQueryClient();
	// Bumped by every input change and emit, so a typed search whose filter
	// lookup resolves late cannot replace a newer query.
	const typedSearchGenerationRef = useRef(0);
	const { debounced: scheduleTypedSearch, cancelDebounce: cancelTypedSearch } =
		useDebouncedFunction(
			(text: string, generation: number) =>
				applyTypedTextUnlessFilter(text, generation),
			SEARCH_DEBOUNCE_MS,
		);
	const cancelPendingTypedSearch = () => {
		cancelTypedSearch();
		typedSearchGenerationRef.current += 1;
	};

	const emitQuery = (query: string) => {
		cancelPendingTypedSearch();
		lastEmittedRef.current = query;
		onChange(query);
	};
	const appliedFreeText = () =>
		extractFreeText(lastEmittedRef.current, chipKeys);

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
			cancelTypedSearch();
			typedSearchGenerationRef.current += 1;
			lastEmittedRef.current = value;
		}
		dispatch({ type: "reconcile", freeText: extractFreeText(value, chipKeys) });
	}, [value, chipKeys, cancelTypedSearch]);

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
				if (result.data) {
					optionsByKey.set(categories[index].key, result.data);
				} else if (result.isError) {
					erroredKeys.add(categories[index].key);
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

	// Until a category's results for the typed text arrive, its unfiltered
	// options are filtered locally so matching rows stay visible. A category
	// whose query failed shows no rows; the retry row replaces them.
	const typeaheadOptionsByKey = new Map<string, readonly FilterOption[]>(
		typeaheadQuerySource.length === 0 || typeaheadQueryPending
			? []
			: suggestionOptions.optionsByKey,
	);
	if (typeaheadQuerySource.length > 0) {
		for (const [key, options] of unfilteredOptions.optionsByKey) {
			const settled =
				!typeaheadQueryPending &&
				(typeaheadOptionsByKey.has(key) ||
					suggestionOptions.erroredKeys.has(key));
			if (!settled) {
				typeaheadOptionsByKey.set(
					key,
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
	const allInlineOptionRows = inlineOptionRowsFor(
		unfilteredOptions.optionsByKey,
	);

	const valueSuggestions =
		!typeaheadActive || typedInlinePrefix !== null
			? []
			: collectValueSuggestions(
					inputValue,
					submenuCategories,
					typeaheadOptionsByKey,
					chipValues,
				);

	// A failed suggestion query for the current input ends loading, so the live
	// region announces the failure instead of repeating "Loading suggestions".
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
		typeaheadLoading,
		typeaheadError,
		typeaheadEmpty,
	});

	const retryTypeahead = () => {
		suggestionOptions.refetch();
	};

	const updateFromChips = (tokens: string[], freeText?: string) => {
		const nextFreeText = freeText ?? appliedFreeText();
		if (freeText !== undefined) {
			dispatch({ type: "setTypedFreeText", value: freeText });
		}
		emitQuery(composeFilterQuery(tokens, chipKeys, nextFreeText));
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

	// The typed text only located the suggestion, so it is dropped either way.
	const toggleValueSuggestion = (token: string) => {
		updateFromChips(
			chipValues.includes(token)
				? chipValues.filter((chip) => chip !== token)
				: [...chipValues, token],
			"",
		);
		dispatch({ type: "close", input: "clear" });
	};

	// Returning to the category list highlights the row that was open, so the
	// keyboard position is never lost when the option rows unmount.
	const returnToCategories = () => {
		setHighlightedValue(activeCategoryKey ?? "");
		dispatch({ type: "leaveCategory" });
	};

	const commitCategoryOption = (token: string) => {
		updateFromChips([...chipValues, token]);
		returnToCategories();
	};

	const toggleCategoryOption = (token: string) => {
		updateFromChips(
			chipValues.includes(token)
				? chipValues.filter((chip) => chip !== token)
				: [...chipValues, token],
		);
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
		applyInlineChips(
			chipValues.includes(token)
				? chipValues.filter((chip) => chip !== token)
				: withInlineOption(token),
		);
	};

	// Enter or Tab on a highlighted inline option row or value suggestion. When
	// typed text located an applied option, keeps it and clears the text;
	// otherwise toggles it as a click does. Returns false for any other token.
	const completeHighlightedOption = (token: string) => {
		const isInlineRow = inlineOptionRows.some(
			({ categoryKey, option }) => optionToken(categoryKey, option) === token,
		);
		const isSuggestion = valueSuggestions.some(
			(suggestion) => suggestion.token === token,
		);
		if (!isInlineRow && !isSuggestion) {
			return false;
		}
		const keepApplied =
			inputValue.trim().length > 0 && chipValues.includes(token);
		if (isInlineRow) {
			if (keepApplied) {
				applyInlineChips(chipValues);
			} else {
				toggleInlineOption(token);
			}
		} else if (keepApplied) {
			updateFromChips(chipValues, "");
			dispatch({ type: "close", input: "clear" });
		} else {
			toggleValueSuggestion(token);
		}
		return true;
	};

	const focusAndLeaveCategory = () => {
		inputRef.current?.focus();
		returnToCategories();
	};

	// Typed text that could still be a filter being searched for (a category
	// name, or an option the menu lists for it) is not applied to the results
	// yet, so they do not empty out mid-word. Options are looked up with the
	// same queries as the suggestions, so one beyond a category's first page
	// counts, and a failed lookup also holds the text back. Held text applies
	// when the menu is dismissed or switched to all filters.
	const couldBeFilterSearch = async (text: string) => {
		if (text.length === 0) {
			return false;
		}
		const matchesLoadedOption = [
			...unfilteredOptions.optionsByKey.values(),
		].some((options) => filterOptionsByText(options, text).length > 0);
		if (matchCategories(text, categories).length > 0 || matchesLoadedOption) {
			return true;
		}
		const results = await Promise.allSettled(
			categories.map((category) =>
				queryClient.fetchQuery(
					filterComboboxOptions(category.key, category.getOptions, text, true),
				),
			),
		);
		return results.some(
			(result) =>
				result.status === "rejected" ||
				filterOptionsByText(result.value, text).length > 0,
		);
	};

	const applyTypedTextUnlessFilter = async (
		text: string,
		generation: number,
	) => {
		const couldBeFilter = await couldBeFilterSearch(text);
		if (generation !== typedSearchGenerationRef.current) {
			return;
		}
		emitQuery(
			composeFilterQuery(
				queryToChips(lastEmittedRef.current, chipKeys),
				chipKeys,
				couldBeFilter ? "" : text,
			),
		);
	};

	const applyTypedSearch = () => {
		cancelPendingTypedSearch();
		const query = composeFilterQuery(chipValues, chipKeys, typedFreeText);
		if (query !== lastEmittedRef.current) {
			emitQuery(query);
		}
	};

	// Also applies typed text as a free-text search, or steps back out of an
	// open category.
	const showAllFilters = () => {
		inputRef.current?.focus();
		if (mode === "category") {
			focusAndLeaveCategory();
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
			dispatch({ type: "close", input: "restore" });
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

	// Text typed ahead of a `key:` prefix is committed on the spot. Chip tokens
	// in it (e.g. a pasted `owner:me template:docker`) become chips rather than
	// free text that would duplicate the token on the next commit.
	const commitTextBeforePrefix = (text: string) => {
		const priorChips = queryToChips(text, chipKeys);
		const mergedChips = dedupeChips([...chipValues, ...priorChips], chipKeys);
		const freeText = extractFreeText(text, chipKeys);
		emitQuery(composeFilterQuery(mergedChips, chipKeys, freeText));
		return freeText;
	};

	const handleInputValueChange = (nextValue: string) => {
		cancelPendingTypedSearch();
		const typedCategory = parseTypedCategoryPrefix(
			nextValue,
			submenuCategories,
		);
		if (typedCategory) {
			dispatch({
				type: "enterCategory",
				categoryKey: typedCategory.categoryKey,
				query: typedCategory.query,
				typedFreeText: commitTextBeforePrefix(typedCategory.freeText),
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
				value: commitTextBeforePrefix(typedInline.freeText),
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
			const mergedChips = dedupeChips(
				[...chipValues, ...settledChips],
				chipKeys,
			);
			const settledFreeText = extractFreeText(settledText, chipKeys);
			const inputFreeText = inProgressIsPartialChip
				? [settledFreeText, inProgress].filter(Boolean).join(" ")
				: settledFreeText;
			emitQuery(composeFilterQuery(mergedChips, chipKeys, settledFreeText));
			dispatch({ type: "typeFreeText", value: inputFreeText });
			return;
		}

		dispatch({ type: "typeFreeText", value: nextValue });
		scheduleTypedSearch(nextValue.trim(), typedSearchGenerationRef.current);
	};

	// Radix only originates close requests (escape / outside press); opens flow
	// from the caller, so a dismissal restores the free-text input and applies
	// it as a search.
	const handleDismiss = () => {
		if (mode !== "category") {
			applyTypedSearch();
		}
		dispatch({ type: "close", input: "restore" });
	};

	// Chip removal mirrors Backspace: drop the token and keep the current popup
	// and input state untouched.
	const handleRemoveChip = (token: string) => {
		updateFromChips(chipValues.filter((entry) => entry !== token));
		// The emit cancels a pending typed search, so typed text that is not
		// applied yet is looked up again.
		const text = inputValue.trim();
		if (
			typeaheadActive &&
			typedInlinePrefix === null &&
			text !== appliedFreeText() &&
			queryToChips(text, chipKeys).length === 0
		) {
			scheduleTypedSearch(text, typedSearchGenerationRef.current);
		}
	};

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
			updateFromChips(chipValues.slice(0, -1));
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
		// may not be one of the suggested options. While the typed text is
		// debounced the highlight may not have reached the first row yet, so the
		// first matching row is taken, as it would be once highlighted.
		if (
			event.key === "Enter" &&
			mode === "browsing" &&
			typedInlinePrefix !== null &&
			typedInlinePrefix.query.trim().length > 0
		) {
			const { categoryKey } = typedInlinePrefix;
			const highlighted = getHighlightedValue();
			const hasHighlightedOption = inlineOptionRows.some(
				(row) => optionToken(row.categoryKey, row.option) === highlighted,
			);
			const firstRow = typeaheadQueryPending ? inlineOptionRows[0] : undefined;
			const candidate = firstRow
				? optionToken(firstRow.categoryKey, firstRow.option)
				: chipToken(categoryKey, typedInlinePrefix.query.trim());
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

		// Enter and Tab complete the highlighted row, and Tab otherwise moves
		// focus. ArrowRight only opens a highlighted category.
		const isComplete =
			event.key === "Enter" || (event.key === "Tab" && !event.shiftKey);
		if (!isComplete && event.key !== "ArrowRight") {
			return;
		}

		const highlighted = getHighlightedValue();
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
		unfilteredOptionsByKey: unfilteredOptions.optionsByKey,
		unfilteredOptionsErroredKeys: unfilteredOptions.erroredKeys,
		valueSuggestions,
		inlineOptionRows,
		allInlineOptionRows,
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
		},
	};
};
