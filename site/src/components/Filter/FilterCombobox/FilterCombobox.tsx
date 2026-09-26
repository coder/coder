import { cn } from "cn";
import {
	CheckIcon,
	ChevronRightIcon,
	ListFilterIcon,
	SearchIcon,
} from "lucide-react";
import {
	type ReactNode,
	useEffect,
	useId,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { useQuery } from "react-query";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { ListFilterActiveIcon } from "#/components/Icons/ListFilterActiveIcon";
import {
	InputGroupAddon,
	InputGroupButton,
} from "#/components/InputGroup/InputGroup";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useDebouncedValue } from "#/hooks/debounce";
import { useMediaQuery } from "#/hooks/useMediaQuery";
import {
	coarsePointerMediaQuery,
	mobileViewportMediaQuery,
} from "#/utils/mobile";
import {
	chipDisplay,
	chipToken,
	filterOptionsByText,
	optionToken,
} from "./filterQuery";
import {
	chipRowItemHeightClassName,
	FilterComboboxChip,
	FilterComboboxChips,
	FilterComboboxChipsInput,
	FilterComboboxContent,
	FilterComboboxGroup,
	FilterComboboxInputGroup,
	FilterComboboxItem,
	FilterComboboxLabel,
	FilterComboboxList,
	FilterComboboxRoot,
	FilterComboboxStatus,
	isListNavigationKey,
	menuMaxHeightClassName,
} from "./primitives";
import { filterComboboxOptions, SEARCH_DEBOUNCE_MS } from "./queries";
import type { FilterCategory, FilterOption } from "./types";
import {
	noOptionMatchesMessage,
	optionsLoadErrorMessage,
	optionsLoadingMessage,
	SUGGESTIONS_ERROR_MESSAGE,
	useFilterCombobox,
} from "./useFilterCombobox";

/**
 * Delay before hovering another category swaps an open flyout, so a diagonal
 * move into the current flyout does not switch panels.
 */
const CATEGORY_HOVER_DELAY_MS = 300;

/**
 * Chip count at which Clear all appears. Fewer chips are quick to remove one
 * at a time.
 */
const CLEAR_ALL_MIN_CHIPS = 3;

const labelOnlyChipClassName =
	"text-content-primary [&_[data-slot=combobox-chip-remove]]:text-content-secondary";

const flyoutPanelClassName =
	"relative flex w-(--radix-popover-trigger-width) max-w-full shrink-0 flex-col rounded-md border border-border bg-surface-primary shadow-md sm:absolute sm:left-[calc(100%-0.25rem)] sm:z-10 sm:w-max sm:min-w-40 sm:self-start";

// While the menu is open on mobile the field leaves the page flow and pins
// below the navbar, so the software keyboard cannot squeeze the dropdown.
// `top-20` is the 72px navbar plus an 8px gap.
const mobileOverlayClassName =
	"fixed inset-x-6 top-20 z-30 w-auto bg-surface-primary";

type FilterComboboxProps = Readonly<{
	value: string;
	onChange: (query: string) => void;
	categories: readonly FilterCategory[];
	placeholder?: string;
	className?: string;
	/**
	 * Error to surface below the input (e.g. the server rejected the filter
	 * query). When set, the input is marked invalid and linked to the message.
	 */
	errorMessage?: string;
}>;

/**
 * Filter input that shows applied filters as chips and opens a cmdk menu. The
 * menu lists categories, which open as pointer flyouts or as keyboard and
 * mobile drill-ins; option rows for categories with `inlineOptions`; and
 * cross-category value suggestions for typed text. State lives in
 * `useFilterCombobox`.
 */
