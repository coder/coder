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
	useMemo,
	useRef,
	useState,
} from "react";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { ListFilterActiveIcon } from "#/components/Icons/ListFilterActiveIcon";
import {
	InputGroupAddon,
	InputGroupButton,
} from "#/components/InputGroup/InputGroup";
import { Switch } from "#/components/Switch/Switch";
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
import type { FilterCategory, FilterOption } from "./types";
import { useFilterCombobox } from "./useFilterCombobox";

/**
 * Unified workspace filter input: renders committed chips plus a cmdk-driven
 * popup that browses categories and surfaces cross-category value suggestions.
 * State lives in `useFilterCombobox`.
 */
const CATEGORY_HOVER_DELAY_MS = 300;

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
		browseAll,
		inputValue,
		committedFreeText,
		activeCategoryKey,
		activeCategory,
		activeOptions,
		activeOptionsError,
		statusMessage,
		listedCategories,
		browseCategoryOptions,
		valueSuggestions,
		inlineOptions,
		mainInlineOptions,
		chipValues,
		highlightedItem,
		scopeWidened,
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
	const panelCategoryKey = activeCategoryKey ?? flyoutCategoryKey;
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
		(category) => category.key === flyoutCategoryKey,
	);
	const flyoutOptions =
		activeCategoryKey === null &&
		flyoutCategoryKey !== null &&
		(browseAll || inputValue.trim().length === 0)
			? browseCategoryOptions.get(flyoutCategoryKey)
			: undefined;
	const selectFlyoutOption = (token: string) => {
		actions.selectCategoryOption(token);
		updateFlyoutCategory(null, true);
	};
	const selectCategory = (categoryKey: string) => {
		updateFlyoutCategory(null, true);
		actions.selectCategory(categoryKey);
	};

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
								"h-9.5 min-w-0 shrink-0 gap-1.5 rounded-none rounded-l-md pl-2.5 pr-3 text-sm [&>svg]:p-0",
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
							<span className="hidden sm:inline">Filters</span>
						</InputGroupButton>
					</InputGroupAddon>
					<InputGroupAddon className="h-9.5 self-start px-2">
						<SearchIcon aria-hidden className="size-icon-sm" />
					</InputGroupAddon>
					<FilterComboboxChips>
						{chipValues.map((token) => {
							const display = chipDisplay(token, categories);
							const inlineOption = mainInlineOptions.find(
								({ categoryKey, option }) => {
									const optionToken =
										option.token ?? chipToken(categoryKey, option.value);
									return optionToken === token;
								},
							);
							const labelOnly = inlineOption?.categoryKey === "attribute";
							// Applied tokens read as query syntax, so they are always
							// lowercase even when the menu shows a display label.
							const prefix = (labelOnly ? "" : display.key).toLowerCase();
							const value = (
								labelOnly
									? (inlineOption?.option.appliedLabel ??
										inlineOption?.option.label ??
										display.value)
									: display.value
							).toLowerCase();
							const displayText = prefix ? chipToken(prefix, value) : value;
							return (
								<FilterComboboxChip
									key={token}
									value={token}
									removeLabel={`Remove ${displayText}`}
									className={
										labelOnly
											? "text-content-primary [&_[data-slot=combobox-chip-remove]]:text-content-secondary"
											: undefined
									}
								>
									<ChipLabel prefix={prefix} value={value} />
								</FilterComboboxChip>
							);
						})}
						{categories.map((category) =>
							category.scopeToggle && scopeWidened(category.key) ? (
								<FilterComboboxChip
									key={`${category.key}-scope`}
									removeLabel={`Remove ${category.scopeToggle.pillLabel.toLowerCase()}`}
									onRemove={() => actions.toggleScope(category.key)}
								>
									{category.scopeToggle.pillLabel.toLowerCase()}
								</FilterComboboxChip>
							) : null,
						)}
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
							{activeCategoryKey === null ? (
								<MainPanel
									embedded
									listedCategories={listedCategories}
									valueSuggestions={valueSuggestions}
									inlineOptions={inlineOptions}
									typeaheadError={typeaheadError}
									drillIn
									registerCategoryRow={registerCategoryRow}
									onSelectCategory={selectCategory}
									onHoverCategory={updateFlyoutCategory}
									onToggleInlineOption={actions.toggleInlineOption}
									onSelectSuggestion={actions.selectValueSuggestion}
									onRetry={actions.retryTypeahead}
								/>
							) : (
								<CategoryOptionsList
									embedded
									isMobile
									offset={0}
									activeCategory={activeCategory}
									activeCategoryKey={activeCategoryKey}
									activeOptions={activeOptions}
									activeOptionsError={activeOptionsError}
									selectedTokens={chipValues}
									chipKey={optionChipKey(activeCategoryKey)}
									scopeWidened={scopeWidened(activeCategoryKey)}
									onToggleScope={actions.toggleScope}
									inputValue={inputValue}
									onInputValueChange={actions.onInputValueChange}
									retryActiveOptions={actions.retryActiveOptions}
									onSelectOption={actions.selectCategoryOption}
								/>
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
								listedCategories={listedCategories}
								valueSuggestions={valueSuggestions}
								inlineOptions={inlineOptions}
								typeaheadError={typeaheadError}
								drillIn={isCoarsePointer || activeCategoryKey !== null}
								registerCategoryRow={registerCategoryRow}
								onSelectCategory={selectCategory}
								onHoverCategory={updateFlyoutCategory}
								onToggleInlineOption={actions.toggleInlineOption}
								onSelectSuggestion={actions.selectValueSuggestion}
								onRetry={actions.retryTypeahead}
							/>
							{activeCategoryKey !== null ? (
								<CategoryOptionsList
									offset={panelOffset}
									activeCategory={activeCategory}
									activeCategoryKey={activeCategoryKey}
									activeOptions={activeOptions}
									activeOptionsError={activeOptionsError}
									selectedTokens={chipValues}
									chipKey={optionChipKey(activeCategoryKey)}
									scopeWidened={scopeWidened(activeCategoryKey)}
									onToggleScope={actions.toggleScope}
									inputValue={inputValue}
									onInputValueChange={actions.onInputValueChange}
									retryActiveOptions={actions.retryActiveOptions}
									onSelectOption={actions.selectCategoryOption}
								/>
							) : (
								flyoutCategory &&
								flyoutOptions && (
									<HoverCategoryPanel
										key={flyoutCategory.key}
										category={flyoutCategory}
										offset={panelOffset}
										options={flyoutOptions}
										selectedTokens={chipValues}
										chipKey={optionChipKey(flyoutCategory.key)}
										scopeWidened={scopeWidened(flyoutCategory.key)}
										onToggleScope={actions.toggleScope}
										onMouseEnter={cancelHoverSwitch}
										onSelectOption={selectFlyoutOption}
									/>
								)
							)}
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

const groupInlineOptions = (
	options: readonly InlineOption[],
): Array<[string, InlineOption[]]> => {
	const groups = new Map<string, InlineOption[]>();
	for (const option of options) {
		const group = groups.get(option.categoryLabel);
		if (group) {
			group.push(option);
		} else {
			groups.set(option.categoryLabel, [option]);
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
	const valueSuggestionsByCategory = new Map<string, ValueSuggestion[]>();
	for (const suggestion of valueSuggestions) {
		const categorySuggestions = valueSuggestionsByCategory.get(
			suggestion.categoryLabel,
		);
		if (categorySuggestions) {
			categorySuggestions.push(suggestion);
		} else {
			valueSuggestionsByCategory.set(suggestion.categoryLabel, [suggestion]);
		}
	}

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
			{groupInlineOptions(inlineOptions).map(([label, options]) => (
				<FilterComboboxGroup
					className="mt-2 border-t border-border pt-2 first:mt-0 first:border-t-0 first:pt-0"
					key={label}
				>
					<FilterComboboxLabel className="pt-0 opacity-80">
						{label}
					</FilterComboboxLabel>
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
								{showIcon && option.startIcon ? (
									<OptionIcon>{option.startIcon}</OptionIcon>
								) : null}
								<span>{option.label}</span>
								{selected && (
									<CheckIcon aria-hidden className="ml-auto size-4 shrink-0" />
								)}
							</FilterComboboxItem>
						);
					})}
				</FilterComboboxGroup>
			))}
			{[...valueSuggestionsByCategory.entries()].map(
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
								{suggestion.option.startIcon ? (
									<OptionIcon>{suggestion.option.startIcon}</OptionIcon>
								) : null}
								{suggestion.option.label}
								{suggestion.selected && (
									<CheckIcon aria-hidden className="ml-auto size-4 shrink-0" />
								)}
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
		<div className="-mx-2 -mb-2 mt-2 flex items-center gap-2 border-t border-border px-3 py-2.5">
			<Switch
				id={id}
				size="sm"
				checked={checked}
				onCheckedChange={() => onToggle(categoryKey)}
				// Keep focus in the combobox input so keyboard navigation continues.
				onMouseDown={(event) => event.preventDefault()}
			/>
			<label htmlFor={id} className="text-sm text-content-primary">
				{label}
			</label>
		</div>
	);
}

type HoverCategoryPanelProps = Readonly<{
	category: FilterCategory;
	offset: number;
	options: readonly FilterOption[];
	selectedTokens: readonly string[];
	chipKey: string;
	scopeWidened: boolean;
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
	scopeWidened,
	onToggleScope,
	onMouseEnter,
	onSelectOption,
}: HoverCategoryPanelProps) {
	const [query, setQuery] = useState("");
	const filteredOptions = useMemo(() => {
		const normalized = query.trim().toLowerCase();
		return normalized.length === 0
			? options
			: options.filter(
					(option) =>
						option.label.toLowerCase().includes(normalized) ||
						option.value.toLowerCase().includes(normalized),
				);
	}, [options, query]);
	const searchable = options.length > 10;
	if (filteredOptions.length === 0) {
		return null;
	}

	return (
		<div
			onMouseEnter={onMouseEnter}
			className={cn(flyoutPanelClassName, "p-2")}
			style={{
				top: searchable ? 0 : offset,
				maxHeight: `min(20rem, calc(var(--radix-popper-available-height) - ${searchable ? 0 : offset}px))`,
			}}
		>
			{searchable && (
				<FlyoutSearch
					label={category.label}
					value={query}
					onChange={setQuery}
				/>
			)}
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
							onClick={() => onSelectOption(token)}
						>
							{option.startIcon ? (
								<OptionIcon>{option.startIcon}</OptionIcon>
							) : null}
							<span>{option.label}</span>
							{selected && (
								<CheckIcon aria-hidden className="ml-auto size-4 shrink-0" />
							)}
						</button>
					);
				})}
			</div>
			{category.scopeToggle && (
				<FlyoutScopeToggle
					categoryKey={category.key}
					label={category.scopeToggle.label}
					checked={scopeWidened}
					onToggle={onToggleScope}
				/>
			)}
		</div>
	);
}

