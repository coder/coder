import { ListFilterIcon, SearchIcon } from "lucide-react";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	InputGroupAddon,
	InputGroupButton,
} from "#/components/InputGroup/InputGroup";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Combobox,
	ComboboxChip,
	ComboboxChips,
	ComboboxChipsInput,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxGroup,
	ComboboxInputGroup,
	ComboboxItem,
	ComboboxLabel,
	ComboboxList,
	ComboboxStatus,
	ComboboxValue,
} from "./Combobox";
import { chipToken } from "./filterQuery";
import type { FilterCategory, FilterOption, SearchResult } from "./types";
import { useFilterCombobox } from "./useFilterCombobox";

type FilterComboboxProps = Readonly<{
	value: string;
	onChange: (query: string) => void;
	categories: readonly FilterCategory[];
	placeholder?: string;
	className?: string;
	/** Marks the input invalid (e.g. the server rejected the filter query). */
	invalid?: boolean;
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
		<Combobox
			open={open}
			onDismiss={actions.dismiss}
			value={chipValues}
			onRemoveValue={actions.removeChip}
			inputValue={inputValue}
			onInputValueChange={actions.onInputValueChange}
			onItemHighlighted={actions.onItemHighlighted}
			label={placeholder}
		>
			<ComboboxInputGroup className={className}>
				<InputGroupAddon className="min-h-10">
					<SearchIcon aria-hidden className="size-icon-sm" />
				</InputGroupAddon>
				<ComboboxChips>
					<ComboboxValue>
						{(selected: string[]) => (
							<>
								{selected.map((token) => (
									<ComboboxChip key={token}>{token}</ComboboxChip>
								))}
								{activeCategory && committedFreeText.length > 0 && (
									<Badge
										variant="outline"
										size="md"
										data-slot="combobox-chip-search"
										className="font-medium"
										aria-hidden
									>
										{committedFreeText}
									</Badge>
								)}
								{activeCategory && (
									<Badge
										variant="dashed"
										size="md"
										data-slot="combobox-chip-draft"
										className="font-medium"
										aria-hidden
									>
										{activeCategory.key}:
									</Badge>
								)}
								<ComboboxChipsInput
									ref={actions.setInputRef}
									aria-label={placeholder}
									aria-invalid={invalid || undefined}
									placeholder={
										selected.length > 0 || activeCategory ? "" : placeholder
									}
									onFocus={actions.onInputFocus}
									onKeyDown={actions.onInputKeyDown}
								/>
							</>
						)}
					</ComboboxValue>
				</ComboboxChips>
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
			</ComboboxInputGroup>
			<ComboboxContent>
				{/* Keep mounted so polite status announcements stay consistent. */}
				<ComboboxStatus>{statusMessage}</ComboboxStatus>
				{typeahead.active ? (
					<TypeaheadList
						listedCategories={listedCategories}
						valueSuggestions={valueSuggestions}
						searchResults={searchResults}
						searchResultsLabel={searchResultsLabel}
						showSearchSection={typeahead.showSearchResults}
						showValueSuggestions={typeahead.showValueSuggestions}
						typeaheadLoading={typeahead.loading}
						typeaheadError={typeahead.error}
						onSelectCategory={actions.selectCategory}
						onSelectSuggestion={actions.selectValueSuggestion}
						onSelectSearchResult={actions.selectSearchResult}
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
			</ComboboxContent>
		</Combobox>
	);
}

const OPTION_ITEM_CLASS = "gap-2 px-2 py-2.5";

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
	showValueSuggestions: boolean;
	typeaheadLoading: boolean;
	typeaheadError: boolean;
	onSelectCategory: (categoryKey: string) => void;
	onSelectSuggestion: (token: string) => void;
	onSelectSearchResult: (result: SearchResult) => void;
}>;

function TypeaheadList({
	listedCategories,
	valueSuggestions,
	searchResults,
	searchResultsLabel,
	showSearchSection,
	showValueSuggestions,
	typeaheadLoading,
	typeaheadError,
	onSelectCategory,
	onSelectSuggestion,
	onSelectSearchResult,
}: TypeaheadListProps) {
	const valueSuggestionsByCategory = new Map<string, ValueSuggestion[]>();
	for (const suggestion of valueSuggestions) {
		const group =
			valueSuggestionsByCategory.get(suggestion.categoryLabel) ?? [];
		group.push(suggestion);
		valueSuggestionsByCategory.set(suggestion.categoryLabel, group);
	}

	const isEmpty =
		listedCategories.length === 0 &&
		!showValueSuggestions &&
		!showSearchSection &&
		!typeaheadLoading &&
		!typeaheadError;

	return (
		<>
			{isEmpty && <ComboboxEmpty>No filters found.</ComboboxEmpty>}
			<ComboboxList className="p-3 data-[empty]:p-3">
				{listedCategories.map((category) => (
					<ComboboxItem
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
					</ComboboxItem>
				))}
				{[...valueSuggestionsByCategory.entries()].map(
					([categoryLabel, suggestions]) => (
						<ComboboxGroup key={categoryLabel}>
							<ComboboxLabel>{categoryLabel}</ComboboxLabel>
							{suggestions.map((suggestion) => (
								<ComboboxItem
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
								</ComboboxItem>
							))}
						</ComboboxGroup>
					),
				)}
				{showSearchSection && (
					<ComboboxGroup>
						<ComboboxLabel>{searchResultsLabel}</ComboboxLabel>
						{searchResults.map((result) => (
							<ComboboxItem
								className={OPTION_ITEM_CLASS}
								key={result.value}
								value={result.value}
								onSelect={() => onSelectSearchResult(result)}
							>
								{(result.startIcon ?? result.imageUrl !== undefined) ? (
									<span aria-hidden>
										{result.startIcon ??
											(result.imageUrl !== undefined ? (
												<Avatar
													src={result.imageUrl}
													fallback={result.label}
													size="md"
												/>
											) : null)}
									</span>
								) : null}
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
							</ComboboxItem>
						))}
					</ComboboxGroup>
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
					<div className="px-2 py-2.5 text-center text-sm text-content-secondary">
						Couldn&rsquo;t load suggestions.
					</div>
				)}
			</ComboboxList>
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
			<ComboboxEmpty>No filters found.</ComboboxEmpty>
			<ComboboxList className="p-3 data-[empty]:p-3">
				{activeCategoryKey !== null && (
					<ComboboxGroup>
						{activeCategory && (
							<ComboboxLabel>{activeCategory.label}</ComboboxLabel>
						)}
						{activeOptions.map((option) => {
							const item =
								option.token ?? chipToken(activeCategoryKey, option.value);
							return (
								<ComboboxItem
									className={OPTION_ITEM_CLASS}
									key={item}
									value={item}
									onSelect={() => onSelectOption(item)}
								>
									{option.startIcon ? (
										<span aria-hidden>{option.startIcon}</span>
									) : null}
									<span className="text-content-primary">{option.label}</span>
								</ComboboxItem>
							);
						})}
					</ComboboxGroup>
				)}
			</ComboboxList>
		</>
	);
}