export function FilterCombobox({
	value,
	onChange,
	categories,
	placeholder = "Search and filter…",
	className,
	errorMessage,
}: FilterComboboxProps) {
	const {
		open,
		inputValue,
		typedFreeText,
		activeCategoryKey,
		activeCategory,
		activeOptions,
		activeOptionsError,
		statusMessage,
		listedCategories,
		categoriesNarrowedByText,
		scopeMatchKey,
		categoryPlaceholderCount,
		autoHighlight,
		unfilteredOptionsByKey,
		unfilteredOptionsErroredKeys,
		valueSuggestions,
		inlineSections,
		chipValues,
		highlightRef,
		scopeState,
		scopePillCategoryKey,
		showsOwnQueryKey,
		optionTokenFor,
		typeaheadError,
		actions,
	} = useFilterCombobox({
		value,
		onChange,
		categories,
	});
	const { setInputRef } = actions;

	const errorId = useId();
	const invalid = errorMessage !== undefined;
	const isCoarsePointer = useMediaQuery(coarsePointerMediaQuery);
	const isMobile = useMediaQuery(mobileViewportMediaQuery);
	const mobileOverlay = isMobile && open;
	// Category shown in the pointer flyout. Distinct from `activeCategoryKey`,
	// which is the committed drill-in state shared with keyboard navigation.
	// Reset whenever the menu opens or closes so a dismissed flyout does not
	// reappear next time.
	const [flyout, setFlyout] = useState<{
		categoryKey: string | null;
		openAtReset: boolean;
	}>({ categoryKey: null, openAtReset: open });
	if (flyout.openAtReset !== open) {
		setFlyout({ categoryKey: null, openAtReset: open });
	}
	const flyoutCategoryKey = flyout.categoryKey;
	const setFlyoutCategoryKey = (categoryKey: string | null) =>
		setFlyout({ categoryKey, openAtReset: open });
	// Highlighted category row, tracked here instead of the full highlight so
	// moving through option rows does not re-render the lists.
	const [highlightedCategoryKey, setHighlightedCategoryKey] = useState<
		string | null
	>(null);
	// A scope match shows its flyout only while its row is highlighted, and
	// never on coarse pointers.
	const shownFlyoutKey =
		flyoutCategoryKey ??
		(!isCoarsePointer &&
		scopeMatchKey !== null &&
		highlightedCategoryKey === scopeMatchKey
			? scopeMatchKey
			: null);
	const categoryRows = useRef(new Map<string, HTMLDivElement>());
	const hoverTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	const cancelHoverSwitch = () => {
		if (hoverTimeoutRef.current !== null) {
			clearTimeout(hoverTimeoutRef.current);
			hoverTimeoutRef.current = null;
		}
	};
	useEffect(() => cancelHoverSwitch, []);
	const updateFlyoutCategory = (
		categoryKey: string | null,
		immediate = false,
	) => {
		cancelHoverSwitch();
		if (
			categoryKey === null ||
			immediate ||
			flyoutCategoryKey === null ||
			flyoutCategoryKey === categoryKey
		) {
			setFlyoutCategoryKey(categoryKey);
			return;
		}
		hoverTimeoutRef.current = setTimeout(() => {
			setFlyoutCategoryKey(categoryKey);
			hoverTimeoutRef.current = null;
		}, CATEGORY_HOVER_DELAY_MS);
	};
	const registerCategoryRow = (key: string, element: HTMLDivElement | null) => {
		if (!element) {
			return;
		}
		categoryRows.current.set(key, element);
		return () => {
			categoryRows.current.delete(key);
		};
	};
	// Side panels align with the row that opened them. Measured after commit so
	// keyboard-entered categories line up too, not only pointer-hovered ones.
	const panelCategoryKey = activeCategoryKey ?? shownFlyoutKey;
	const [panelOffset, setPanelOffset] = useState(0);
	useLayoutEffect(() => {
		const row =
			panelCategoryKey === null
				? undefined
				: categoryRows.current.get(panelCategoryKey);
		// The main menu scrolls, so the row's position is measured within its
		// visible area.
		const scrollTop =
			row?.closest<HTMLElement>('[data-slot="filter-main-panel"]')?.scrollTop ??
			0;
		setPanelOffset(row ? Math.max(0, row.offsetTop - scrollTop) : 0);
	}, [panelCategoryKey]);
	// The flyout follows cmdk's highlight, which pointer and keyboard both move.
	// A highlight never opens a closed flyout; `shownFlyoutKey` handles the
	// scope match.
	const handleHighlightedValueChange = (
		highlighted: string,
		previous: string,
	) => {
		actions.onHighlightedValueChange(highlighted, previous);
		// A cleared highlight leaves the flyouts as they are.
		if (highlighted === "") {
			return;
		}
		const isCategoryRow = listedCategories.some(
			(category) => category.key === highlighted,
		);
		setHighlightedCategoryKey(isCategoryRow ? highlighted : null);
		// A flyout whose row was hidden is closed.
		const flyoutOpen = listedCategories.some(
			(category) => category.key === flyoutCategoryKey,
		);
		if (!flyoutOpen || highlighted === flyoutCategoryKey) {
			return;
		}
		updateFlyoutCategory(isCategoryRow ? highlighted : null);
	};
	// Typed text narrows the category rows, so a click enters the category and
	// drops the text, and only a scope match gets a flyout. Enter on a row
	// listed only by the scope match applies the text as a search instead.
	// The flyout renders only on wider viewports.
	const flyoutOptions = useFlyoutOptions(
		activeCategoryKey === null &&
			!isMobile &&
			(!categoriesNarrowedByText || shownFlyoutKey === scopeMatchKey)
			? listedCategories.find((category) => category.key === shownFlyoutKey)
			: undefined,
		unfilteredOptionsByKey,
		unfilteredOptionsErroredKeys,
		actions.retryUnfilteredOptions,
	);
	// The hook announces its own states; the flyout is view state, so its load
	// state joins the announcement here.
	const flyoutLoadMessage = !flyoutOptions
		? undefined
		: flyoutOptions.loading
			? optionsLoadingMessage(flyoutOptions.category.label)
			: flyoutOptions.failed
				? optionsLoadErrorMessage(flyoutOptions.category.label)
				: flyoutOptions.emptyMessage
					? noOptionMatchesMessage(flyoutOptions.category.label)
					: undefined;
	// Toggling clears text typed to find the category, so the flyout is pinned
	// open explicitly rather than through the scope match.
	const toggleFlyoutScope = (categoryKey: string) => {
		setFlyoutCategoryKey(categoryKey);
		actions.toggleScope(categoryKey, { clearCategorySearch: true });
	};
	const liveRegionMessage = [flyoutLoadMessage, statusMessage]
		.filter(Boolean)
		.join(" ");
	const selectFlyoutOption = (token: string) => {
		actions.toggleCategoryOption(token);
		updateFlyoutCategory(null);
		actions.focusInput();
	};
	const selectCategory = (categoryKey: string) => {
		updateFlyoutCategory(null);
		actions.selectCategory(categoryKey);
	};

	const mainPanelEmpty =
		listedCategories.length === 0 &&
		categoryPlaceholderCount === 0 &&
		valueSuggestions.length === 0 &&
		inlineSections.length === 0 &&
		!typeaheadError;

	const mainPanelProps = {
		listedCategories,
		categoryPlaceholderCount,
		valueSuggestions,
		inlineSections,
		typeaheadError,
		registerCategoryRow,
		onSelectCategory: selectCategory,
		onOpenFlyout: updateFlyoutCategory,
		onToggleInlineOption: actions.toggleInlineOption,
		onSelectSuggestion: actions.toggleValueSuggestion,
		onRetry: actions.retryTypeahead,
		onRetryInlineOptions: actions.retryUnfilteredOptions,
	};
	const categoryOptionsList =
		activeCategoryKey === null ? undefined : (
			<CategoryOptionsList
				embedded={isMobile}
				offset={panelOffset}
				category={activeCategory}
				options={activeOptions}
				optionsError={activeOptionsError}
				unfilteredOptionCount={
					unfilteredOptionsByKey.get(activeCategoryKey)?.length
				}
				selectedTokens={chipValues}
				optionTokenFor={(option) => optionTokenFor(activeCategoryKey, option)}
				scope={scopeState(activeCategoryKey)}
				onToggleScope={actions.toggleScope}
				searchValue={inputValue}
				onSearchChange={actions.onInputValueChange}
				onRetry={actions.retryActiveOptions}
				onSelectOption={(token) => {
					actions.toggleCategoryOption(token);
					// The search field or a clicked row may hold focus and unmount
					// with the list. On mobile, refocusing would reopen the keyboard.
					if (!isMobile) {
						actions.focusInput();
					}
				}}
			/>
		);

	return (
		<>
			<FilterComboboxRoot
				open={open}
				autoHighlight={autoHighlight}
				onDismiss={actions.dismiss}
				onRemoveValue={actions.removeChip}
				inputValue={inputValue}
				onInputValueChange={actions.onInputValueChange}
				highlightRef={highlightRef}
				onHighlightedValueChange={handleHighlightedValueChange}
				label={placeholder}
				className={cn(mobileOverlay && "min-h-10")}
			>
				<FilterComboboxInputGroup
					className={cn(className, mobileOverlay && mobileOverlayClassName)}
				>
					<InputGroupAddon className="self-stretch items-start border-0 border-r border-solid border-border p-0">
						<InputGroupButton
							type="button"
							variant="subtle"
							aria-label="Filters"
							aria-expanded={open}
							aria-haspopup="listbox"
							className={cn(
								"h-9.5 min-w-0 shrink-0 rounded-none rounded-l-md pl-2.5 pr-3 text-sm [&>svg]:p-0",
								chipValues.length > 0 && "text-content-primary",
							)}
							onMouseDown={(event) => {
								// Keeps focus in the combobox input, which both menu
								// actions focus, so aria-activedescendant navigation works.
								event.preventDefault();
							}}
							onClick={(event) => {
								// Keyboard and assistive-technology activation (detail 0)
								// only opens the menu; pointer clicks toggle it.
								if (event.detail === 0) {
									actions.showAllFilters();
									return;
								}
								actions.toggleMenu();
							}}
						>
							<span className="flex size-5 shrink-0 items-center justify-center">
								{chipValues.length > 0 ? (
									<ListFilterActiveIcon
										aria-hidden
										data-testid="filter-active-icon"
										className="size-5!"
									/>
								) : (
									<ListFilterIcon aria-hidden className="size-icon-sm" />
								)}
							</span>
						</InputGroupButton>
					</InputGroupAddon>
					<InputGroupAddon className="h-9.5 self-start px-2">
						<SearchIcon aria-hidden className="size-icon-sm" />
					</InputGroupAddon>
					<FilterComboboxChips>
						{chipValues.map((token) => {
							const display = chipDisplay(
								token,
								showsOwnQueryKey(token) ? [] : categories,
							);
							const category = categories.find(
								(entry) => entry.key === display.key,
							);
							const pillToggle =
								category && scopePillCategoryKey(token) === category.key
									? category.scopeToggle
									: undefined;
							const labelOnly = category?.chipLabelOnly === true;
							const labelOption =
								labelOnly && category
									? unfilteredOptionsByKey
											.get(category.key)
											?.find(
												(option) => optionToken(category.key, option) === token,
											)
									: undefined;
							// Applied tokens read as query syntax, so they are always
							// lowercase even when the menu shows a display label.
							const prefix = (labelOnly ? "" : display.key).toLowerCase();
							const value = (
								labelOption?.appliedLabel ??
								labelOption?.label ??
								display.value
							).toLowerCase();
							const displayText = prefix ? chipToken(prefix, value) : value;
							return (
								<span key={token} className="inline-flex min-w-0 max-w-full">
									<FilterComboboxChip
										value={token}
										removeLabel={`Remove ${displayText}`}
										className={cn(
											labelOnly && labelOnlyChipClassName,
											pillToggle && "rounded-r-none",
										)}
									>
										<ChipLabel prefix={prefix} value={value} />
									</FilterComboboxChip>
									{/* Joined to its chip, since it widens that chip's filter. */}
									{category && pillToggle && (
										<FilterComboboxChip
											removeLabel={pillToggle.pillRemoveLabel(value)}
											onRemove={(event) => {
												// The pill unmounts, so keyboard removal keeps focus
												// in the search input.
												if (event.detail === 0) {
													actions.focusInput();
												}
												actions.removeScopePill(category.key);
											}}
											// Only the pill shrinks, so the pair never overflows the field.
											className="min-w-0 rounded-l-none border-l-surface-primary"
										>
											<Tooltip>
												<TooltipTrigger asChild>
													<span className="min-w-0 truncate">
														{`${pillToggle.pillPrefix} `}
														<span className="text-content-primary">
															{value}
														</span>
													</span>
												</TooltipTrigger>
												<TooltipContent className="max-w-64 text-balance">
													{scopeState(category.key)?.label ?? ""}
												</TooltipContent>
											</Tooltip>
										</FilterComboboxChip>
									)}
								</span>
							);
						})}
						{activeCategory && typedFreeText.length > 0 && (
							<Badge
								variant="outline"
								size="md"
								data-slot="combobox-chip-search"
								className={cn(chipRowItemHeightClassName, "px-2 font-medium")}
							>
								{typedFreeText}
							</Badge>
						)}
						{/* Decorative draft prefix: the live region already announces
							    "Filtering by <category>", so this stays hidden. */}
						{activeCategory && (
							<Badge
								variant="dashed"
								size="md"
								data-slot="combobox-chip-draft"
								className={cn(chipRowItemHeightClassName, "px-2 font-medium")}
								aria-hidden
							>
								{`${activeCategory.key}:`}
							</Badge>
						)}
						<FilterComboboxChipsInput
							className="focus:placeholder:text-transparent"
							ref={setInputRef}
							aria-label={placeholder}
							aria-invalid={invalid || undefined}
							aria-errormessage={invalid ? errorId : undefined}
							placeholder={
								open || chipValues.length > 0 || activeCategory
									? ""
									: placeholder
							}
							onFocus={actions.onInputFocus}
							// A click on the already focused input reopens a dismissed menu.
							onClick={actions.onInputFocus}
							onKeyDown={actions.onInputKeyDown}
						/>
						{chipValues.length >= CLEAR_ALL_MIN_CHIPS && (
							<Badge
								asChild
								variant="outline"
								hover
								className={cn(
									chipRowItemHeightClassName,
									"px-2 font-medium text-content-secondary hover:text-content-primary",
								)}
							>
								<button
									type="button"
									// A click does not move focus to the button, so a focused
									// input keeps it.
									onMouseDown={(event) => event.preventDefault()}
									onClick={(event) => {
										// The button unmounts, as does an open category's search
										// field on wider viewports, so focus either held moves to
										// the input instead of the page body.
										const hadFocus =
											document.activeElement === event.currentTarget;
										actions.clearAll();
										if (hadFocus || (activeCategoryKey !== null && !isMobile)) {
											actions.focusInput();
										}
									}}
								>
									Clear all
								</button>
							</Badge>
						)}
					</FilterComboboxChips>
				</FilterComboboxInputGroup>
				<FilterComboboxContent
					align="start"
					side="bottom"
					avoidCollisions={!isMobile}
					className="relative left-0 w-(--radix-popover-trigger-width) max-w-(--radix-popper-available-width) overflow-visible border-0 bg-transparent shadow-none data-[state=closed]:hidden sm:w-fit"
				>
					{/* Keep mounted so polite status announcements stay consistent. */}
					<FilterComboboxStatus>{liveRegionMessage}</FilterComboboxStatus>
					{isMobile ? (
						(activeCategoryKey !== null || !mainPanelEmpty) && (
							<div
								data-slot="filter-mobile-panel"
								className={cn(
									menuMaxHeightClassName,
									"flex w-(--radix-popover-trigger-width) max-w-full min-h-0 flex-col overflow-hidden rounded-md border border-border bg-surface-primary p-2 shadow-md",
								)}
							>
								{categoryOptionsList ?? (
									<MainPanel embedded drillIn {...mainPanelProps} />
								)}
							</div>
						)
					) : (
						<div
							className="relative flex items-start gap-1 overflow-visible"
							onMouseLeave={() => {
								updateFlyoutCategory(null);
								setHighlightedCategoryKey(null);
								actions.setHighlightedValue("");
							}}
						>
							{!mainPanelEmpty && (
								<MainPanel
									drillIn={
										isCoarsePointer ||
										activeCategoryKey !== null ||
										categoriesNarrowedByText
									}
									{...mainPanelProps}
								/>
							)}
							{categoryOptionsList ??
								(flyoutOptions && (
									<FlyoutCategoryPanel
										key={flyoutOptions.category.key}
										offset={panelOffset}
										flyout={flyoutOptions}
										selectedTokens={chipValues}
										optionTokenFor={(option) =>
											optionTokenFor(flyoutOptions.category.key, option)
										}
										scope={scopeState(flyoutOptions.category.key)}
										onToggleScope={toggleFlyoutScope}
										onMouseEnter={cancelHoverSwitch}
										onSelectOption={selectFlyoutOption}
									/>
								))}
						</div>
					)}
				</FilterComboboxContent>
			</FilterComboboxRoot>
			{invalid && (
				<span
					id={errorId}
					role="alert"
					className="text-sm text-content-destructive"
				>
					{errorMessage}
				</span>
			)}
		</>
	);
}

