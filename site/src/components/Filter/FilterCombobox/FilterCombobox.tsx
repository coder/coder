import { ListFilterIcon, SearchIcon } from "lucide-react";
import { type ReactNode, useId } from "react";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { ListFilterActiveIcon } from "#/components/Icons/ListFilterActiveIcon";
import {
	InputGroupAddon,
	InputGroupButton,
} from "#/components/InputGroup/InputGroup";
import { Spinner } from "#/components/Spinner/Spinner";
import { chipDisplay, chipToken } from "./filterQuery";
import {
	FilterComboboxChip,
	FilterComboboxChips,
	FilterComboboxChipsInput,
	FilterComboboxContent,
	FilterComboboxEmpty,
	FilterComboboxGroup,
	FilterComboboxInputGroup,
	FilterComboboxItem,
	FilterComboboxLabel,
	FilterComboboxList,
	FilterComboboxRoot,
	FilterComboboxStatus,
} from "./primitives";
import type { FilterCategory, FilterOption, SearchResult } from "./types";
import { useFilterCombobox } from "./useFilterCombobox";

/**
 * Unified workspace filter input: renders committed chips plus a cmdk-driven
 * popup that browses categories, surfaces cross-category value suggestions, and
 * (optionally) previews matching resources. State lives in `useFilterCombobox`.
 */
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
	/** Debounced free-text resource previews (e.g. matching workspaces). */
	getSearchResults?: (query: string) => Promise<SearchResult[]>;
	onSearchResultSelect?: (result: SearchResult) => void;
	searchResultsLabel?: string;
}>;