type CategoryOptionsListProps = Readonly<{
	embedded?: boolean;
	isMobile?: boolean;
	offset: number;
	activeCategory: FilterCategory | undefined;
	activeCategoryKey: string | null;
	activeOptions: readonly FilterOption[] | undefined;
	activeOptionsError: boolean;
	selectedTokens: readonly string[];
	chipKey: string;
	scopeWidened: boolean;
	onToggleScope: (categoryKey: string) => void;
	inputValue: string;
	onInputValueChange: (value: string) => void;
	retryActiveOptions: () => void;
	onSelectOption: (token: string) => void;
}>;

function CategoryOptionsList({
	embedded = false,
	isMobile = false,
	offset,
	activeCategory,
	activeCategoryKey,
	activeOptions,
	activeOptionsError,
	selectedTokens,
	chipKey,
	scopeWidened,
	onToggleScope,
	inputValue,
	onInputValueChange,
	retryActiveOptions,
	onSelectOption,
}: CategoryOptionsListProps) {
	if (activeCategoryKey === null) {
		return null;
	}

	if (activeOptionsError) {
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
					Couldn&rsquo;t load {activeCategory ? activeCategory.label : "filter"}{" "}
					options.
				</span>
				<Button size="sm" variant="outline" onClick={retryActiveOptions}>
					Retry
				</Button>
			</div>
		);
	}

	if (activeOptions === undefined || activeOptions.length === 0) {
		return null;
	}

	const searchable = activeOptions.length > 10;
	return (
		<div
			className={cn(
				flyoutPanelClassName,
				"p-2",
				embedded &&
					"min-h-0 flex-1 w-full rounded-none border-0 bg-transparent p-0 shadow-none",
			)}
			style={
				embedded
					? undefined
					: {
							top: searchable ? 0 : offset,
							maxHeight: `min(20rem, calc(var(--radix-popper-available-height) - ${searchable ? 0 : offset}px))`,
						}
			}
		>
			{!isMobile && searchable && activeCategory && (
				<FlyoutSearch
					label={activeCategory.label}
					value={inputValue}
					onChange={onInputValueChange}
				/>
			)}
			<FilterComboboxList className="min-h-0 flex-1 touch-pan-y overflow-y-auto overscroll-contain p-0 pr-1">
				{activeOptions.map((option) => {
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
							{option.startIcon ? (
								<OptionIcon>{option.startIcon}</OptionIcon>
							) : null}
							{option.label}
							{selected && (
								<CheckIcon aria-hidden className="ml-auto size-4 shrink-0" />
							)}
						</FilterComboboxItem>
					);
				})}
			</FilterComboboxList>
			{activeCategory?.scopeToggle && (
				<FlyoutScopeToggle
					categoryKey={activeCategory.key}
					label={activeCategory.scopeToggle.label}
					checked={scopeWidened}
					onToggle={onToggleScope}
				/>
			)}
		</div>
	);
}