const OPTION_ITEM_CLASS = "min-h-8.5 gap-2 px-2 py-1.25";
const INLINE_GROUP_CLASS =
	"mt-2 border-t border-border pt-2 first:mt-0 first:border-t-0 first:pt-0";

// Categories with more options than this get a search field in their panel.
export const SEARCHABLE_OPTION_COUNT = 10;

const categoryLoadErrorMessage = (category: FilterCategory | undefined) =>
	optionsLoadErrorMessage(category ? category.label : "filter");

type LoadErrorProps = Readonly<{ message: string; onRetry: () => void }>;

function LoadError({ message, onRetry }: LoadErrorProps) {
	return (
		<div className="flex flex-col items-center gap-2 px-2 py-2.5 text-center text-sm text-content-secondary">
			<span>{message}</span>
			<Button size="sm" variant="outline" onClick={onRetry}>
				Retry
			</Button>
		</div>
	);
}

function EmptyOptions({ message }: { message: string }) {
	return (
		<div className="px-2 py-1.5 text-sm text-content-secondary">{message}</div>
	);
}

function LoadingOptions() {
	return (
		<div className="flex justify-center px-2 py-2.5">
			<Spinner loading size="sm" />
		</div>
	);
}

// Fixed 24px slot so icons, avatars, and status dots of different sizes align.
function OptionIcon({ children }: { children: ReactNode }): ReactNode {
	return (
		<span
			aria-hidden
			className="flex size-6 shrink-0 items-center justify-center"
		>
			{children}
		</span>
	);
}