export function FilterCombobox({
	value,
	onChange,
	categories,
	placeholder = "Search and filter…",
	className,
	errorMessage,
	getSearchResults,
	onSearchResultSelect,
	searchResultsLabel = "Results",
}: FilterComboboxProps) {
	const {
		open,
		inputValue,
		committedFreeText,
		activeCategoryKey,
		activeCategory,
		activeOptions,
		activeOptionsLoading,
		activeOptionsError,
		statusMessage,
		listedCategories,
		valueSuggestions,
		searchResults,
		chipValues,
		typeahead,
		actions,
	} = useFilterCombobox({
		value,
		onChange,
		categories,
		getSearchResults,
		onSearchResultSelect,
	});

	const errorId = useId();
	const invalid = errorMessage !== undefined;

	return (
		<>
			<FilterComboboxRoot
				open={open}
				onDismiss={actions.dismiss}
				onRemoveValue={actions.removeChip}
				inputValue={inputValue}
				onInputValueChange={actions.onInputValueChange}
				onItemHighlighted={actions.onItemHighlighted}
				label={placeholder}
			>
				<FilterComboboxInputGroup className={className}>
					<InputGroupAddon className="min-h-10 self-start pt-1">
						<SearchIcon aria-hidden className="size-icon-sm" />
					</InputGroupAddon>
					<FilterComboboxChips>
						{chipValues.map((token) => {
							const display = chipDisplay(token, categories);
							const displayText = display.key
								? chipToken(display.key, display.value)
								: display.value;
							return (
								<FilterComboboxChip
									key={token}
									value={token}
									removeLabel={`Remove ${displayText}`}
								>
									<ChipLabel prefix={display.key} value={display.value} />
								</FilterComboboxChip>
							);
						})}
						{activeCategory && committedFreeText.length > 0 && (
							<Badge
								variant="outline"
								size="md"
								data-slot="combobox-chip-search"
								className="px-2 text-sm font-medium leading-4"
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
								className="px-2 text-sm font-medium leading-4"
								aria-hidden
							>
								{`${activeCategory.key}:`}
							</Badge>
						)}
						<FilterComboboxChipsInput
							ref={actions.setInputRef}
							aria-label={placeholder}
							aria-invalid={invalid || undefined}
							aria-errormessage={invalid ? errorId : undefined}
							placeholder={
								chipValues.length > 0 || activeCategory ? "" : placeholder
							}
							onFocus={actions.onInputFocus}
							onKeyDown={actions.onInputKeyDown}
						/>
					</FilterComboboxChips>
					<InputGroupAddon
						align="inline-end"
						className="w-10 items-start self-stretch border-0 border-l border-solid border-border p-0"
					>
						<InputGroupButton
							type="button"
							variant="subtle"
							aria-label="Toggle filters"
							aria-expanded={open}
							aria-haspopup="listbox"
							className="min-h-10 w-10 min-w-10 shrink-0 rounded-none rounded-r-md px-0 pt-2.5 [&>svg]:p-0"
							onMouseDown={(event) => {
								// Prevent the button from taking focus on pointer open.
								// toggleFilterMenu focuses the combobox input next so
								// aria-activedescendant keyboard navigation still works.
								event.preventDefault();
							}}
							onClick={actions.toggleMenu}
						>
							{/* Rendered 2px larger than the plain icon so both read as the
							    same size; `!` overrides the button's `[&>svg]:size-*` rule. */}
							{chipValues.length > 0 ? (
								<ListFilterActiveIcon
									aria-hidden
									data-testid="filter-active-icon"
									className="size-5!"
								/>
							) : (
								<ListFilterIcon aria-hidden className="size-icon-sm" />
							)}
						</InputGroupButton>
					</InputGroupAddon>
				</FilterComboboxInputGroup>
				<FilterComboboxContent>
					{/* Keep mounted so polite status announcements stay consistent. */}
					<FilterComboboxStatus>{statusMessage}</FilterComboboxStatus>
					{typeahead.active ? (
						<TypeaheadList
							listedCategories={listedCategories}
							valueSuggestions={valueSuggestions}
							searchResults={searchResults}
							searchResultsLabel={searchResultsLabel}
							showSearchSection={typeahead.showSearchResults}
							typeaheadLoading={typeahead.loading}
							typeaheadError={typeahead.error}
							typeaheadErrorLabel={typeahead.errorLabel}
							onSelectCategory={actions.selectCategory}
							onSelectSuggestion={actions.selectValueSuggestion}
							onSelectSearchResult={actions.selectSearchResult}
							onRetry={actions.retryTypeahead}
						/>
					) : (
						<CategoryOptionsList
							activeCategory={activeCategory}
							activeCategoryKey={activeCategoryKey}
							activeOptions={activeOptions}
							activeOptionsLoading={activeOptionsLoading}
							activeOptionsError={activeOptionsError}
							retryActiveOptions={actions.retryActiveOptions}
							onSelectOption={actions.selectCategoryOption}
						/>
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

function ResultIcon({ result }: { result: SearchResult }): ReactNode {
	if (result.startIcon) {
		return <OptionIcon>{result.startIcon}</OptionIcon>;
	}
	if (result.imageUrl !== undefined) {
		return (
			<OptionIcon>
				<Avatar src={result.imageUrl} fallback={result.label} size="sm" />
			</OptionIcon>
		);
	}
	return null;
}

type ValueSuggestion = {
	categoryLabel: string;
	token: string;
	option: Pick<FilterOption, "label" | "startIcon">;
};

type TypeaheadListProps = Readonly<{
	listedCategories: readonly FilterCategory[];
	valueSuggestions: readonly ValueSuggestion[];
	searchResults: readonly SearchResult[];
	searchResultsLabel: string;
	showSearchSection: boolean;
	typeaheadLoading: boolean;
	typeaheadError: boolean;
	typeaheadErrorLabel: string;
	onSelectCategory: (categoryKey: string) => void;
	onSelectSuggestion: (token: string) => void;
	onSelectSearchResult: (result: SearchResult) => void;
	onRetry: () => void;
}>;

function TypeaheadList({
	listedCategories,
	valueSuggestions,
	searchResults,
	searchResultsLabel,
	showSearchSection,
	typeaheadLoading,
	typeaheadError,
	typeaheadErrorLabel,
	onSelectCategory,
	onSelectSuggestion,
	onSelectSearchResult,
	onRetry,
}: TypeaheadListProps) {
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
		!showSearchSection &&
		!typeaheadLoading &&
		!typeaheadError;

	return (
		<>
			{isEmpty && <FilterComboboxEmpty>No filters found.</FilterComboboxEmpty>}
			<FilterComboboxList className="p-2">
				{listedCategories.map((category) => (
					<FilterComboboxItem
						className={OPTION_ITEM_CLASS}
						key={category.key}
						value={category.key}
						onSelect={() => onSelectCategory(category.key)}
					>
						{category.icon && <OptionIcon>{category.icon}</OptionIcon>}
						{category.label}
					</FilterComboboxItem>
				))}
				{[...valueSuggestionsByCategory.entries()].map(
					([categoryLabel, suggestions]) => (
						<FilterComboboxGroup key={categoryLabel}>
							<FilterComboboxLabel>{categoryLabel}</FilterComboboxLabel>
							{suggestions.map((suggestion) => (
								<FilterComboboxItem
									className={OPTION_ITEM_CLASS}
									key={suggestion.token}
									value={suggestion.token}
									onSelect={() => onSelectSuggestion(suggestion.token)}
								>
									{suggestion.option.startIcon ? (
										<OptionIcon>{suggestion.option.startIcon}</OptionIcon>
									) : null}
									{suggestion.option.label}
								</FilterComboboxItem>
							))}
						</FilterComboboxGroup>
					),
				)}
				{showSearchSection && (
					<FilterComboboxGroup>
						<FilterComboboxLabel>{searchResultsLabel}</FilterComboboxLabel>
						{searchResults.map((result) => (
							<FilterComboboxItem
								className={OPTION_ITEM_CLASS}
								key={result.value}
								value={result.value}
								onSelect={() => onSelectSearchResult(result)}
							>
								<ResultIcon result={result} />
								<span className="truncate">{result.label}</span>
							</FilterComboboxItem>
						))}
					</FilterComboboxGroup>
				)}
				{typeaheadLoading && (
					<div className="flex items-center justify-center px-2 py-2.5">
						<Spinner loading size="sm" label="Loading suggestions" />
					</div>
				)}
				{typeaheadError && !typeaheadLoading && (
					<div className="flex flex-col items-center gap-2 px-2 py-2.5 text-center text-sm text-content-secondary">
						<span>{typeaheadErrorLabel}</span>
						<Button size="sm" variant="outline" onClick={onRetry}>
							Retry
						</Button>
					</div>
				)}
			</FilterComboboxList>
		</>
	);
}

type CategoryOptionsListProps = Readonly<{
	activeCategory: FilterCategory | undefined;
	activeCategoryKey: string | null;
	activeOptions: readonly FilterOption[] | undefined;
	activeOptionsLoading: boolean;
	activeOptionsError: boolean;
	retryActiveOptions: () => void;
	onSelectOption: (token: string) => void;
}>;

function CategoryOptionsList({
	activeCategory,
	activeCategoryKey,
	activeOptions,
	activeOptionsLoading,
	activeOptionsError,
	retryActiveOptions,
	onSelectOption,
}: CategoryOptionsListProps) {
	// While the popover animates closed the active category resets to null; render
	// nothing rather than flashing a "Loading…"/empty state on the way out.
	if (activeCategoryKey === null) {
		return null;
	}

	if (activeOptionsError) {
		return (
			<div className="flex flex-col items-center gap-2 px-3 py-6 text-center text-sm text-content-secondary">
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

	if (activeOptionsLoading || activeOptions === undefined) {
		return (
			<div
				className="px-3 py-6 text-center text-sm text-content-secondary"
				aria-hidden
			>
				Loading…
			</div>
		);
	}

	return (
		<>
			<FilterComboboxEmpty>
				{activeCategory
					? `No ${activeCategory.label} matches`
					: "No filters found."}
			</FilterComboboxEmpty>
			<FilterComboboxList className="p-2">
				{activeCategoryKey !== null && (
					<FilterComboboxGroup>
						{activeCategory && (
							<FilterComboboxLabel>{activeCategory.label}</FilterComboboxLabel>
						)}
						{activeOptions.map((option) => {
							const item =
								option.token ?? chipToken(activeCategoryKey, option.value);
							return (
								<FilterComboboxItem
									className={OPTION_ITEM_CLASS}
									key={item}
									value={item}
									onSelect={() => onSelectOption(item)}
								>
									{option.startIcon ? (
										<OptionIcon>{option.startIcon}</OptionIcon>
									) : null}
									{option.label}
								</FilterComboboxItem>
							);
						})}
					</FilterComboboxGroup>
				)}
			</FilterComboboxList>
		</>
	);
}
