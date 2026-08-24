import { ListFilterIcon, SearchIcon } from "lucide-react";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	InputGroupAddon,
	InputGroupButton,
} from "#/components/InputGroup/InputGroup";
import { Spinner } from "#/components/Spinner/Spinner";
import { chipToken } from "./filterQuery";
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
	FilterComboboxValue,
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
	/** Marks the input invalid (e.g. the server rejected the filter query). */
	invalid?: boolean;
	/** Id of the visible error message, linked from the input when invalid. */
	errorId?: string;
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
	invalid = false,
	errorId,
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

	return (
		<FilterComboboxRoot
			open={open}
			onDismiss={actions.dismiss}
			value={chipValues}
			onRemoveValue={actions.removeChip}
			inputValue={inputValue}
			onInputValueChange={actions.onInputValueChange}
			onItemHighlighted={actions.onItemHighlighted}
			label={placeholder}
		>
			<FilterComboboxInputGroup className={className}>
				<InputGroupAddon className="min-h-10">
					<SearchIcon aria-hidden className="size-icon-sm" />
				</InputGroupAddon>
				<FilterComboboxChips>
					<FilterComboboxValue>
						{(selected: string[]) => (
							<>
								{selected.map((token) => (
									<FilterComboboxChip key={token} value={token}>
										{token}
									</FilterComboboxChip>
								))}
								{activeCategory && committedFreeText.length > 0 && (
									<Badge
										variant="outline"
										size="md"
										data-slot="combobox-chip-search"
										className="font-medium"
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
										className="font-medium"
										aria-hidden
									>
										{/* A single-key category previews its chip prefix
										    (e.g. `status:`); a multi-key one (Attributes
										    commits `outdated:true`, etc.) shows its label. */}
										{activeCategory.chipKeys &&
										!activeCategory.chipKeys.includes(activeCategory.key)
											? activeCategory.label
											: `${activeCategory.key}:`}
									</Badge>
								)}
								<FilterComboboxChipsInput
									ref={actions.setInputRef}
									aria-label={placeholder}
									aria-invalid={invalid || undefined}
									aria-errormessage={invalid ? errorId : undefined}
									placeholder={
										selected.length > 0 || activeCategory ? "" : placeholder
									}
									onFocus={actions.onInputFocus}
									onKeyDown={actions.onInputKeyDown}
								/>
							</>
						)}
					</FilterComboboxValue>
				</FilterComboboxChips>
				<InputGroupAddon
					align="inline-end"
					className="w-10 items-center self-stretch border-0 border-l border-solid border-border p-0"
				>
					<InputGroupButton
						type="button"
						variant="subtle"
						aria-label="Toggle filters"
						aria-expanded={open}
						aria-haspopup="listbox"
						className="min-h-10 w-10 min-w-10 shrink-0 rounded-none rounded-r-md px-0 [&>svg]:p-0"
						onMouseDown={(event) => {
							// Prevent the button from taking focus on pointer open.
							// toggleFilterMenu focuses the combobox input next so
							// aria-activedescendant keyboard navigation still works.
							event.preventDefault();
						}}
						onClick={actions.toggleMenu}
					>
						<ListFilterIcon aria-hidden className="size-icon-sm" />
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
	);
}

const OPTION_ITEM_CLASS = "gap-2 px-2 py-2.5";

function resultIcon(result: SearchResult) {
	if (result.startIcon) {
		return <span aria-hidden>{result.startIcon}</span>;
	}
	if (result.imageUrl !== undefined) {
		return (
			<span aria-hidden>
				<Avatar src={result.imageUrl} fallback={result.label} size="md" />
			</span>
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
	const valueSuggestionsByCategory = Map.groupBy(
		valueSuggestions,
		(suggestion) => suggestion.categoryLabel,
	);

	const isEmpty =
		listedCategories.length === 0 &&
		valueSuggestions.length === 0 &&
		!showSearchSection &&
		!typeaheadLoading &&
		!typeaheadError;

	return (
		<>
			{isEmpty && <FilterComboboxEmpty>No filters found.</FilterComboboxEmpty>}
			<FilterComboboxList className="p-3">
				{listedCategories.map((category) => (
					<FilterComboboxItem
						className={OPTION_ITEM_CLASS}
						key={category.key}
						value={category.key}
						onSelect={() => onSelectCategory(category.key)}
					>
						{category.icon && (
							<span
								aria-hidden
								className="flex size-icon-sm shrink-0 items-center justify-center text-content-secondary [&>svg]:size-icon-sm"
							>
								{category.icon}
							</span>
						)}
						<span className="text-content-primary">{category.label}</span>
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
										<span aria-hidden>{suggestion.option.startIcon}</span>
									) : null}
									<span className="text-content-primary">
										{suggestion.option.label}
									</span>
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
								{resultIcon(result)}
								<span className="flex min-w-0 flex-col">
									<span className="truncate text-content-primary">
										{result.label}
									</span>
									{result.subtitle && (
										<span className="truncate text-xs text-content-secondary">
											{result.subtitle}
										</span>
									)}
								</span>
							</FilterComboboxItem>
						))}
					</FilterComboboxGroup>
				)}
				{typeaheadLoading && (
					<div
						className="flex items-center justify-center px-2 py-2.5"
						aria-hidden
					>
						<Spinner loading size="sm" />
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
			<FilterComboboxList className="p-3">
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
										<span aria-hidden>{option.startIcon}</span>
									) : null}
									<span className="text-content-primary">{option.label}</span>
								</FilterComboboxItem>
							);
						})}
					</FilterComboboxGroup>
				)}
			</FilterComboboxList>
		</>
	);
}