type OptionRowContentProps = Readonly<{
	icon?: ReactNode;
	label: ReactNode;
	selected: boolean;
}>;

function OptionRowContent({ icon, label, selected }: OptionRowContentProps) {
	return (
		<>
			{icon ? <OptionIcon>{icon}</OptionIcon> : null}
			<span>{label}</span>
			{selected && (
				<CheckIcon aria-hidden className="ml-auto size-4 shrink-0" />
			)}
		</>
	);
}

function ChipLabel({
	prefix,
	value,
}: {
	prefix: string;
	value: string;
}): ReactNode {
	if (prefix.length === 0) {
		return value;
	}
	return (
		<>
			<span className="text-content-secondary group-hover/chip:text-content-primary">
				{prefix}:
			</span>
			<span className="text-content-primary">{value}</span>
		</>
	);
}

type InlineSection = {
	category: FilterCategory;
	heading: string;
	rows: readonly {
		token: string;
		selected: boolean;
		showIcon: boolean;
		option: FilterOption;
	}[];
	status: "ready" | "loading" | "failed";
	loadRowValue: string;
};

type ValueSuggestion = {
	categoryLabel: string;
	selected: boolean;
	token: string;
	option: Pick<FilterOption, "label" | "startIcon">;
};

const groupByCategoryLabel = <T extends { categoryLabel: string }>(
	items: readonly T[],
): Array<[string, T[]]> => {
	const groups = new Map<string, T[]>();
	for (const item of items) {
		const group = groups.get(item.categoryLabel);
		if (group) {
			group.push(item);
		} else {
			groups.set(item.categoryLabel, [item]);
		}
	}
	return [...groups];
};

