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
	matchCategories,
	parseChipToken,
	parseTypedCategoryPrefix,
	queryToChips,
} from "./filterQuery";
import { filterComboboxOptions, SEARCH_DEBOUNCE_MS } from "./queries";
import type { FilterCategory, FilterOption } from "./types";

/**
 * The popup has three mutually exclusive modes. `closed` hides it; `browsing`
 * lists categories plus free-text typeahead suggestions; `category` narrows to
 * one category's options. `open` and `isBrowsing` are derived from `mode`.
 */
type Mode = "closed" | "browsing" | "category";

type State = {
	mode: Mode;
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
	| { type: "close"; input: "restore" | "clear" | "keep" }
	| { type: "reconcile"; freeText: string };

const closeState = (
	state: State,
	input: "restore" | "clear" | "keep",
): State => ({
	mode: "closed",
	browseAll: false,
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
	typeaheadError: boolean;
	typeaheadEmpty: boolean;
};

// Live-region text for each terminal state so screen readers hear loading,
// failures, and empty results rather than silence. Typeahead loading is voiced
// by the standalone <Spinner label> instead, to avoid a duplicate announcement.
const deriveStatusMessage = ({
	activeCategoryLabel,
	activeOptionsLoading,
	activeOptionsError,
	activeOptionsEmpty,
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
	if (typeaheadError) {
		return "Couldn't load suggestions.";
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
	// cmdk only reports highlight changes when its value is controlled.
	const [highlightedItem, setHighlightedItem] = useState("");
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
	const chipKeyOf = (token: string) => parseChipToken(token, chipKeys)?.key;
	const optionToken = (category: FilterCategory, option: FilterOption) =>
		option.token ?? chipToken(category.key, option.value);

	// Categories with a flyout or drill-in list. Inline categories render their
	// options directly in the main panel and never enter category mode.
	const submenuCategories = categories.filter(
		(category) => !category.inlineOptions,
	);
	const inlineCategories = categories.filter(
		(category) => category.inlineOptions,
	);
	const typeaheadActive = activeCategoryKey === null && isBrowsing;
	// A typed `status:` style prefix for an inline category narrows the main
	// panel to that category's options; the text after the colon is the query.
	const typedInlinePrefix =
		typeaheadActive && !browseAll
			? parseTypedCategoryPrefix(inputValue, inlineCategories)
			: null;

	// Category rows preview their options while the menu is open with an empty
	// input. The empty-query key is shared with the category view, so entering a
	// category reuses the cached result. Inline options always load because
	// applied chips take their labels from them, and other categories load so
	// single-option ones can be left out before the menu opens.
	const previewsEnabled = isBrowsing && activeCategoryKey === null;
	const previewOptions = useQueries({
		queries: categories.map((category) =>
			filterComboboxOptions(
				category.key,
				category.getOptions,
				"",
				previewsEnabled ||
					Boolean(category.inlineOptions || !category.showWhenSingleOption),
			),
		),
		combine: (results) => {
			const optionsByKey = new Map<string, readonly FilterOption[]>();
			results.forEach((result, index) => {
				if (result.data) {
					optionsByKey.set(categories[index].key, result.data);
				}
			});
			return {
				optionsByKey,
				isPending: results.some(
					(result, index) =>
						categories[index].inlineOptions &&
						!result.isError &&
						result.data === undefined,
				),
			};
		},
	});
	// Filtering by a category with at most one option would not narrow the
	// results, so categories stay out of the menu until they offer a real
	// choice. An applied chip keeps the category listed so it can change.
	const menuCategories = submenuCategories.filter((category) => {
		if (
			category.showWhenSingleOption ||
			chipValues.some((token) =>
				(category.chipKeys ?? [category.key]).includes(chipKeyOf(token) ?? ""),
			)
		) {
			return true;
		}
		return (previewOptions.optionsByKey.get(category.key)?.length ?? 0) > 1;
	});

	const categoryQuery =
		activeCategoryKey !== null || browseAll ? "" : inputValue.trim();
	const listedCategories =
		!open || typedInlinePrefix !== null
			? []
			: categoryQuery.length === 0
				? menuCategories
				: matchCategories(categoryQuery, menuCategories);

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

	const typeaheadQuerySource =
		typeaheadActive && !browseAll
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
			results.forEach((result, index) => {
				if (result.data) {
					optionsByKey.set(categories[index].key, result.data);
				}
			});
			return {
				optionsByKey,
				isError: results.some((result) => result.isError),
				// A not-yet-loaded (but not errored) query counts as fetching so the
				// popup keeps a spinner rather than flashing an empty section.
				isFetching: results.some(
					(result) =>
						result.isFetching || (!result.isError && result.data === undefined),
				),
				refetch: () => {
					for (const result of results) {
						void result.refetch();
					}
				},
			};
		},
	});

	const inlineOptionsSource =
		typeaheadQuerySource.length === 0
			? previewOptions.optionsByKey
			: typeaheadQueryPending
				? new Map<string, readonly FilterOption[]>()
				: suggestionOptions.optionsByKey;
	const inlineOptionsFor = (
		optionsByCategory: ReadonlyMap<string, readonly FilterOption[]>,
	) =>
		categories.flatMap((category) => {
			if (!category.inlineOptions) {
				return [];
			}
			return (optionsByCategory.get(category.key) ?? []).map((option) => {
				const token = option.token ?? chipToken(category.key, option.value);
				return {
					categoryKey: category.key,
					categoryLabel: category.inlineOptionsLabel ?? `${category.label} is…`,
					selected: chipValues.includes(token),
					showIcon: category.inlineOptionIcons ?? false,
					option,
				};
			});
		});
	const inlineOptions = open
		? inlineOptionsFor(inlineOptionsSource).filter(
				(option) =>
					typedInlinePrefix === null ||
					option.categoryKey === typedInlinePrefix.categoryKey,
			)
		: [];
	const mainInlineOptions = inlineOptionsFor(previewOptions.optionsByKey);

	const valueSuggestions =
		!typeaheadActive || typeaheadQueryPending || typedInlinePrefix !== null
			? []
			: collectValueSuggestions(
					inputValue,
					submenuCategories,
					suggestionOptions.optionsByKey,
					chipValues,
				);

	// A rejected suggestion query must not leave the popup spinning forever;
	// treat an error as "done loading" and surface it instead.
	const suggestionsError =
		activeCategoryKey === null && isBrowsing && suggestionOptions.isError;
	const valueSuggestionsLoading =
		activeCategoryKey === null &&
		isBrowsing &&
		inputValue.trim().length > 0 &&
		!suggestionsError &&
		suggestionOptions.isFetching;

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
		inlineOptions.length === 0 &&
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
		typeaheadError,
		typeaheadEmpty,
	});

	const retryTypeahead = () => {
		suggestionOptions.refetch();
	};

	const updateFromChips = (tokens: string[], freeText?: string) => {
		const exclusiveCategoryFor = (token: string) => {
			const key = parseChipToken(token, chipKeys)?.key;
			return categories.find(
				(category) =>
					category.inlineOptionsExclusive &&
					key !== undefined &&
					(category.chipKeys ?? [category.key]).includes(key),
			);
		};
		const normalizedTokens = tokens.filter((token, index) => {
			const category = exclusiveCategoryFor(token);
			return (
				category === undefined ||
				!tokens
					.slice(index + 1)
					.some((next) => exclusiveCategoryFor(next) === category)
			);
		});
		const nextFreeText = freeText ?? committedFreeText;
		if (freeText !== undefined) {
			dispatch({ type: "setCommittedFreeText", value: freeText });
		}
		emitQuery(
			composeFilterQuery(normalizedTokens, chipKeys, nextFreeText),
			true,
		);
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

	const selectValueSuggestion = (token: string) => {
		const selected = chipValues.includes(token);
		updateFromChips(
			selected
				? chipValues.filter((chip) => chip !== token)
				: [...chipValues, token],
			selected ? committedFreeText : "",
		);
		dispatch({ type: "close", input: selected ? "restore" : "clear" });
	};

	// Returning to the category list highlights the row that was open, so the
	// keyboard position is never lost when the option rows unmount.
	const returnToCategories = () => {
		setHighlightedItem(activeCategoryKey ?? "");
		dispatch({ type: "leaveCategory" });
	};

	const selectCategoryOption = (token: string) => {
		updateFromChips([...chipValues, token]);
		returnToCategories();
	};

	// Typed filter text is dropped once an option is picked, but workspace
	// search text typed ahead of a `status:` prefix is kept.
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
				: [...chipValues, token],
		);
	};

	const leaveCategory = () => {
		inputRef.current?.focus();
		returnToCategories();
	};

	// From inside a category the toggle steps back to the category list rather
	// than closing, so an accidental click can be corrected without reopening the menu.
	const showFilterMenu = () => {
		inputRef.current?.focus();
		if (mode === "category") {
			leaveCategory();
			return;
		}
		dispatch({ type: "showAllFilters" });
	};

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

		const normalized = nextValue.trim().toLowerCase();
		const matchesSelectedOption =
			normalized.length > 0 &&
			categories.some((category) =>
				(previewOptions.optionsByKey.get(category.key) ?? []).some((option) => {
					const token = optionToken(category, option);
					return (
						chipValues.includes(token) &&
						(option.label.toLowerCase().includes(normalized) ||
							option.value.toLowerCase().includes(normalized))
					);
				}),
			);
		if (matchesSelectedOption) {
			cancelDebounce();
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
			const highlighted = highlightedItem;
			const candidate = chipToken(activeCategory.key, inputValue.trim());
			const hasHighlightedOption = activeOptions?.some(
				(option) => optionToken(activeCategory, option) === highlighted,
			);
			if (
				!activeOptionsLoading &&
				!hasHighlightedOption &&
				parseChipToken(candidate, chipKeys)
			) {
				event.preventDefault();
				selectCategoryOption(candidate);
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
			const candidate = chipToken(
				typedInlinePrefix.categoryKey,
				typedInlinePrefix.query.trim(),
			);
			const hasHighlightedOption = inlineOptions.some(
				({ categoryKey, option }) =>
					(option.token ?? chipToken(categoryKey, option.value)) ===
					highlightedItem,
			);
			if (
				!typeaheadQueryPending &&
				!hasHighlightedOption &&
				parseChipToken(candidate, chipKeys)
			) {
				event.preventDefault();
				applyInlineChips(
					chipValues.includes(candidate)
						? chipValues
						: [...chipValues, candidate],
				);
				return;
			}
		}

		if (
			(event.key === "ArrowRight" || event.key === "Enter") &&
			mode === "browsing"
		) {
			const category = listedCategories.find(
				(entry) => entry.key === highlightedItem,
			);
			if (category) {
				event.preventDefault();
				selectCategory(category.key);
				return;
			}
		}

		// Tab completes a highlighted filter and otherwise moves focus.
		const isTabComplete = event.key === "Tab" && !event.shiftKey;
		if (!isTabComplete || mode !== "browsing") {
			return;
		}

		const highlighted = highlightedItem;
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
		activeOptionsError,
		statusMessage,
		listedCategories,
		// Typed text is narrowing the category rows.
		filteringCategories: categoryQuery.length > 0,
		browseCategoryOptions: previewOptions.optionsByKey,
		valueSuggestions,
		inlineOptions,
		mainInlineOptions,
		chipValues,
		highlightedItem,
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
			selectCategory,
			selectCategoryOption,
			toggleInlineOption,
			selectValueSuggestion,
			onInputFocus: handleInputFocus,
			onInputKeyDown: handleInputKeyDown,
			onInputValueChange: handleInputValueChange,
			setHighlightedItem,
		},
	};
};
