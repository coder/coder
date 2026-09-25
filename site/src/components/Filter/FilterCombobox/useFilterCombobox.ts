import {
	type KeyboardEvent as ReactKeyboardEvent,
	useEffect,
	useMemo,
	useReducer,
	useRef,
	useState,
} from "react";
import { useQueries, useQuery } from "react-query";
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
	 * out of a category. The full category list is shown and typed text stays a
	 * workspace search instead of narrowing filters.
	 */
	browseAll: boolean;
	activeCategoryKey: string | null;
	inputValue: string;
	committedFreeText: string;
};

type Action =
	| { type: "openBrowsing" }
	| { type: "showAllFilters" }
	| {
			type: "enterCategory";
			categoryKey: string;
			query: string;
			committedFreeText: string;
	  }
	| { type: "typeInCategory"; value: string }
	| { type: "typeFilterSearch"; value: string }
	| { type: "typeFreeText"; value: string }
	| { type: "setCommittedFreeText"; value: string }
	| { type: "leaveCategory" }
	| { type: "close"; input: "restore" | "clear" }
	| { type: "reconcile"; freeText: string };

const closeState = (state: State, input: "restore" | "clear"): State => ({
	mode: "closed",
	browseAll: false,
	activeCategoryKey: null,
	committedFreeText: state.committedFreeText,
	inputValue: input === "restore" ? state.committedFreeText : "",
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
				committedFreeText: action.committedFreeText.trim(),
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
				committedFreeText: action.value.trim(),
			};
		case "setCommittedFreeText":
			return { ...state, committedFreeText: action.value.trim() };
		case "leaveCategory":
			return {
				...state,
				mode: "browsing",
				browseAll: true,
				activeCategoryKey: null,
				inputValue: state.committedFreeText,
			};
		case "close":
			return closeState(state, action.input);
		case "reconcile":
			return {
				...state,
				browseAll: false,
				committedFreeText: action.freeText,
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
 * Drives the unified workspace filter combobox: a `mode` state machine for the
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
			committedFreeText: freeText,
		};
	});
	const { mode, browseAll, activeCategoryKey, inputValue, committedFreeText } =
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

	const { debounced: debouncedOnChange, cancelDebounce } = useDebouncedFunction(
		(query: string) => {
			lastEmittedRef.current = query;
			onChange(query);
		},
		SEARCH_DEBOUNCE_MS,
	);

	const emitQuery = (query: string, immediate = false) => {
		if (immediate) {
			cancelDebounce();
			lastEmittedRef.current = query;
			onChange(query);
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
	// Scope toggles are on by default. This tracks the ones switched off while
	// the category has no chip; once a chip exists its key is the source of truth.
	const [narrowedScopes, setNarrowedScopes] = useState<ReadonlySet<string>>(
		() => new Set(),
	);
	const chipKeyOf = (token: string) => parseChipToken(token, chipKeys)?.key;
	const isScopeWidened = (category: FilterCategory) => {
		const toggle = category.scopeToggle;
		if (!toggle) {
			return false;
		}
		const appliedKeys = chipValues.map(chipKeyOf);
		if (appliedKeys.includes(toggle.chipKey)) {
			return true;
		}
		if (appliedKeys.includes(category.key)) {
			return false;
		}
		return !narrowedScopes.has(category.key);
	};
	// Query key the category's options commit under right now.
	const optionChipKey = (category: FilterCategory) =>
		category.scopeToggle && isScopeWidened(category)
			? category.scopeToggle.chipKey
			: category.key;

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
	// from them, and for categories that can be left out with a single option,
	// so they are left out before the menu opens. Shares its cache key with the
	// category view.
	const unfilteredOptionsEnabled = isBrowsing && activeCategoryKey === null;
	const unfilteredOptions = useQueries({
		queries: categories.map((category) =>
			filterComboboxOptions(
				category.key,
				category.getOptions,
				"",
				unfilteredOptionsEnabled ||
					Boolean(category.inlineOptions || !category.showWhenSingleOption),
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
	// Filtering by a category with at most one option would not narrow the
	// results, so categories stay out of the menu until they offer a real
	// choice. An applied chip keeps the category listed so it can change, and a
	// failed lookup keeps it listed so its flyout can offer a retry.
	const menuCategories = submenuCategories.filter((category) => {
		if (
			category.showWhenSingleOption ||
			unfilteredOptions.erroredKeys.has(category.key) ||
			chipValues.some((token) =>
				(category.chipKeys ?? [category.key]).includes(chipKeyOf(token) ?? ""),
			)
		) {
			return true;
		}
		return (unfilteredOptions.optionsByKey.get(category.key)?.length ?? 0) > 1;
	});

	const categoryQuery =
		activeCategoryKey !== null || browseAll ? "" : inputValue.trim();
	// Typing the start of a word in a scope toggle's pill label (e.g. `sha` for
	// `shared with owner`) finds its category, so the toggle is one step away.
	const findScopeMatch = (query: string) => {
		const scopeQuery = query.trim().toLowerCase();
		if (scopeQuery.length < 3) {
			return undefined;
		}
		return menuCategories.find((category) => {
			const pillLabel = category.scopeToggle?.pillLabel.toLowerCase();
			return (
				pillLabel !== undefined &&
				(pillLabel.startsWith(scopeQuery) ||
					pillLabel.split(" ").some((word) => word.startsWith(scopeQuery)))
			);
		});
	};
	const scopeMatchedCategory =
		typedInlinePrefix !== null ? undefined : findScopeMatch(categoryQuery);
	const matchedCategories = matchCategories(categoryQuery, menuCategories);
	const listedCategories =
		!open || typedInlinePrefix !== null
			? []
			: categoryQuery.length === 0
				? menuCategories
				: scopeMatchedCategory &&
						!matchedCategories.includes(scopeMatchedCategory)
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
	// options are filtered locally so matching rows stay visible.
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
					submenuCategories.map((category) => ({
						...category,
						chipKey: optionChipKey(category),
					})),
					typeaheadOptionsByKey,
					chipValues,
				);

	// A failed suggestion query for the current input ends loading, so the live
	// region announces the failure instead of repeating "Loading suggestions".
	const suggestionsError =
		typeaheadQuerySource.length > 0 &&
		!typeaheadQueryPending &&
		suggestionOptions.isError;
	// Includes the debounce window so the live region does not announce an
	// empty result before the request starts.
	const valueSuggestionsLoading =
		typeaheadQuerySource.length > 0 &&
		!suggestionsError &&
		(typeaheadQueryPending || suggestionOptions.isFetching);

	const typeaheadError = suggestionsError;

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

	// Announce a live-region message for each terminal state so screen readers
	// hear loading, failures, and empty results rather than silence.
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
		const nextFreeText = freeText ?? committedFreeText;
		if (freeText !== undefined) {
			dispatch({ type: "setCommittedFreeText", value: freeText });
		}
		emitQuery(composeFilterQuery(tokens, chipKeys, nextFreeText), true);
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
		const freeText = browseAll ? committedFreeText : "";
		emitQuery(composeFilterQuery(chipValues, chipKeys, freeText), true);
		dispatch({
			type: "enterCategory",
			categoryKey,
			query: "",
			committedFreeText: freeText,
		});
	};

	// A category with a scope toggle owns one chip across both of its keys, so
	// adding under one key drops the chip under the other.
	const withOptionToken = (token: string) => {
		const key = chipKeyOf(token);
		const category = categories.find(
			(entry) =>
				entry.scopeToggle &&
				(entry.key === key || entry.scopeToggle.chipKey === key),
		);
		const otherKey =
			category?.scopeToggle && key === category.key
				? category.scopeToggle.chipKey
				: category?.key;
		return [
			...chipValues.filter(
				(chip) => otherKey === undefined || chipKeyOf(chip) !== otherKey,
			),
			token,
		];
	};

	// Text typed to find a category in the main menu is filter search, so it is
	// dropped once a filter is picked from that category's flyout.
	const typedFilterText =
		mode === "browsing" && !browseAll && inputValue.trim().length > 0;

	const toggleScope = (categoryKey: string) => {
		const category = categories.find((entry) => entry.key === categoryKey);
		const toggle = category?.scopeToggle;
		if (!category || !toggle) {
			return;
		}
		const widened = isScopeWidened(category);
		const fromKey = widened ? toggle.chipKey : category.key;
		const toKey = widened ? category.key : toggle.chipKey;
		setNarrowedScopes((previous) => {
			const next = new Set(previous);
			if (widened) {
				next.add(category.key);
			} else {
				next.delete(category.key);
			}
			return next;
		});
		const rewritten = chipValues.map((token) => {
			const parsed = parseChipToken(token, chipKeys);
			return parsed?.key === fromKey ? chipToken(toKey, parsed.value) : token;
		});
		if (typedFilterText) {
			updateFromChips(rewritten, "");
			dispatch({ type: "typeFreeText", value: "" });
		} else if (rewritten.some((token, index) => token !== chipValues[index])) {
			updateFromChips(rewritten);
		}
	};

	// The typed text only located the suggestion, so it is dropped either way.
	const toggleValueSuggestion = (token: string) => {
		updateFromChips(
			chipValues.includes(token)
				? chipValues.filter((chip) => chip !== token)
				: withOptionToken(token),
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
		updateFromChips(withOptionToken(token), typedFilterText ? "" : undefined);
		returnToCategories();
	};

	const toggleCategoryOption = (token: string) => {
		if (!chipValues.includes(token)) {
			commitCategoryOption(token);
			return;
		}
		updateFromChips(
			chipValues.filter((chip) => chip !== token),
			typedFilterText ? "" : undefined,
		);
		returnToCategories();
	};

	// Typed filter text is dropped once an option is picked. Workspace search
	// text is kept: text typed ahead of a `status:` prefix, and all text in
	// browse-all mode.
	const applyInlineChips = (tokens: string[]) => {
		const freeText =
			browseAll || typedInlinePrefix !== null ? committedFreeText : "";
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

	// Typed text located the highlighted row, so completing it from the
	// keyboard keeps an applied option instead of removing it. A click on the
	// row still removes it. Returns false when the token is not an option row.
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

	const leaveCategory = () => {
		inputRef.current?.focus();
		returnToCategories();
	};

	// Typed text that could still be a filter being searched for (a category
	// name or a loaded option) is not applied to the results yet, so they do
	// not empty out mid-word. It applies once it matches no filter, or when the
	// menu is dismissed or switched to the full filter list.
	const couldBeFilterSearch = (text: string) => {
		const normalized = text.trim().toLowerCase();
		if (normalized.length === 0) {
			return false;
		}
		if (
			matchCategories(normalized, menuCategories).length > 0 ||
			findScopeMatch(normalized)
		) {
			return true;
		}
		return [...unfilteredOptions.optionsByKey.values()].some(
			(options) => filterOptionsByText(options, normalized).length > 0,
		);
	};

	const applyTypedSearch = () => {
		const query = composeFilterQuery(chipValues, chipKeys, committedFreeText);
		if (query !== lastEmittedRef.current) {
			emitQuery(query, true);
		}
	};

	const showFilterMenu = () => {
		inputRef.current?.focus();
		if (mode === "category") {
			leaveCategory();
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
			leaveCategory();
			return;
		}
		if (open) {
			if (!browseAll && inputValue.trim().length > 0) {
				showFilterMenu();
				return;
			}
			dispatch({ type: "close", input: "restore" });
			return;
		}
		inputRef.current?.focus();
		showFilterMenu();
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
		emitQuery(composeFilterQuery(mergedChips, chipKeys, freeText), true);
		return freeText;
	};

	const handleInputValueChange = (nextValue: string) => {
		const typedCategory = parseTypedCategoryPrefix(
			nextValue,
			submenuCategories,
		);
		if (typedCategory) {
			dispatch({
				type: "enterCategory",
				categoryKey: typedCategory.categoryKey,
				query: typedCategory.query,
				committedFreeText: commitTextBeforePrefix(typedCategory.freeText),
			});
			return;
		}

		if (mode === "category") {
			dispatch({ type: "typeInCategory", value: nextValue });
			return;
		}

		// Inline category prefixes are filter search, never workspace search, so
		// the prefix itself is withheld until an option is picked.
		const typedInline = parseTypedCategoryPrefix(nextValue, inlineCategories);
		if (typedInline) {
			dispatch({
				type: "setCommittedFreeText",
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
			emitQuery(
				composeFilterQuery(mergedChips, chipKeys, settledFreeText),
				true,
			);
			dispatch({ type: "typeFreeText", value: inputFreeText });
			return;
		}

		const searchText = couldBeFilterSearch(nextValue) ? "" : nextValue;
		emitQuery(composeFilterQuery(chipValues, chipKeys, searchText), false);
		dispatch({ type: "typeFreeText", value: nextValue });
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
	};

	const handleInputKeyDown = (event: ReactKeyboardEvent<HTMLInputElement>) => {
		const isBackspaceOrDelete =
			event.key === "Backspace" || event.key === "Delete";

		if (event.key === "ArrowLeft" && mode === "category") {
			event.preventDefault();
			leaveCategory();
			return;
		}

		if (isBackspaceOrDelete && inputValue === "" && mode === "category") {
			event.preventDefault();
			leaveCategory();
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
			const lastKey = chipKeyOf(chipValues[chipValues.length - 1]);
			const widenedCategory = categories.find(
				(category) =>
					category.scopeToggle !== undefined &&
					category.scopeToggle.chipKey === lastKey,
			);
			if (widenedCategory) {
				toggleScope(widenedCategory.key);
				return;
			}
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
		committedFreeText,
		activeCategoryKey,
		activeCategory,
		activeOptions,
		activeOptionsError,
		statusMessage,
		listedCategories,
		// Typed text is narrowing the category rows.
		filteringCategories: categoryQuery.length > 0,
		// Category found through its scope toggle label, whose flyout opens.
		scopeMatchKey: scopeMatchedCategory?.key ?? null,
		unfilteredOptionsByKey: unfilteredOptions.optionsByKey,
		unfilteredOptionsErroredKeys: unfilteredOptions.erroredKeys,
		valueSuggestions,
		inlineOptionRows,
		allInlineOptionRows,
		chipValues,
		highlightRef,
		// Whether a category's scope toggle is on, and the query key its options
		// commit under right now.
		scopeWidened: (categoryKey: string) => {
			const category = categories.find((entry) => entry.key === categoryKey);
			return category ? isScopeWidened(category) : false;
		},
		// Value of the category's applied chip under either of its scope keys.
		scopeValue: (categoryKey: string) => {
			const toggle = categories.find(
				(entry) => entry.key === categoryKey,
			)?.scopeToggle;
			const keys = [categoryKey, ...(toggle ? [toggle.chipKey] : [])];
			const values = chipValues.flatMap((token) => {
				const parsed = parseChipToken(token, keys);
				return parsed ? [parsed.value] : [];
			});
			return values.at(-1);
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
			showMenu: showFilterMenu,
			dismiss: handleDismiss,
			removeChip: handleRemoveChip,
			// Removes every chip and keeps the typed search text.
			clearChips: () => updateFromChips([]),
			retryActiveOptions,
			retryTypeahead,
			retryUnfilteredOptions: unfilteredOptions.refetch,
			selectCategory,
			toggleCategoryOption,
			toggleInlineOption,
			toggleScope,
			toggleValueSuggestion,
			onInputFocus: handleInputFocus,
			onInputKeyDown: handleInputKeyDown,
			onInputValueChange: handleInputValueChange,
			setHighlightedValue,
		},
	};
};