type MainPanelProps = Readonly<{
	listedCategories: readonly FilterCategory[];
	/** Nonzero while the category list is still unknown. */
	categoryPlaceholderCount: number;
	valueSuggestions: readonly ValueSuggestion[];
	inlineSections: readonly InlineSection[];
	typeaheadError: boolean;
	embedded?: boolean;
	/** Clicking a category enters it instead of opening its pointer flyout. */
	drillIn: boolean;
	registerCategoryRow: (
		key: string,
		element: HTMLDivElement | null,
	) => (() => void) | undefined;
	onSelectCategory: (categoryKey: string) => void;
	/**
	 * Opens a category's flyout. Switching from another open flyout waits
	 * `CATEGORY_HOVER_DELAY_MS` unless `immediate`.
	 */
	onOpenFlyout: (categoryKey: string, immediate?: boolean) => void;
	onToggleInlineOption: (token: string) => void;
	onSelectSuggestion: (token: string) => void;
	onRetry: () => void;
	onRetryInlineOptions: (categoryKey: string) => void;
}>;

function MainPanel({
	listedCategories,
	categoryPlaceholderCount,
	valueSuggestions,
	inlineSections,
	typeaheadError,
	embedded = false,
	drillIn,
	registerCategoryRow,
	onSelectCategory,
	onOpenFlyout,
	onToggleInlineOption,
	onSelectSuggestion,
	onRetry,
	onRetryInlineOptions,
}: MainPanelProps) {
	return (
		<FilterComboboxList
			data-slot="filter-main-panel"
			className={cn(
				menuMaxHeightClassName,
				"w-(--radix-popover-trigger-width) max-w-full shrink-0 rounded-md border border-border bg-surface-primary p-2 shadow-md sm:w-64",
				embedded &&
					"w-full rounded-none border-0 bg-transparent p-0 shadow-none",
			)}
			aria-busy={categoryPlaceholderCount > 0 || undefined}
		>
			{Array.from({ length: categoryPlaceholderCount }, (_, index) => (
				<div
					key={`placeholder-${index}`}
					aria-hidden
					data-slot="category-placeholder"
					className={cn(OPTION_ITEM_CLASS, "flex items-center")}
				>
					<Skeleton className="size-4 shrink-0" />
					<Skeleton variant="text" className="w-24" />
				</div>
			))}
			{listedCategories.map((category) => (
				<FilterComboboxItem
					ref={(element) => registerCategoryRow(category.key, element)}
					className={OPTION_ITEM_CLASS}
					key={category.key}
					value={category.key}
					onMouseEnter={() => onOpenFlyout(category.key)}
					onSelect={() => {
						if (drillIn) {
							onSelectCategory(category.key);
							return;
						}
						onOpenFlyout(category.key, true);
					}}
				>
					{category.icon && <OptionIcon>{category.icon}</OptionIcon>}
					<span className="shrink-0">{category.label}</span>
					<ChevronRightIcon aria-hidden className="ml-auto shrink-0" />
				</FilterComboboxItem>
			))}
			{inlineSections.map(
				({ category, heading, rows, status, loadRowValue }) => (
					<FilterComboboxGroup
						className={INLINE_GROUP_CLASS}
						key={category.key}
					>
						<FilterComboboxLabel className="pt-0 opacity-80">
							{heading}
						</FilterComboboxLabel>
						{status === "failed" && (
							<EmptyOptions message={categoryLoadErrorMessage(category)} />
						)}
						{/* One row for loading and Retry keeps cmdk's highlight on it while
					    a Retry runs. */}
						{status !== "ready" && (
							<FilterComboboxItem
								className={cn(
									OPTION_ITEM_CLASS,
									status === "loading" && "justify-center",
								)}
								value={loadRowValue}
								onSelect={() => {
									if (status === "failed") {
										onRetryInlineOptions(category.key);
									}
								}}
							>
								{status === "failed" ? (
									"Retry"
								) : (
									<>
										<Spinner loading size="sm" aria-hidden />
										<span className="sr-only">
											{optionsLoadingMessage(category.label)}
										</span>
									</>
								)}
							</FilterComboboxItem>
						)}
						{rows.map(({ token, option, selected, showIcon }) => (
							<FilterComboboxItem
								className={cn(
									OPTION_ITEM_CLASS,
									(!showIcon || selected) && "text-content-primary",
								)}
								key={token}
								value={token}
								onSelect={() => onToggleInlineOption(token)}
							>
								<OptionRowContent
									icon={showIcon ? option.startIcon : undefined}
									label={option.label}
									selected={selected}
								/>
							</FilterComboboxItem>
						))}
					</FilterComboboxGroup>
				),
			)}
			{groupByCategoryLabel(valueSuggestions).map(
				([categoryLabel, suggestions]) => (
					<FilterComboboxGroup key={categoryLabel}>
						<FilterComboboxLabel>{categoryLabel}</FilterComboboxLabel>
						{suggestions.map((suggestion) => (
							<FilterComboboxItem
								className={cn(
									OPTION_ITEM_CLASS,
									suggestion.selected && "text-content-primary",
								)}
								key={suggestion.token}
								value={suggestion.token}
								onSelect={() => onSelectSuggestion(suggestion.token)}
							>
								<OptionRowContent
									icon={suggestion.option.startIcon}
									label={suggestion.option.label}
									selected={suggestion.selected}
								/>
							</FilterComboboxItem>
						))}
					</FilterComboboxGroup>
				),
			)}
			{typeaheadError && (
				<LoadError message={SUGGESTIONS_ERROR_MESSAGE} onRetry={onRetry} />
			)}
		</FilterComboboxList>
	);
}

