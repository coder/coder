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
import { chipDisplay, chipToken } from "./filterQuery";
import {
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
} from "./primitives";
import { filterComboboxOptions, SEARCH_DEBOUNCE_MS } from "./queries";
import type { FilterCategory, FilterOption } from "./types";
import { useFilterCombobox } from "./useFilterCombobox";

/**
 * Unified workspace filter input: renders committed chips plus a cmdk-driven
 * popup that browses categories and surfaces cross-category value suggestions.
 * State lives in `useFilterCombobox`.
 */
const CATEGORY_HOVER_DELAY_MS = 300;

const labelOnlyChipClassName =
	"text-content-primary [&_[data-slot=combobox-chip-remove]]:text-content-secondary";

const flyoutPanelClassName =
	"relative flex w-(--radix-popover-trigger-width) max-w-full shrink-0 flex-col rounded-md border border-border bg-surface-primary shadow-md sm:absolute sm:left-[calc(100%-0.25rem)] sm:z-10 sm:w-max sm:min-w-40 sm:self-start";

// While the menu is open on mobile the field leaves the page flow and pins
// below the navbar, so the software keyboard cannot squeeze the dropdown.
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
		committedFreeText,
		activeCategoryKey,
		activeCategory,
		activeOptions,
		activeOptionsError,
		statusMessage,
		listedCategories,
		filteringCategories,
		scopeMatchKey,
		browseCategoryOptions,
		valueSuggestions,
		inlineOptions,
		mainInlineOptions,
		chipValues,
		highlightedItem,
		scopeWidened,
		scopeValue,
		optionChipKey,
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
	// Category previewed in the pointer flyout. Distinct from `activeCategoryKey`,
	// which is the committed drill-in state shared with keyboard navigation.
	// Reset whenever the menu opens or closes so a dismissed flyout does not
	// reappear next time.
	const [flyout, setFlyout] = useState<{
		categoryKey: string | null;
		menuOpen: boolean;
	}>({ categoryKey: null, menuOpen: open });
	if (flyout.menuOpen !== open) {
		setFlyout({ categoryKey: null, menuOpen: open });
	}
	const flyoutCategoryKey =
		flyout.menuOpen === open ? flyout.categoryKey : null;
	const setFlyoutCategoryKey = (categoryKey: string | null) =>
		setFlyout({ categoryKey, menuOpen: open });
	// Typing a scope toggle label (e.g. `shared`) opens that category's flyout
	// while its row is highlighted, so the toggle is visible.
	const shownFlyoutKey =
		flyoutCategoryKey ??
		(!isCoarsePointer &&
		scopeMatchKey !== null &&
		highlightedItem === scopeMatchKey
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
	// Switching between open flyouts is delayed so a diagonal move through a
	// neighboring category into the current flyout does not swap panels.
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
		if (element) {
			categoryRows.current.set(key, element);
		} else {
			categoryRows.current.delete(key);
		}
	};
	// Side panels align with the row that opened them. Measured after commit so
	// keyboard-entered categories line up too, not only pointer-hovered ones.
	const panelCategoryKey = activeCategoryKey ?? shownFlyoutKey;
	const [panelOffset, setPanelOffset] = useState(0);
	useLayoutEffect(() => {
		setPanelOffset(
			panelCategoryKey === null
				? 0
				: (categoryRows.current.get(panelCategoryKey)?.offsetTop ?? 0),
		);
	}, [panelCategoryKey]);
	// cmdk owns the highlighted row for both pointer and keyboard, so the flyout
	// follows it: it closes when the highlight leaves the category rows and
	// switches when it lands on another category.
	const handleItemHighlighted = (highlighted: string) => {
		actions.setHighlightedItem(highlighted);
		if (flyoutCategoryKey === null || highlighted === flyoutCategoryKey) {
			return;
		}
		const isCategoryRow = listedCategories.some(
			(category) => category.key === highlighted,
		);
		updateFlyoutCategory(isCategoryRow ? highlighted : null, !isCategoryRow);
	};
	const flyoutCategory = categories.find(
		(category) => category.key === shownFlyoutKey,
	);
	// Typed text narrows the category rows, so a click enters the category like
	// Enter does and only a scope match gets a flyout.
	const flyoutOptions =
		activeCategoryKey === null &&
		shownFlyoutKey !== null &&
		(!filteringCategories || shownFlyoutKey === scopeMatchKey)
			? browseCategoryOptions.get(shownFlyoutKey)
			: undefined;
	// Toggling clears text typed to find the category, so the flyout is pinned
	// open explicitly rather than through the scope match.
	const toggleFlyoutScope = (categoryKey: string) => {
		setFlyoutCategoryKey(categoryKey);
		actions.toggleScope(categoryKey);
	};
	const selectFlyoutOption = (token: string) => {
		actions.selectCategoryOption(token);
		updateFlyoutCategory(null, true);
		actions.focusInput();
	};
	const selectCategory = (categoryKey: string) => {
		updateFlyoutCategory(null, true);
		actions.selectCategory(categoryKey);
	};

	const scopeFor = (categoryKey: string): ScopeState | undefined => {
		const toggle = categories.find(
			(category) => category.key === categoryKey,
		)?.scopeToggle;
		return toggle
			? {
					widened: scopeWidened(categoryKey),
					label: toggle.label(scopeValue(categoryKey)),
				}
			: undefined;
	};
	const mainPanelProps = {
		listedCategories,
		valueSuggestions,
		inlineOptions,
		typeaheadError,
		registerCategoryRow,
		onSelectCategory: selectCategory,
		onHoverCategory: updateFlyoutCategory,
		onToggleInlineOption: actions.toggleInlineOption,
		onSelectSuggestion: actions.selectValueSuggestion,
		onRetry: actions.retryTypeahead,
	};
	const categoryOptionsList =
		activeCategoryKey === null ? undefined : (
			<CategoryOptionsList
				embedded={isMobile}
				offset={panelOffset}
				category={activeCategory}
				options={activeOptions}
				optionsError={activeOptionsError}
				previewCount={browseCategoryOptions.get(activeCategoryKey)?.length}
				selectedTokens={chipValues}
				chipKey={optionChipKey(activeCategoryKey)}
				scope={scopeFor(activeCategoryKey)}
				onToggleScope={actions.toggleScope}
				searchValue={inputValue}
				onSearchChange={actions.onInputValueChange}
				onRetry={actions.retryActiveOptions}
				onSelectOption={actions.selectCategoryOption}
			/>
		);

	return (
		<>
			<FilterComboboxRoot
				open={open}
				onDismiss={actions.dismiss}
				onRemoveValue={actions.removeChip}
				inputValue={inputValue}
				onInputValueChange={actions.onInputValueChange}
				highlightedValue={highlightedItem}
				onHighlightedValueChange={handleItemHighlighted}
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
							aria-label="Toggle filters"
							aria-expanded={open}
							aria-haspopup="listbox"
							className={cn(
								"h-9.5 min-w-0 shrink-0 rounded-none rounded-l-md pl-2.5 pr-3 text-sm [&>svg]:p-0",
								chipValues.length > 0 && "text-content-primary",
							)}
							onMouseDown={(event) => {
								// Prevent the button from taking focus on pointer open.
								// toggleFilterMenu focuses the combobox input next so
								// aria-activedescendant keyboard navigation still works.
								event.preventDefault();
							}}
							onKeyDown={(event) => {
								if (event.key !== "Enter") {
									return;
								}
								event.preventDefault();
								event.stopPropagation();
								actions.showMenu();
							}}
							onClick={(event) => {
								if (event.detail === 0) {
									actions.showMenu();
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
						{chipValues.map((token, index) => {
							const display = chipDisplay(token, categories);
							const category = categories.find(
								(entry) => entry.key === display.key,
							);
							// The scope pill follows the last chip its category owns.
							const scopePillLabel =
								category?.scopeToggle &&
								scopeWidened(category.key) &&
								!chipValues
									.slice(index + 1)
									.some(
										(later) =>
											chipDisplay(later, categories).key === category.key,
									)
									? category.scopeToggle.pillLabel.toLowerCase()
									: undefined;
							const labelOnly = category?.inlineOptionsLabelOnly === true;
							const inlineOption = labelOnly
								? mainInlineOptions.find(
										({ categoryKey, option }) =>
											(option.token ?? chipToken(categoryKey, option.value)) ===
											token,
									)
								: undefined;
							// Applied tokens read as query syntax, so they are always
							// lowercase even when the menu shows a display label.
							const prefix = (labelOnly ? "" : display.key).toLowerCase();
							const value = (
								inlineOption?.option.appliedLabel ??
								inlineOption?.option.label ??
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
											scopePillLabel && "rounded-r-none",
										)}
									>
										<ChipLabel prefix={prefix} value={value} />
									</FilterComboboxChip>
									{/* Joined to its chip, since it widens that chip's filter. */}
									{category && scopePillLabel && (
										<FilterComboboxChip
											removeLabel={`Remove ${scopePillLabel}`}
											onRemove={() => actions.toggleScope(category.key)}
											// Only the pill shrinks, so the pair never overflows the field.
											className={cn(
												labelOnlyChipClassName,
												"min-w-0 rounded-l-none border-l-surface-primary",
											)}
										>
											<Tooltip>
												<TooltipTrigger asChild>
													<span className="min-w-0 truncate">
														{scopePillLabel}
													</span>
												</TooltipTrigger>
												<TooltipContent>
													{splitIntoTwoLines(
														scopeFor(category.key)?.label ?? "",
													).map((line) => (
														<span key={line} className="block">
															{line}
														</span>
													))}
												</TooltipContent>
											</Tooltip>
										</FilterComboboxChip>
									)}
								</span>
							);
						})}
						{activeCategory && committedFreeText.length > 0 && (
							<Badge
								variant="outline"
								size="md"
								data-slot="combobox-chip-search"
								className="px-2 font-medium"
							>
								{committedFreeText}
							</Badge>
						)}
						{/* Decorative draft prefix: the live region already announces
							    "Filtering by <category>", so this stays hidden. */}
						{activeCategory && (
							<Badge
								variant="dashed"
								size="md"
								data-slot="combobox-chip-draft"
								className="px-2 font-medium"
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
					</FilterComboboxChips>
				</FilterComboboxInputGroup>
				<FilterComboboxContent
					align="start"
					side="bottom"
					avoidCollisions={!isMobile}
					className="relative left-0 w-(--radix-popover-trigger-width) max-w-(--radix-popper-available-width) overflow-visible border-0 bg-transparent shadow-none data-[state=closed]:hidden sm:w-fit"
				>
					{/* Keep mounted so polite status announcements stay consistent. */}
					<FilterComboboxStatus>{statusMessage}</FilterComboboxStatus>
					{isMobile ? (
						<div
							data-slot="filter-mobile-panel"
							data-testid="filter-mobile-panel"
							className="flex max-h-[min(24rem,var(--radix-popper-available-height))] w-(--radix-popover-trigger-width) max-w-full min-h-0 flex-col overflow-hidden rounded-md border border-border bg-surface-primary p-2 shadow-md"
						>
							{categoryOptionsList ?? (
								<MainPanel embedded drillIn {...mainPanelProps} />
							)}
						</div>
					) : (
						<div
							className="relative flex items-start gap-1 overflow-visible"
							onMouseLeave={() => {
								updateFlyoutCategory(null, true);
								actions.setHighlightedItem("");
							}}
						>
							<MainPanel
								drillIn={
									isCoarsePointer ||
									activeCategoryKey !== null ||
									filteringCategories
								}
								{...mainPanelProps}
							/>
							{categoryOptionsList ??
								(flyoutCategory && flyoutOptions && (
									<HoverCategoryPanel
										key={flyoutCategory.key}
										category={flyoutCategory}
										offset={panelOffset}
										options={flyoutOptions}
										selectedTokens={chipValues}
										chipKey={optionChipKey(flyoutCategory.key)}
										scope={scopeFor(flyoutCategory.key)}
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

// Categories with more options than this get a search field in their panel.
const SEARCHABLE_OPTION_COUNT = 10;

function NoMatchingOptions() {
	return (
		<div className="px-2 py-1.5 text-sm text-content-secondary">
			No matching options
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

type InlineOption = {
	categoryKey: string;
	categoryLabel: string;
	selected: boolean;
	showIcon: boolean;
	option: FilterOption;
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
	valueSuggestions: readonly ValueSuggestion[];
	inlineOptions: readonly InlineOption[];
	typeaheadError: boolean;
	embedded?: boolean;
	/**
	 * Clicking a category commits it as the active category (touch and mobile
	 * drill-in, or switching while one is already active) instead of only
	 * previewing it in the pointer flyout.
	 */
	drillIn: boolean;
	registerCategoryRow: (key: string, element: HTMLDivElement | null) => void;
	onSelectCategory: (categoryKey: string) => void;
	onHoverCategory: (categoryKey: string | null, immediate?: boolean) => void;
	onToggleInlineOption: (token: string) => void;
	onSelectSuggestion: (token: string) => void;
	onRetry: () => void;
}>;

function MainPanel({
	listedCategories,
	valueSuggestions,
	inlineOptions,
	typeaheadError,
	embedded = false,
	drillIn,
	registerCategoryRow,
	onSelectCategory,
	onHoverCategory,
	onToggleInlineOption,
	onSelectSuggestion,
	onRetry,
}: MainPanelProps) {
	const isEmpty =
		listedCategories.length === 0 &&
		valueSuggestions.length === 0 &&
		inlineOptions.length === 0 &&
		!typeaheadError;

	if (isEmpty) {
		return null;
	}

	return (
		<FilterComboboxList
			data-slot="filter-main-panel"
			className={cn(
				"w-(--radix-popover-trigger-width) max-w-full shrink-0 rounded-md border border-border bg-surface-primary p-2 shadow-md sm:w-64",
				embedded &&
					"w-full rounded-none border-0 bg-transparent p-0 shadow-none",
			)}
		>
			{listedCategories.map((category) => (
				<FilterComboboxItem
					ref={(element) => registerCategoryRow(category.key, element)}
					className={OPTION_ITEM_CLASS}
					key={category.key}
					value={category.key}
					onMouseEnter={() => onHoverCategory(category.key)}
					onSelect={() => {
						if (drillIn) {
							onSelectCategory(category.key);
							return;
						}
						onHoverCategory(category.key, true);
					}}
				>
					{category.icon && <OptionIcon>{category.icon}</OptionIcon>}
					<span className="shrink-0">{category.label}</span>
					<ChevronRightIcon aria-hidden className="ml-auto shrink-0" />
				</FilterComboboxItem>
			))}
			{groupByCategoryLabel(inlineOptions).map(([label, options]) => (
				<FilterComboboxGroup
					className="mt-2 border-t border-border pt-2 first:mt-0 first:border-t-0 first:pt-0"
					key={label}
				>
					{label && (
						<FilterComboboxLabel className="pt-0 opacity-80">
							{label}
						</FilterComboboxLabel>
					)}
					{options.map(({ categoryKey, option, selected, showIcon }) => {
						const token = option.token ?? chipToken(categoryKey, option.value);
						return (
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
						);
					})}
				</FilterComboboxGroup>
			))}
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
				<div className="flex flex-col items-center gap-2 px-2 py-2.5 text-center text-sm text-content-secondary">
					<span>Couldn&rsquo;t load suggestions.</span>
					<Button size="sm" variant="outline" onClick={onRetry}>
						Retry
					</Button>
				</div>
			)}
		</FilterComboboxList>
	);
}

type FlyoutSearchProps = Readonly<{
	label: string;
	value: string;
	onChange: (value: string) => void;
}>;

function FlyoutSearch({ label, value, onChange }: FlyoutSearchProps) {
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
				onKeyDown={(event) => event.stopPropagation()}
			/>
		</div>
	);
}

type ScopeState = Readonly<{ widened: boolean; label: string }>;

/**
 * Splits text at the space nearest its middle, so a tooltip shows it as two
 * even lines and sizes to the longer one.
 */
function splitIntoTwoLines(text: string): string[] {
	const middle = text.length / 2;
	let splitAt = -1;
	for (
		let index = text.indexOf(" ");
		index !== -1;
		index = text.indexOf(" ", index + 1)
	) {
		if (
			splitAt === -1 ||
			Math.abs(index - middle) < Math.abs(splitAt - middle)
		) {
			splitAt = index;
		}
	}
	return splitAt === -1
		? [text]
		: [text.slice(0, splitAt), text.slice(splitAt + 1)];
}

type FlyoutScopeToggleProps = Readonly<{
	categoryKey: string;
	label: string;
	checked: boolean;
	onToggle: (categoryKey: string) => void;
}>;

function FlyoutScopeToggle({
	categoryKey,
	label,
	checked,
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
				onCheckedChange={() => onToggle(categoryKey)}
				// Keep focus in the combobox input so keyboard navigation continues.
				onMouseDown={(event) => event.preventDefault()}
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
	empty: boolean;
	scope: ScopeState | undefined;
	onToggleScope: (categoryKey: string) => void;
	onMouseEnter?: () => void;
	children: ReactNode;
}>;

// Shared shell for a category's options: search header, scrolling list, empty
// state, and scope toggle footer.
function OptionsPanel({
	category,
	embedded = false,
	offset,
	search,
	empty,
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
				/>
			)}
			{children}
			{empty && <NoMatchingOptions />}
			{category && scope && (
				<FlyoutScopeToggle
					categoryKey={category.key}
					label={scope.label}
					checked={scope.widened}
					onToggle={onToggleScope}
				/>
			)}
		</div>
	);
}

type HoverCategoryPanelProps = Readonly<{
	category: FilterCategory;
	offset: number;
	options: readonly FilterOption[];
	selectedTokens: readonly string[];
	chipKey: string;
	scope: ScopeState | undefined;
	onToggleScope: (categoryKey: string) => void;
	onMouseEnter: () => void;
	onSelectOption: (token: string) => void;
}>;

function HoverCategoryPanel({
	category,
	offset,
	options,
	selectedTokens,
	chipKey,
	scope,
	onToggleScope,
	onMouseEnter,
	onSelectOption,
}: HoverCategoryPanelProps) {
	const [query, setQuery] = useState("");
	const trimmedQuery = query.trim();
	// `options` is the preview page, which may be truncated, so searches go
	// through the category's loader. The preview is filtered locally meanwhile.
	const debouncedQuery = useDebouncedValue(trimmedQuery, SEARCH_DEBOUNCE_MS);
	const searchResults = useQuery(
		filterComboboxOptions(
			category.key,
			category.getOptions,
			debouncedQuery,
			debouncedQuery.length > 0,
		),
	);
	const normalized = trimmedQuery.toLowerCase();
	const filteredOptions =
		normalized.length === 0
			? options
			: debouncedQuery === trimmedQuery && searchResults.data
				? searchResults.data
				: options.filter(
						(option) =>
							option.label.toLowerCase().includes(normalized) ||
							option.value.toLowerCase().includes(normalized),
					);
	const searchable = options.length > SEARCHABLE_OPTION_COUNT;
	if (filteredOptions.length === 0 && !searchable) {
		return null;
	}

	return (
		<OptionsPanel
			category={category}
			offset={offset}
			search={searchable ? { value: query, onChange: setQuery } : undefined}
			empty={filteredOptions.length === 0}
			scope={scope}
			onToggleScope={onToggleScope}
			onMouseEnter={onMouseEnter}
		>
			<div className="min-h-0 flex-1 overflow-y-auto overscroll-contain pr-1">
				{filteredOptions.map((option) => {
					const token = option.token ?? chipToken(chipKey, option.value);
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
	previewCount: number | undefined;
	selectedTokens: readonly string[];
	chipKey: string;
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
	previewCount,
	selectedTokens,
	chipKey,
	scope,
	onToggleScope,
	searchValue,
	onSearchChange,
	onRetry,
	onSelectOption,
}: CategoryOptionsListProps) {
	if (optionsError) {
		return (
			<div
				className={cn(
					flyoutPanelClassName,
					"items-center gap-2 px-3 py-6 text-center text-sm text-content-secondary",
					embedded && "w-full rounded-none border-0 bg-transparent shadow-none",
				)}
				style={embedded ? undefined : { top: offset }}
			>
				<span>
					Couldn&rsquo;t load {category ? category.label : "filter"} options.
				</span>
				<Button size="sm" variant="outline" onClick={onRetry}>
					Retry
				</Button>
			</div>
		);
	}

	// Decided from the unfiltered list so the search field stays mounted while
	// results load or shrink.
	const searchable =
		!embedded &&
		(previewCount ?? options?.length ?? 0) > SEARCHABLE_OPTION_COUNT;
	if ((options === undefined || options.length === 0) && !searchable) {
		return null;
	}

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
			empty={options?.length === 0}
			scope={scope}
			onToggleScope={onToggleScope}
		>
			<FilterComboboxList className="min-h-0 flex-1 touch-pan-y overflow-y-auto overscroll-contain p-0 pr-1">
				{options?.map((option) => {
					const item = option.token ?? chipToken(chipKey, option.value);
					const selected = selectedTokens.includes(item);
					return (
						<FilterComboboxItem
							className={cn(
								OPTION_ITEM_CLASS,
								selected && "text-content-primary",
							)}
							key={item}
							value={item}
							onSelect={() => onSelectOption(item)}
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
