import { Badge } from "components/Badge/Badge";
import { Spinner } from "components/Spinner/Spinner";
import { useEffectEvent } from "hooks/hookPolyfills";
import { ListFilter, SearchIcon, XIcon } from "lucide-react";
import {
	type KeyboardEvent,
	type Ref,
	useCallback,
	useEffect,
	useLayoutEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { cn } from "utils/cn";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/**
 * Represents a single option in a filter category dropdown.
 */
export type FilterOption = {
	label: string;
	value: string;
	/** Optional icon/avatar rendered before the label. */
	startIcon?: React.ReactNode;
	/** Optional secondary text (e.g. email) rendered below the label. */
	subtitle?: string;
};

/**
 * Describes a filter category (e.g. "Owner", "Status").  Consumers pass an
 * array of these to configure which categories are available.
 */
export type FilterCategory = {
	/** Machine-readable key used in the query string (e.g. "owner"). */
	key: string;
	/** Human-readable label shown in the UI (e.g. "Owner"). */
	label: string;
	/**
	 * Async function that returns available options for this category.
	 * Receives the current search text typed by the user inside the category
	 * dropdown so the consumer can do server-side filtering.
	 */
	getOptions: (query: string) => Promise<FilterOption[]>;
	/** Optional icon rendered next to the category button. */
	icon?: React.ReactNode;
	/**
	 * When true, multiple values can be selected for this category
	 * (e.g. `status:running status:stopped`). Defaults to false
	 * (selecting a new value replaces the existing chip).
	 */
	multiSelect?: boolean;
};

/**
 * A single active filter chip displayed in the input area.
 */
export type FilterChip = {
	/** The category key (e.g. "owner"). */
	key: string;
	/** The selected value (e.g. "me"). Empty string means incomplete. */
	value: string;
};

/**
 * A search result representing an actual resource (e.g. a workspace) rather
 * than a filter option. Displayed in a separate "Results" section.
 */
export type SearchResult = {
	label: string;
	value: string;
	/** Optional icon/avatar rendered before the label. */
	startIcon?: React.ReactNode;
	/** Optional secondary text rendered below the label. */
	subtitle?: string;
};

export type FilterSearchFieldProps = {
	/** Current filter query string (e.g. "owner:me status:running"). */
	value: string;
	/** Called whenever the serialized query string changes. */
	onChange: (query: string) => void;
	/** Available filter categories. */
	categories: FilterCategory[];
	/**
	 * Optional async function that returns matching resources (e.g.
	 * workspaces) for the current freeform search text. Results are shown in
	 * a dedicated section so the user can see what their query will match.
	 */
	getSearchResults?: (query: string) => Promise<SearchResult[]>;
	placeholder?: string;
	className?: string;
	autoFocus?: boolean;
	ref?: Ref<HTMLInputElement>;
	"aria-label"?: string;
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Parse a query string like "owner:me status:running my-workspace" into
 * structured chips + freeform text.
 */
function parseQuery(
	query: string,
	categories: FilterCategory[],
): { chips: FilterChip[]; freeform: string } {
	if (!query.trim()) {
		return { chips: [], freeform: "" };
	}

	const categoryKeys = new Set(categories.map((c) => c.key));
	const chips: FilterChip[] = [];
	const freeformParts: string[] = [];

	for (const token of query.split(" ")) {
		const colonIdx = token.indexOf(":");
		if (colonIdx > 0) {
			const key = token.slice(0, colonIdx);
			const value = token.slice(colonIdx + 1);
			if (categoryKeys.has(key) && value) {
				chips.push({ key, value });
				continue;
			}
		}
		if (token) {
			freeformParts.push(token);
		}
	}

	return { chips, freeform: freeformParts.join(" ") };
}

/**
 * Serialize chips + freeform text back into a query string.  Tags always
 * come first, freeform text at the end.
 */
function serializeQuery(chips: FilterChip[], freeform: string): string {
	const parts: string[] = [];
	for (const chip of chips) {
		if (chip.value) {
			parts.push(`${chip.key}:${chip.value}`);
		}
	}
	const trimmed = freeform.trim();
	if (trimmed) {
		parts.push(trimmed);
	}
	return parts.join(" ");
}

/** A single result from searching all categories. */
type GlobalSearchResult = {
	category: FilterCategory;
	option: FilterOption;
};

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

interface ChipBadgeProps {
	chip: FilterChip;
	categoryLabel: string;
	onRemove: () => void;
}

const ChipBadge: React.FC<ChipBadgeProps> = ({
	chip,
	categoryLabel,
	onRemove,
}) => {
	const isComplete = chip.value !== "";
	return (
		<Badge
			variant="default"
			size="md"
			className={cn(
				"shrink-0 gap-0.5 select-none text-content-secondary",
				!isComplete && "border border-dashed border-border-default",
			)}
		>
			<span>{categoryLabel}</span>
			{isComplete && (
				<>
					<span>:</span>
					<span>{chip.value}</span>
					<button
						type="button"
						className="flex items-center rounded-sm p-0 text-content-secondary cursor-pointer bg-transparent border-none outline-none focus-visible:ring-2 focus-visible:ring-content-link"
						onMouseDown={(e) => {
							// Prevent the input from losing focus, which would
							// trigger onBlur -> closeDropdown before the click
							// event fires.
							e.preventDefault();
							e.stopPropagation();
						}}
						onClick={(e) => {
							e.stopPropagation();
							onRemove();
						}}
						aria-label={`Remove ${categoryLabel}:${chip.value} filter`}
					>
						<XIcon className="size-3.5" />
					</button>
				</>
			)}
		</Badge>
	);
};

interface CategoryButtonProps {
	category: FilterCategory;
	onSelect: () => void;
	id?: string;
	isHighlighted?: boolean;
	showFocusRing?: boolean;
	onMouseEnter?: () => void;
}

const CategoryButton: React.FC<CategoryButtonProps> = ({
	category,
	onSelect,
	id,
	isHighlighted,
	showFocusRing,
	onMouseEnter,
}) => {
	return (
		// biome-ignore lint/a11y/useKeyWithClickEvents: Keyboard navigation is handled by the parent input's onKeyDown handler.
		<div
			id={id}
			role="option"
			tabIndex={-1}
			aria-selected={isHighlighted}
			className={cn(
				"inline-flex items-center gap-1.5 rounded-md border border-solid border-border-default",
				"bg-transparent px-3 py-1.5 text-xs font-medium text-content-secondary",
				"cursor-pointer transition-colors",
				"hover:bg-surface-secondary hover:text-content-primary",
				"focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
				isHighlighted && "bg-surface-secondary text-content-primary",
				isHighlighted && showFocusRing && "ring-2 ring-content-link",
			)}
			onMouseDown={(e) => {
				// Keep focus on the input so the popover doesn't close.
				e.preventDefault();
			}}
			onClick={onSelect}
			onMouseEnter={onMouseEnter}
		>
			{category.icon}
			{category.label}
		</div>
	);
};
interface OptionItemProps {
	option: FilterOption;
	onSelect: () => void;
	isHighlighted: boolean;
	showFocusRing?: boolean;
	id: string;
	/** Optional category label prefix (used in global search results). */
	categoryLabel?: string;
	onMouseEnter?: () => void;
}

const OptionItem: React.FC<OptionItemProps> = ({
	option,
	onSelect,
	isHighlighted,
	showFocusRing,
	id,
	categoryLabel,
	onMouseEnter,
}) => {
	return (
		// biome-ignore lint/a11y/useKeyWithClickEvents: Keyboard navigation is handled by the parent input's onKeyDown handler.
		<div
			id={id}
			role="option"
			tabIndex={-1}
			aria-selected={isHighlighted}
			className={cn(
				"flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm cursor-pointer",
				"transition-colors",
				isHighlighted
					? "bg-surface-secondary text-content-primary"
					: "text-content-secondary",
				isHighlighted && showFocusRing && "ring-2 ring-content-link",
			)}
			onClick={onSelect}
			onMouseEnter={onMouseEnter}
			onMouseDown={(e) => {
				// Prevent blur on the input when clicking an option.
				e.preventDefault();
			}}
		>
			{option.startIcon && (
				<span className="flex shrink-0 items-center justify-center size-6">
					{option.startIcon}
				</span>
			)}
			<span className="flex flex-col min-w-0">
				<span className="text-sm font-medium truncate text-content-primary">
					{categoryLabel && (
						<span className="text-content-secondary font-normal">
							{categoryLabel}:{" "}
						</span>
					)}
					{option.label}
				</span>
				{option.subtitle && (
					<span className="text-xs text-content-secondary truncate">
						{option.subtitle}
					</span>
				)}
			</span>
		</div>
	);
};

interface SearchResultsSectionProps {
	searchResults: SearchResult[];
	isLoading: boolean;
	isReady: boolean;
	highlightedIndex: number;
	showFocusRing: boolean;
	/** The nav index offset for keyboard navigation. */
	navIndexOffset: number;
	/** Called when a search result is selected. */
	onSelect: (result: SearchResult) => void;
	/** Called when a search result is hovered. */
	onHighlight: (index: number) => void;
}

const SearchResultsSection: React.FC<SearchResultsSectionProps> = ({
	searchResults,
	isLoading,
	isReady,
	highlightedIndex,
	showFocusRing,
	navIndexOffset,
	onSelect,
	onHighlight,
}) => {
	if (isLoading && searchResults.length === 0) {
		return (
			<div>
				<div className="flex items-center justify-center py-4">
					<Spinner size="sm" loading />
				</div>
			</div>
		);
	}

	if (!isReady || searchResults.length === 0) {
		return null;
	}

	return (
		<div className="px-2 pb-2 pt-1">
			<p
				role="presentation"
				className="text-xs font-medium text-content-secondary px-2 pb-1"
			>
				Workspaces
			</p>
			{searchResults.map((result, idx) => {
				const navIdx = navIndexOffset + idx;
				return (
					// biome-ignore lint/a11y/useKeyWithClickEvents: Keyboard navigation is handled by the parent input's onKeyDown handler.
					<div
						key={result.value}
						id={`filter-option-${navIdx}`}
						role="option"
						tabIndex={-1}
						aria-selected={highlightedIndex === navIdx}
						className={cn(
							"flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm cursor-pointer",
							"transition-colors",
							highlightedIndex === navIdx
								? "bg-surface-secondary text-content-primary"
								: "text-content-secondary",
							highlightedIndex === navIdx &&
								showFocusRing &&
								"ring-2 ring-content-link",
						)}
						onClick={() => onSelect(result)}
						onMouseEnter={() => onHighlight(navIdx)}
						onMouseDown={(e) => {
							e.preventDefault();
						}}
					>
						{result.startIcon && (
							<span className="flex shrink-0 items-center justify-center size-6">
								{result.startIcon}
							</span>
						)}
						<span className="flex flex-col min-w-0">
							<span className="text-sm font-medium truncate text-content-primary">
								{result.label}
							</span>
							{result.subtitle && (
								<span className="text-xs text-content-secondary truncate">
									{result.subtitle}
								</span>
							)}
						</span>
					</div>
				);
			})}
		</div>
	);
};
// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

type DropdownMode =
	| { type: "categories" }
	| { type: "options"; categoryKey: string }
	| { type: "typeahead" };

export const FilterSearchField: React.FC<FilterSearchFieldProps> = ({
	value = "",
	onChange,
	categories,
	getSearchResults,
	placeholder = "Search...",
	className,
	autoFocus = false,
	ref,
	...ariaProps
}) => {
	// ---- Refs ----
	const internalRef = useRef<HTMLInputElement | null>(null);
	const containerRef = useRef<HTMLDivElement | null>(null);

	// ---- Derived state from the controlled value ----
	const parsed = useMemo(
		() => parseQuery(value, categories),
		[value, categories],
	);

	// ---- Local state ----
	const [chips, setChips] = useState<FilterChip[]>(parsed.chips);
	const [freeformText, setFreeformText] = useState(parsed.freeform);
	const [isOpen, setIsOpen] = useState(false);
	const [dropdownMode, setDropdownMode] = useState<DropdownMode>({
		type: "categories",
	});
	const [highlightedIndex, setHighlightedIndex] = useState(-1);
	const [isKeyboardNav, setIsKeyboardNav] = useState(false);
	const [categoryOptions, setCategoryOptions] = useState<FilterOption[]>([]);
	const [isLoadingOptions, setIsLoadingOptions] = useState(false);
	const [categorySearchText, setCategorySearchText] = useState("");

	// Typeahead: categories whose keys start with the current input.
	const [typeaheadMatches, setTypeaheadMatches] = useState<FilterCategory[]>(
		[],
	);

	// Global search: results from searching all categories.
	const [globalResults, setGlobalResults] = useState<GlobalSearchResult[]>([]);
	const [isLoadingGlobal, setIsLoadingGlobal] = useState(false);

	// Resource search results (e.g. matching workspaces).
	const [searchResults, setSearchResults] = useState<SearchResult[]>([]);
	const [isLoadingSearch, setIsLoadingSearch] = useState(false);
	const searchIdRef = useRef(0);

	// Track in-flight request IDs so stale responses are discarded.
	const globalSearchIdRef = useRef(0);
	const categorySearchIdRef = useRef(0);

	// Debounce timer for freeform/typeahead API calls.
	const debounceRef = useRef<ReturnType<typeof setTimeout> | undefined>(
		undefined,
	);

	// Tracks whether the current global search query has had at least one
	// completed fetch.  This prevents a "No results found" flash while the
	// very first request for a new query is still in-flight.
	const [_globalSearchReady, setGlobalSearchReady] = useState(false);
	const [searchResultsReady, setSearchResultsReady] = useState(false);

	// ---- Category lookup ----
	const categoryMap = useMemo(() => {
		const map = new Map<string, FilterCategory>();
		for (const cat of categories) {
			map.set(cat.key, cat);
		}
		return map;
	}, [categories]);

	// ---- Sync from external value ----
	useEffect(() => {
		const newParsed = parseQuery(value, categories);
		setChips(newParsed.chips);
		setFreeformText(newParsed.freeform);
	}, [value, categories]);

	// Clean up debounce timer on unmount.
	useEffect(() => {
		return () => {
			if (debounceRef.current) clearTimeout(debounceRef.current);
		};
	}, []);

	// ---- Emit changes to parent ----
	const emitChange = useCallback(
		(nextChips: FilterChip[], nextFreeform: string) => {
			const completeChips = nextChips.filter((c) => c.value !== "");
			const serialized = serializeQuery(completeChips, nextFreeform);
			onChange(serialized);
		},
		[onChange],
	);

	// ---- Auto-focus ----
	const focusOnMount = useEffectEvent((): void => {
		if (autoFocus) {
			internalRef.current?.focus();
		}
	});
	useLayoutEffect(() => {
		focusOnMount();
	}, [focusOnMount]);

	// ---- Ref forwarding ----
	const setRefs = useCallback(
		(el: HTMLInputElement | null) => {
			internalRef.current = el;
			if (typeof ref === "function") {
				ref(el);
			} else if (ref) {
				(ref as React.MutableRefObject<HTMLInputElement | null>).current = el;
			}
		},
		[ref],
	);

	// ---- Load options for a category ----
	const loadCategoryOptions = useCallback(
		async (categoryKey: string, query: string) => {
			const category = categoryMap.get(categoryKey);
			if (!category) return;
			const requestId = ++categorySearchIdRef.current;
			setIsLoadingOptions(true);
			try {
				const options = await category.getOptions(query);
				if (categorySearchIdRef.current === requestId) {
					setCategoryOptions(options);
				}
			} finally {
				if (categorySearchIdRef.current === requestId) {
					setIsLoadingOptions(false);
				}
			}
		},
		[categoryMap],
	);

	// ---- Load resource search results (e.g. workspaces) ----
	const loadSearchResults = useCallback(
		async (query: string) => {
			if (!getSearchResults) return;
			const requestId = ++searchIdRef.current;
			setIsLoadingSearch(true);
			setSearchResultsReady(false);
			try {
				const results = await getSearchResults(query);
				if (searchIdRef.current === requestId) {
					setSearchResults(results);
				}
			} catch {
				if (searchIdRef.current === requestId) {
					setSearchResults([]);
				}
			} finally {
				if (searchIdRef.current === requestId) {
					setIsLoadingSearch(false);
					setSearchResultsReady(true);
				}
			}
		},
		[getSearchResults],
	);

	// ---- Load global search results across all categories ----
	const loadGlobalResults = useCallback(
		async (query: string) => {
			const requestId = ++globalSearchIdRef.current;
			setIsLoadingGlobal(true);
			setGlobalSearchReady(false);
			try {
				const allResults: GlobalSearchResult[] = [];
				const lowerQuery = query.toLowerCase();
				// Query every category, then decide what to keep:
				// - Category name matches query (e.g. "sta" → "status"):
				//   include ALL its options.
				// - Category name doesn't match: include only options
				//   whose label/value contains the query (e.g. "test" →
				//   owner "testuser01").
				const promises = categories.map(async (category) => {
					const categoryNameMatches =
						category.key.toLowerCase().includes(lowerQuery) ||
						category.label.toLowerCase().includes(lowerQuery);
					if (categoryNameMatches) {
						const options = await category.getOptions("");
						return options.map((option) => ({ category, option }));
					}
					const options = await category.getOptions(query);
					const filtered = options.filter((option) => {
						const haystack = [option.label, option.value]
							.join(" ")
							.toLowerCase();
						return haystack.includes(lowerQuery);
					});
					return filtered.map((option) => ({ category, option }));
				});
				const settled = await Promise.allSettled(promises);
				for (const result of settled) {
					if (result.status === "fulfilled") {
						allResults.push(...result.value);
					}
				}
				if (globalSearchIdRef.current === requestId) {
					setGlobalResults(allResults);
				}
			} finally {
				if (globalSearchIdRef.current === requestId) {
					setIsLoadingGlobal(false);
					setGlobalSearchReady(true);
				}
			}
		},
		[categories],
	);
	// ---- Dropdown open/close ----
	const openDropdown = useCallback(
		(mode: DropdownMode = { type: "categories" }) => {
			setIsOpen(true);
			setDropdownMode(mode);
			setHighlightedIndex(-1);
			if (mode.type === "options") {
				setCategorySearchText("");
				loadCategoryOptions(mode.categoryKey, "");
			}
		},
		[loadCategoryOptions],
	);

	const closeDropdown = useCallback(() => {
		// Cancel any pending debounced API calls.
		if (debounceRef.current) {
			clearTimeout(debounceRef.current);
		}
		// Remove any incomplete chips on close.
		const completeChips = chips.filter((c) => c.value !== "");
		setChips(completeChips);
		setIsOpen(false);
		setDropdownMode({ type: "categories" });
		setCategorySearchText("");
		setCategoryOptions([]);
		setHighlightedIndex(-1);
		setTypeaheadMatches([]);
		setGlobalResults([]);
		setSearchResults([]);
		setIsLoadingOptions(false);
		setIsLoadingGlobal(false);
		setIsLoadingSearch(false);
		setSearchResultsReady(true);
		setGlobalSearchReady(true);
		// Cancel stale in-flight requests.
		++categorySearchIdRef.current;
		++globalSearchIdRef.current;
		++searchIdRef.current;
		emitChange(completeChips, freeformText);
	}, [chips, freeformText, emitChange]);
	// ---- Select a search result (e.g. a workspace) ----
	const selectSearchResult = useCallback(
		(result: SearchResult) => {
			const completeChips = chips.filter((c) => c.value !== "");
			setChips(completeChips);
			setFreeformText(result.value);
			setIsOpen(false);
			setDropdownMode({ type: "categories" });
			setCategorySearchText("");
			setCategoryOptions([]);
			setHighlightedIndex(-1);
			setTypeaheadMatches([]);
			setGlobalResults([]);
			setSearchResults([]);
			emitChange(completeChips, result.value);
		},
		[chips, emitChange],
	);

	// ---- Category selection (from default dropdown) ----
	const selectCategory = useCallback(
		(categoryKey: string) => {
			const newChip: FilterChip = { key: categoryKey, value: "" };
			const isMulti = categoryMap.get(categoryKey)?.multiSelect;
			// Strip incomplete chips. For single-select categories also
			// remove existing chips with the same key.
			const nextChips = [
				...chips.filter(
					(c) => c.value !== "" && (isMulti || c.key !== categoryKey),
				),
				newChip,
			];
			setChips(nextChips);
			setFreeformText("");
			setDropdownMode({ type: "options", categoryKey });
			setCategorySearchText("");
			setHighlightedIndex(-1);
			setTypeaheadMatches([]);
			setGlobalResults([]);
			setSearchResults([]);
			setIsLoadingGlobal(false);
			setIsLoadingSearch(false);
			setSearchResultsReady(true);
			setGlobalSearchReady(true);
			// Cancel stale in-flight requests.
			++globalSearchIdRef.current;
			++searchIdRef.current;
			loadCategoryOptions(categoryKey, "");
			internalRef.current?.focus();
		},
		[chips, categoryMap, loadCategoryOptions],
	);
	// ---- Option selection (completes a chip) ----
	const selectOption = useCallback(
		(option: FilterOption, categoryKey?: string) => {
			let nextChips: FilterChip[];
			if (categoryKey) {
				// Global search: add chip. For single-select categories
				// replace any existing chip with the same key.
				const isMulti = categoryMap.get(categoryKey)?.multiSelect;
				nextChips = [
					...chips.filter(
						(c) => c.value !== "" && (isMulti || c.key !== categoryKey),
					),
					{ key: categoryKey, value: option.value },
				];
			} else {
				// Options mode: fill the incomplete chip with the selected
				// value. For single-select categories also remove any
				// other chip with the same key.
				const incompleteKey = chips.find((c) => c.value === "")?.key;
				const isMulti = incompleteKey
					? categoryMap.get(incompleteKey)?.multiSelect
					: false;
				nextChips = chips
					.filter((c) => {
						if (c.value === "") return true;
						return isMulti || c.key !== incompleteKey;
					})
					.map((c) => (c.value === "" ? { ...c, value: option.value } : c));
			}
			setChips(nextChips);
			setCategorySearchText("");
			setFreeformText("");
			setDropdownMode({ type: "categories" });
			setHighlightedIndex(-1);
			setTypeaheadMatches([]);
			setGlobalResults([]);
			setSearchResults([]);
			setIsLoadingOptions(false);
			setIsLoadingGlobal(false);
			setIsLoadingSearch(false);
			setSearchResultsReady(true);
			setGlobalSearchReady(true);
			setCategoryOptions([]);
			// Cancel stale in-flight requests.
			++categorySearchIdRef.current;
			++globalSearchIdRef.current;
			++searchIdRef.current;
			emitChange(nextChips, "");
			setIsOpen(true);
			internalRef.current?.focus();
		},
		[chips, categoryMap, emitChange],
	);
	// ---- Remove a chip by its original index in the chips array ----
	const removeChip = useCallback(
		(chipIndex: number) => {
			const nextChips = chips.filter((_, i) => i !== chipIndex);
			setChips(nextChips);
			emitChange(nextChips, freeformText);
			internalRef.current?.focus();
		},
		[chips, freeformText, emitChange],
	);

	// ---- Compute typeahead matches ----
	const computeTypeahead = useCallback(
		(text: string) => {
			const trimmed = text.trim().toLowerCase();
			if (!trimmed) {
				setTypeaheadMatches([]);
				return [];
			}
			const matches = categories.filter(
				(c) =>
					c.key.toLowerCase().startsWith(trimmed) ||
					c.label.toLowerCase().startsWith(trimmed),
			);
			setTypeaheadMatches(matches);
			return matches;
		},
		[categories],
	);

	// Categories that contain the input text (for typeahead filtering).
	const filteredCategories = useMemo(() => {
		const trimmed = freeformText.trim().toLowerCase();
		if (!trimmed) return categories;
		return categories.filter(
			(c) =>
				c.key.toLowerCase().includes(trimmed) ||
				c.label.toLowerCase().includes(trimmed),
		);
	}, [categories, freeformText]);
	// ---- Check if user is typing a "key:" pattern ----
	const checkForKeyColonPattern = useCallback(
		(text: string) => {
			// Match pattern like "owner:" at the end of input.
			const match = text.match(/(\S+):$/);
			if (match) {
				const key = match[1];
				const category = categoryMap.get(key);
				if (category) {
					// Remove the "key:" from freeform and start a chip.
					const remaining = text.slice(0, text.length - match[0].length).trim();
					setFreeformText(remaining);
					const newChip: FilterChip = { key, value: "" };
					const isMulti = categoryMap.get(key)?.multiSelect;
					const nextChips = [
						...chips.filter(
							(c) => c.value !== "" && (isMulti || c.key !== key),
						),
						newChip,
					];
					setChips(nextChips);
					setDropdownMode({ type: "options", categoryKey: key });
					setCategorySearchText("");
					setHighlightedIndex(-1);
					setTypeaheadMatches([]);
					setGlobalResults([]);
					loadCategoryOptions(key, "");
					return true;
				}
			}

			// Match complete pattern like "owner:me " (trailing space).
			const completeMatch = text.match(/(\S+):(\S+)\s$/);
			if (completeMatch) {
				const key = completeMatch[1];
				const val = completeMatch[2];
				const category = categoryMap.get(key);
				if (category && val) {
					const remaining = text
						.slice(0, text.length - completeMatch[0].length)
						.trim();
					setFreeformText(remaining);
					const newChip: FilterChip = { key, value: val };
					const isMulti = categoryMap.get(key)?.multiSelect;
					const nextChips = [
						...chips.filter(
							(c) => c.value !== "" && (isMulti || c.key !== key),
						),
						newChip,
					];
					setChips(nextChips);
					emitChange(nextChips, remaining);
					setDropdownMode({ type: "categories" });
					return true;
				}
			}

			return false;
		},
		[categoryMap, chips, loadCategoryOptions, emitChange],
	);

	// ---- Freeform input handling ----
	const handleInputChange = useCallback(
		(e: React.ChangeEvent<HTMLInputElement>) => {
			const text = e.target.value;

			// When in options mode, the input is used for searching options.
			if (dropdownMode.type === "options") {
				setCategorySearchText(text);
				setHighlightedIndex(-1);
				loadCategoryOptions(dropdownMode.categoryKey, text);
				return;
			}

			if (checkForKeyColonPattern(text)) {
				return;
			}

			setFreeformText(text);
			const trimmed = text.trim();

			if (!trimmed) {
				// Empty input: show category buttons.
				if (!isOpen) {
					openDropdown({ type: "categories" });
				} else {
					setDropdownMode({ type: "categories" });
					setTypeaheadMatches([]);
					setGlobalResults([]);
					setSearchResults([]);
				}
				return;
			}

			// Compute typeahead matches (e.g. "own" -> "owner").
			computeTypeahead(trimmed);

			if (!isOpen) {
				setIsOpen(true);
			}
			setDropdownMode({ type: "typeahead" });
			setHighlightedIndex(-1);
			setGlobalResults([]);
			setIsLoadingGlobal(true);
			// Clear previous debounce timer.
			if (debounceRef.current) {
				clearTimeout(debounceRef.current);
			}
			// Debounce the API calls.
			debounceRef.current = setTimeout(() => {
				loadGlobalResults(trimmed);
				loadSearchResults(trimmed);
			}, 300);
		},
		[
			dropdownMode,
			isOpen,
			openDropdown,
			loadCategoryOptions,
			checkForKeyColonPattern,
			computeTypeahead,
			loadGlobalResults,
			loadSearchResults,
		],
	);

	// ---- Flat list of items for keyboard nav in each mode ----
	type NavItem =
		| { kind: "category"; category: FilterCategory }
		| { kind: "option"; option: FilterOption }
		| { kind: "global"; result: GlobalSearchResult }
		| { kind: "search"; result: SearchResult };

	const navItems = useMemo((): NavItem[] => {
		if (dropdownMode.type === "categories") {
			return categories.map((c) => ({ kind: "category", category: c }));
		}
		if (dropdownMode.type === "options") {
			return categoryOptions.map((o) => ({ kind: "option", option: o }));
		}
		if (dropdownMode.type === "typeahead") {
			const items: NavItem[] = filteredCategories.map((c) => ({
				kind: "category",
				category: c,
			}));
			for (const r of globalResults) {
				items.push({ kind: "global", result: r });
			}
			if (searchResultsReady) {
				for (const r of searchResults) {
					items.push({ kind: "search", result: r });
				}
			}
			return items;
		}
		return [];
	}, [
		dropdownMode,
		categories,
		filteredCategories,
		categoryOptions,
		globalResults,
		searchResults,
		searchResultsReady,
	]);
	// ---- Keyboard navigation ----
	const handleKeyDown = useCallback(
		(e: KeyboardEvent<HTMLInputElement>) => {
			if (e.key === "Escape") {
				closeDropdown();
				internalRef.current?.blur();
				return;
			}

			if (
				e.key === "Backspace" &&
				dropdownMode.type !== "options" &&
				freeformText === "" &&
				chips.length > 0
			) {
				// Remove the last chip.
				e.preventDefault();
				removeChip(chips.length - 1);
				return;
			}

			if (
				e.key === "Backspace" &&
				dropdownMode.type === "options" &&
				categorySearchText === ""
			) {
				// Cancel the current incomplete chip selection.
				e.preventDefault();
				const nextChips = chips.filter((c) => c.value !== "");
				setChips(nextChips);
				setDropdownMode({ type: "categories" });
				setCategorySearchText("");
				return;
			}

			// Tab completion: fill in the top typeahead match.
			if (
				e.key === "Tab" &&
				dropdownMode.type === "typeahead" &&
				typeaheadMatches.length > 0
			) {
				e.preventDefault();
				const match = typeaheadMatches[0];
				selectCategory(match.key);
				return;
			}

			if (!isOpen) {
				if (e.key === "ArrowDown" || e.key === "Enter") {
					e.preventDefault();
					openDropdown();
				}
				return;
			}

			const itemCount = navItems.length;

			// ArrowRight/ArrowLeft navigate horizontally in categories mode.
			const isNext =
				e.key === "ArrowDown" ||
				(e.key === "ArrowRight" && dropdownMode.type === "categories");
			const isPrev =
				e.key === "ArrowUp" ||
				(e.key === "ArrowLeft" && dropdownMode.type === "categories");

			if (isNext) {
				e.preventDefault();
				if (itemCount === 0) return;
				setIsKeyboardNav(true);
				setHighlightedIndex((prev) => (prev < 0 ? 0 : (prev + 1) % itemCount));
			} else if (isPrev) {
				e.preventDefault();
				if (itemCount === 0) return;
				setIsKeyboardNav(true);
				setHighlightedIndex((prev) =>
					prev < 0 ? itemCount - 1 : (prev - 1 + itemCount) % itemCount,
				);
			} else if (e.key === "Enter") {
				e.preventDefault();
				if (highlightedIndex < 0) {
					// No item highlighted — submit the current search.
					closeDropdown();
					internalRef.current?.blur();
					return;
				}
				const item = navItems[highlightedIndex];
				if (!item) return;
				if (item.kind === "category") {
					selectCategory(item.category.key);
				} else if (item.kind === "option") {
					selectOption(item.option);
				} else if (item.kind === "global") {
					selectOption(item.result.option, item.result.category.key);
				} else if (item.kind === "search") {
					selectSearchResult(item.result);
				}
			}
		},
		[
			isOpen,
			dropdownMode,
			freeformText,
			chips,
			categorySearchText,
			highlightedIndex,
			typeaheadMatches,
			closeDropdown,
			openDropdown,
			removeChip,
			selectCategory,
			selectOption,
			selectSearchResult,
			navItems,
		],
	);

	// ---- Scroll highlighted item into view ----
	useEffect(() => {
		if (!isOpen || highlightedIndex < 0) return;
		const el = document.getElementById(`filter-option-${highlightedIndex}`);
		if (el) {
			el.scrollIntoView({ block: "nearest" });
		}
	}, [highlightedIndex, isOpen]);

	// ---- Active input value ----
	const inputValue =
		dropdownMode.type === "options" ? categorySearchText : freeformText;

	// Whether the dropdown actually has visible content. When typing
	// freeform text that doesn't match any categories and results
	// haven't loaded yet, the Popover may be "open" but empty.
	const hasDropdownContent = useMemo(() => {
		if (dropdownMode.type === "categories") return true;
		if (dropdownMode.type === "options") return true;
		// Typeahead: has content if any section has items or is loading.
		return (
			filteredCategories.length > 0 ||
			globalResults.length > 0 ||
			searchResults.length > 0 ||
			isLoadingGlobal ||
			isLoadingSearch
		);
	}, [
		dropdownMode,
		filteredCategories,
		globalResults,
		searchResults,
		isLoadingGlobal,
		isLoadingSearch,
	]);

	// Whether to visually show the dropdown (suppress rounded corners
	// and bottom border on the input group).
	const showDropdown = isOpen && hasDropdownContent;

	const activeIncompleteChip = chips.find((c) => c.value === "");
	const activeCategoryLabel = activeIncompleteChip
		? (categoryMap.get(activeIncompleteChip.key)?.label ??
			activeIncompleteChip.key)
		: undefined;

	// Build the index mapping: rendered chip index -> original chips array
	// index. We only render complete chips, but removeChip needs the original
	// index.
	const completeChipEntries = useMemo(() => {
		const entries: Array<{ chip: FilterChip; originalIndex: number }> = [];
		for (let i = 0; i < chips.length; i++) {
			if (chips[i].value !== "") {
				entries.push({ chip: chips[i], originalIndex: i });
			}
		}
		return entries;
	}, [chips]);

	return (
		<div ref={containerRef} className={cn("relative rounded-md", className)}>
			{/* Input group */}
			{/* biome-ignore lint/a11y/useKeyWithClickEvents: Keyboard handled via input's onKeyDown. */}
			<div
				className={cn(
					"group/filter-search flex items-start w-full min-w-0 min-h-10 rounded-md border border-solid border-border bg-transparent transition-colors",
					!(isKeyboardNav && highlightedIndex >= 0) &&
						"has-[:focus]:ring-2 has-[:focus]:ring-content-link",
				)}
				onClick={() => {
					internalRef.current?.focus();
					if (!isOpen) {
						openDropdown();
					}
				}}
			>
				{/* Search icon */}
				<div className="flex shrink-0 items-center justify-center h-10 pl-3 pr-2 text-content-secondary">
					<SearchIcon className="size-4" />
				</div>
				{/* Chips + input area */}
				<div className="flex flex-1 flex-wrap items-center gap-1.5 min-w-0 py-1.5 cursor-text">
					{completeChipEntries.map(({ chip, originalIndex }) => {
						const cat = categoryMap.get(chip.key);
						return (
							<ChipBadge
								key={`${chip.key}-${chip.value}-${originalIndex}`}
								chip={chip}
								categoryLabel={cat?.label ?? chip.key}
								onRemove={() => removeChip(originalIndex)}
							/>
						);
					})}

					{/* Incomplete chip indicator */}
					{activeIncompleteChip && (
						<Badge
							variant="default"
							size="md"
							className="shrink-0 border border-dashed border-border-default"
						>
							<span className="text-content-secondary">
								{activeCategoryLabel}:
							</span>
						</Badge>
					)}

					{/* Text input */}
					<input
						ref={setRefs}
						type="text"
						tabIndex={0}
						className="flex-1 min-w-[40px] h-7 bg-transparent border-none outline-none text-sm text-content-primary placeholder:text-content-secondary"
						placeholder={chips.length === 0 ? placeholder : ""}
						value={inputValue}
						onChange={handleInputChange}
						onFocus={() => {
							if (!isOpen) {
								openDropdown();
							}
						}}
						onBlur={(e) => {
							const related = e.relatedTarget;
							if (containerRef.current?.contains(related)) {
								return;
							}
							closeDropdown();
						}}
						onKeyDown={handleKeyDown}
						role="combobox"
						aria-expanded={isOpen}
						aria-haspopup="listbox"
						aria-autocomplete="list"
						aria-controls={isOpen ? "filter-search-listbox" : undefined}
						aria-activedescendant={
							isOpen && highlightedIndex >= 0 && navItems.length > 0
								? `filter-option-${highlightedIndex}`
								: undefined
						}
						aria-label={ariaProps["aria-label"] ?? "Filter search"}
					/>
				</div>
				{/* ListFilter icon with left border */}
				<div
					className="flex shrink-0 items-center justify-center self-stretch pl-2 pr-3 border-0 border-l border-solid border-border text-content-secondary hover:text-content-primary transition-colors cursor-pointer"
					aria-hidden="true"
				>
					<ListFilter className="size-4" />
				</div>
			</div>

			{/* Dropdown — absolutely positioned so it floats over page content */}
			{showDropdown && (
				<div
					id="filter-search-listbox"
					role="listbox"
					aria-label="Filter options"
					className="absolute left-0 right-0 top-full z-50 max-h-80 overflow-y-auto rounded-md border border-solid border-border bg-surface-primary shadow-lg mt-1.5"
				>
					{/* Categories mode: show "Filter by" + category buttons */}
					{dropdownMode.type === "categories" && (
						<div className="p-3">
							<p
								role="presentation"
								className="text-xs font-medium text-content-secondary mb-2"
							>
								Filter by
							</p>
							<div className="flex flex-wrap gap-2">
								{categories.map((category, index) => (
									<CategoryButton
										key={category.key}
										category={category}
										id={`filter-option-${index}`}
										isHighlighted={highlightedIndex === index}
										showFocusRing={isKeyboardNav}
										onSelect={() => selectCategory(category.key)}
										onMouseEnter={() => {
											setHighlightedIndex(index);
											setIsKeyboardNav(false);
										}}
									/>
								))}
							</div>
						</div>
					)}
					{/* Options mode: show values for a selected category */}
					{dropdownMode.type === "options" && (
						<div>
							{isLoadingOptions ? (
								<div className="flex items-center justify-center py-6">
									<Spinner size="sm" loading />
								</div>
							) : categoryOptions.length === 0 ? (
								<p className="py-6 text-center text-sm text-content-secondary">
									No results found
								</p>
							) : (
								<div className="p-2">
									{categoryOptions.map((option, index) => (
										<OptionItem
											key={option.value}
											id={`filter-option-${index}`}
											option={option}
											isHighlighted={highlightedIndex === index}
											showFocusRing={isKeyboardNav}
											onSelect={() => selectOption(option)}
											onMouseEnter={() => {
												setHighlightedIndex(index);
												setIsKeyboardNav(false);
											}}
										/>
									))}
								</div>
							)}
						</div>
					)}
					{/* Typeahead mode: show category list + suggestions + search results */}
					{dropdownMode.type === "typeahead" && (
						<div>
							{/* Matching categories */}
							{filteredCategories.length > 0 && (
								<div className="p-2">
									{filteredCategories.map((category) => {
										const idx = navItems.findIndex(
											(n) =>
												n.kind === "category" &&
												n.category.key === category.key,
										);
										return (
											// biome-ignore lint/a11y/useKeyWithClickEvents: Keyboard navigation is handled by the parent input's onKeyDown handler.
											<div
												key={category.key}
												id={`filter-option-${idx}`}
												role="option"
												tabIndex={-1}
												aria-selected={highlightedIndex === idx}
												className={cn(
													"flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm cursor-pointer",
													"transition-colors font-medium",
													highlightedIndex === idx
														? "bg-surface-secondary text-content-primary"
														: "text-content-secondary",
													highlightedIndex === idx &&
														isKeyboardNav &&
														"ring-2 ring-content-link",
												)}
												onClick={() => selectCategory(category.key)}
												onMouseEnter={() => {
													setHighlightedIndex(idx);
													setIsKeyboardNav(false);
												}}
												onMouseDown={(e) => {
													e.preventDefault();
												}}
											>
												{category.icon && (
													<span className="flex shrink-0 items-center justify-center size-6 text-content-secondary">
														{category.icon}
													</span>
												)}
												<span>{category.label}</span>
											</div>
										);
									})}
								</div>
							)}
							{/* Suggestions */}
							{isLoadingGlobal &&
							isLoadingSearch &&
							globalResults.length === 0 &&
							searchResults.length === 0 ? (
								<div className="flex items-center justify-center py-4">
									<Spinner size="sm" loading />
								</div>
							) : (
								<>
									{globalResults.length > 0 && (
										<div className="px-2 pb-2 pt-1">
											<p
												id="filter-suggestions-label"
												className="text-xs font-medium text-content-secondary px-2 pb-1"
												role="presentation"
											>
												Filter suggestions
											</p>
											{globalResults.map((result, idx) => {
												const navIdx = filteredCategories.length + idx;
												return (
													<OptionItem
														key={`${result.category.key}-${result.option.value}`}
														id={`filter-option-${navIdx}`}
														option={result.option}
														isHighlighted={highlightedIndex === navIdx}
														showFocusRing={isKeyboardNav}
														categoryLabel={result.category.label}
														onSelect={() =>
															selectOption(result.option, result.category.key)
														}
														onMouseEnter={() => {
															setHighlightedIndex(navIdx);
															setIsKeyboardNav(false);
														}}
													/>
												);
											})}
										</div>
									)}
									{/* Resource search results */}
									{getSearchResults && (
										<SearchResultsSection
											searchResults={searchResults}
											isLoading={isLoadingSearch}
											isReady={searchResultsReady}
											highlightedIndex={highlightedIndex}
											showFocusRing={isKeyboardNav}
											navIndexOffset={
												filteredCategories.length + globalResults.length
											}
											onSelect={selectSearchResult}
											onHighlight={(idx) => {
												setHighlightedIndex(idx);
												setIsKeyboardNav(false);
											}}
										/>
									)}
								</>
							)}
						</div>
					)}
				</div>
			)}
		</div>
	);
};