type FlyoutSearchProps = Readonly<{
	label: string;
	value: string;
	onChange: (value: string) => void;
	navigatesList: boolean;
}>;

function FlyoutSearch({
	label,
	value,
	onChange,
	navigatesList,
}: FlyoutSearchProps) {
	return (
		<div className="-mx-2 -mt-2 mb-2 flex items-center border-b border-border px-3">
			<SearchIcon
				aria-hidden
				className="mr-2 size-icon-sm shrink-0 opacity-50"
			/>
			<input
				aria-label={`Search ${label}`}
				className="h-10 w-full border-0 bg-transparent py-3 text-sm text-content-primary outline-hidden placeholder:text-content-secondary"
				placeholder={`Search ${label.toLowerCase()}…`}
				value={value}
				onChange={(event) => onChange(event.currentTarget.value)}
				onKeyDown={(event) => {
					const reachesList =
						event.key === "Enter" || isListNavigationKey(event);
					if (navigatesList && reachesList) {
						return;
					}
					event.stopPropagation();
				}}
			/>
		</div>
	);
}

type ScopeState = Readonly<{
	widened: boolean;
	label: string;
	disabled: boolean;
}>;

type FlyoutScopeToggleProps = Readonly<{
	categoryKey: string;
	label: string;
	checked: boolean;
	disabled: boolean;
	navigatesList: boolean;
	onToggle: (categoryKey: string) => void;
}>;

function FlyoutScopeToggle({
	categoryKey,
	label,
	checked,
	disabled,
	navigatesList,
	onToggle,
}: FlyoutScopeToggleProps) {
	const id = useId();
	return (
		<div className="-mx-2 -mb-2 mt-2 flex items-start gap-2 border-t border-border px-3 py-2.5">
			<Switch
				id={id}
				size="sm"
				className="shrink-0"
				checked={checked}
				disabled={disabled}
				onCheckedChange={() => onToggle(categoryKey)}
				// Keep focus in the combobox input so keyboard navigation continues.
				onMouseDown={(event) => event.preventDefault()}
				onKeyDown={(event) => {
					if (
						navigatesList &&
						(event.key === "ArrowUp" || event.key === "ArrowDown")
					) {
						// Return focus to the combobox input; the key still reaches
						// cmdk, which moves the highlight into the options.
						event.currentTarget
							.closest("[cmdk-root]")
							?.querySelector<HTMLInputElement>("[cmdk-input]")
							?.focus();
						return;
					}
					// cmdk would otherwise take Enter to pick the highlighted
					// option instead of toggling the switch.
					event.stopPropagation();
				}}
			/>
			{/* Zero basis so the label wraps to the panel width set by the list. */}
			<label
				htmlFor={id}
				className="w-0 min-w-0 flex-1 text-xs text-content-secondary"
			>
				{label}
			</label>
		</div>
	);
}

type OptionsPanelProps = Readonly<{
	category: FilterCategory | undefined;
	/** Rendered inside the mobile panel instead of beside the main menu. */
	embedded?: boolean;
	offset: number;
	/** Shown above the options when the category is searchable. */
	search?: { value: string; onChange: (value: string) => void };
	/**
	 * The options are combobox rows, so list navigation keys and Enter reach
	 * them from the panel's own controls instead of stopping there.
	 */
	navigatesList?: boolean;
	emptyMessage?: string;
	scope: ScopeState | undefined;
	onToggleScope: (categoryKey: string) => void;
	onMouseEnter?: () => void;
	children: ReactNode;
}>;

function OptionsPanel({
	category,
	embedded = false,
	offset,
	search,
	navigatesList = false,
	emptyMessage,
	scope,
	onToggleScope,
	onMouseEnter,
	children,
}: OptionsPanelProps) {
	// A searchable panel is pinned to the top so its search field stays put.
	const top = search ? 0 : offset;
	return (
		<div
			onMouseEnter={onMouseEnter}
			className={cn(
				flyoutPanelClassName,
				"p-2",
				scope && "sm:min-w-60",
				embedded &&
					"min-h-0 flex-1 w-full rounded-none border-0 bg-transparent p-0 shadow-none",
			)}
			style={
				embedded
					? undefined
					: {
							top,
							maxHeight: `min(20rem, calc(var(--radix-popper-available-height) - ${top}px))`,
						}
			}
		>
			{search && category && (
				<FlyoutSearch
					label={category.label}
					value={search.value}
					onChange={search.onChange}
					navigatesList={navigatesList}
				/>
			)}
			{children}
			{emptyMessage && <EmptyOptions message={emptyMessage} />}
			{category && scope && (
				<FlyoutScopeToggle
					categoryKey={category.key}
					label={scope.label}
					checked={scope.widened}
					disabled={scope.disabled}
					navigatesList={navigatesList}
					onToggle={onToggleScope}
				/>
			)}
		</div>
	);
}

// The shown flyout's search and load state, or undefined when no flyout is
// shown. FilterCombobox owns it so the live region announces what the panel
// shows.
const useFlyoutOptions = (
	category: FilterCategory | undefined,
	optionsByKey: ReadonlyMap<string, readonly FilterOption[]>,
	erroredKeys: ReadonlySet<string>,
	retryOptions: (categoryKey: string) => void,
) => {
	const categoryKey = category?.key;
	// Showing another flyout, or none, clears the search.
	const [search, setSearch] = useState({ categoryKey, query: "" });
	if (search.categoryKey !== categoryKey) {
		setSearch({ categoryKey, query: "" });
	}
	const query = search.categoryKey === categoryKey ? search.query : "";
	const setQuery = (next: string) => setSearch({ categoryKey, query: next });
	const trimmedQuery = query.trim();
	// `getOptions` may return only the first page for an empty query, so a
	// typed search calls `getOptions(query)` after the debounce. Until those
	// results arrive, the unfiltered list is filtered locally. The search runs
	// only once the debounced state is the current one, so text debounced for
	// one flyout never reaches another flyout's loader.
	const debouncedSearch = useDebouncedValue(search, SEARCH_DEBOUNCE_MS);
	const debouncedQuery =
		debouncedSearch === search && search.categoryKey === categoryKey
			? trimmedQuery
			: "";
	const searchResults = useQuery(
		filterComboboxOptions(
			categoryKey ?? "",
			category?.getOptions,
			debouncedQuery,
			category !== undefined && debouncedQuery.length > 0,
		),
	);
	if (!category) {
		return undefined;
	}
	const options = optionsByKey.get(category.key);
	const optionsError = erroredKeys.has(category.key);
	const normalized = trimmedQuery.toLowerCase();
	const searchSettled = debouncedQuery === trimmedQuery;
	const searchFailed =
		normalized.length > 0 && searchSettled && searchResults.isError;
	const filteredOptions =
		options === undefined
			? []
			: normalized.length === 0
				? options
				: searchSettled && searchResults.data
					? searchResults.data
					: filterOptionsByText(options, normalized);
	const loading = options === undefined && !optionsError;
	const failed = optionsError || searchFailed;
	return {
		category,
		query,
		setQuery,
		filteredOptions,
		searchable: (options?.length ?? 0) > SEARCHABLE_OPTION_COUNT,
		loading,
		failed,
		retry: optionsError
			? () => retryOptions(category.key)
			: () => void searchResults.refetch(),
		emptyMessage:
			loading || failed || filteredOptions.length > 0
				? undefined
				: normalized.length > 0
					? "No matching options"
					: "No options",
	};
};

type FlyoutCategoryPanelProps = Readonly<{
	offset: number;
	flyout: NonNullable<ReturnType<typeof useFlyoutOptions>>;
	selectedTokens: readonly string[];
	/** Token an option commits, or the applied chip it removes. */
	optionTokenFor: (option: FilterOption) => string;
	scope: ScopeState | undefined;
	onToggleScope: (categoryKey: string) => void;
	onMouseEnter: () => void;
	onSelectOption: (token: string) => void;
}>;

function FlyoutCategoryPanel({
	offset,
	flyout,
	selectedTokens,
	optionTokenFor,
	scope,
	onToggleScope,
	onMouseEnter,
	onSelectOption,
}: FlyoutCategoryPanelProps) {
	const {
		category,
		query,
		setQuery,
		filteredOptions,
		searchable,
		loading,
		failed,
		retry,
		emptyMessage,
	} = flyout;

	return (
		<OptionsPanel
			category={category}
			offset={offset}
			search={searchable ? { value: query, onChange: setQuery } : undefined}
			emptyMessage={emptyMessage}
			scope={scope}
			onToggleScope={onToggleScope}
			onMouseEnter={onMouseEnter}
		>
			{loading && <LoadingOptions />}
			{failed && (
				<LoadError
					message={categoryLoadErrorMessage(category)}
					onRetry={retry}
				/>
			)}
			<div className="min-h-0 flex-1 overflow-y-auto overscroll-contain pr-1">
				{(failed ? [] : filteredOptions).map((option) => {
					const token = optionTokenFor(option);
					const selected = selectedTokens.includes(token);
					return (
						<button
							className={cn(
								"flex min-h-8.5 w-full items-center gap-2 rounded-sm px-2 py-1.25 text-left text-sm font-normal text-nowrap text-content-secondary hover:bg-surface-secondary hover:text-content-primary",
								selected && "text-content-primary",
							)}
							key={token}
							type="button"
							// Keep focus in the combobox input so keyboard navigation continues.
							onMouseDown={(event) => event.preventDefault()}
							onClick={() => onSelectOption(token)}
						>
							<OptionRowContent
								icon={option.startIcon}
								label={option.label}
								selected={selected}
							/>
						</button>
					);
				})}
			</div>
		</OptionsPanel>
	);
}

type CategoryOptionsListProps = Readonly<{
	/** Mobile drill-in: fills the panel and uses the main input as search. */
	embedded: boolean;
	offset: number;
	category: FilterCategory | undefined;
	options: readonly FilterOption[] | undefined;
	optionsError: boolean;
	/** Size of the category's unfiltered option list, when cached. */
	unfilteredOptionCount: number | undefined;
	selectedTokens: readonly string[];
	/** Token an option commits, or the applied chip it removes. */
	optionTokenFor: (option: FilterOption) => string;
	scope: ScopeState | undefined;
	onToggleScope: (categoryKey: string) => void;
	searchValue: string;
	onSearchChange: (value: string) => void;
	onRetry: () => void;
	onSelectOption: (token: string) => void;
}>;

function CategoryOptionsList({
	embedded,
	offset,
	category,
	options,
	optionsError,
	unfilteredOptionCount,
	selectedTokens,
	optionTokenFor,
	scope,
	onToggleScope,
	searchValue,
	onSearchChange,
	onRetry,
	onSelectOption,
}: CategoryOptionsListProps) {
	if (optionsError) {
		return (
			<OptionsPanel
				category={category}
				embedded={embedded}
				offset={offset}
				scope={scope}
				onToggleScope={onToggleScope}
			>
				<LoadError
					message={categoryLoadErrorMessage(category)}
					onRetry={onRetry}
				/>
			</OptionsPanel>
		);
	}

	// Decided from the unfiltered list so the search field stays mounted while
	// results load or shrink.
	const searchable =
		!embedded &&
		(unfilteredOptionCount ?? options?.length ?? 0) > SEARCHABLE_OPTION_COUNT;

	return (
		<OptionsPanel
			category={category}
			embedded={embedded}
			offset={offset}
			search={
				searchable
					? { value: searchValue, onChange: onSearchChange }
					: undefined
			}
			navigatesList
			emptyMessage={
				options?.length !== 0
					? undefined
					: searchValue.trim().length > 0
						? "No matching options"
						: "No options"
			}
			scope={scope}
			onToggleScope={onToggleScope}
		>
			{options === undefined && <LoadingOptions />}
			<FilterComboboxList className="min-h-0 flex-1 touch-pan-y overflow-y-auto overscroll-contain p-0 pr-1">
				{options?.map((option) => {
					const token = optionTokenFor(option);
					const selected = selectedTokens.includes(token);
					return (
						<FilterComboboxItem
							className={cn(
								OPTION_ITEM_CLASS,
								selected && "text-content-primary",
							)}
							key={token}
							value={token}
							onSelect={() => onSelectOption(token)}
						>
							<OptionRowContent
								icon={option.startIcon}
								label={option.label}
								selected={selected}
							/>
						</FilterComboboxItem>
					);
				})}
			</FilterComboboxList>
		</OptionsPanel>
	);
}
