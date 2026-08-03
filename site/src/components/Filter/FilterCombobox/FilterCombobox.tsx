import { ListFilterIcon, SearchIcon } from "lucide-react";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
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
import type { FilterCategory, SearchResult } from "./types";
import { searchResultToken } from "./types";
import { useFilterCombobox } from "./useFilterCombobox";

type FilterComboboxProps = Readonly<{
	value: string;
	onChange: (query: string) => void;
	categories: readonly FilterCategory[];
	placeholder?: string;
	className?: string;
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
	getSearchResults,
	onSearchResultSelect,
	searchResultsLabel = "Results",
}: FilterComboboxProps) {
	const {
		open,
		browseMode,
		inputValue,
		committedFreeText,
		activeCategoryKey,
		activeCategory,
		activeOptions,
		activeOptionsLoading,
		listedCategories,
		valueSuggestions,
		valueSuggestionsLoading,
		searchResults,
		searchResultsLoading,
		chipValues,
		optionItems,
		toggleFilterMenu,
		setInputRef,
		handleInputFocus,
		handleInputKeyDown,
		handleInputValueChange,
		handleOpenChange,
		handleValueChange,
	} = useFilterCombobox({
		value,
		onChange,
		categories,
		getSearchResults,
		onSearchResultSelect,
	});

	const showTypeahead =
		activeCategoryKey === null && browseMode === "typeahead";
	const showValueSuggestions =
		showTypeahead &&
		inputValue.trim().length > 0 &&
		(valueSuggestionsLoading || valueSuggestions.length > 0);
	const showSearchSection =
		showTypeahead &&
		inputValue.trim().length > 0 &&
		Boolean(getSearchResults) &&
		(searchResultsLoading || searchResults.length > 0);

	const valueSuggestionsByCategory = new Map<string, typeof valueSuggestions>();
	for (const suggestion of valueSuggestions) {
		const group =
			valueSuggestionsByCategory.get(suggestion.categoryLabel) ?? [];
		group.push(suggestion);
		valueSuggestionsByCategory.set(suggestion.categoryLabel, group);
	}

	const statusMessage = (() => {
		if (activeCategory) {
			if (activeOptionsLoading) {
				return `Loading ${activeCategory.label} options`;
			}
			return `Filtering by ${activeCategory.label}`;
		}
		if (!showTypeahead) {
			return "";
		}
		if (
			(valueSuggestionsLoading && valueSuggestions.length === 0) ||
			(searchResultsLoading && searchResults.length === 0)
		) {
			return "Loading suggestions";
		}
		return "";
	})();

	return (
		<Combobox
			multiple
			autoHighlight="always"
			filter={null}
			openOnInputClick={false}
			open={open}
			onOpenChange={handleOpenChange}
			value={chipValues}
			onValueChange={handleValueChange}
			inputValue={inputValue}
			onInputValueChange={handleInputValueChange}
			items={optionItems}
		>
			<ComboboxInputGroup className={className}>
				<InputGroupAddon>
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
									ref={setInputRef}
									aria-label={placeholder}
									placeholder={
										selected.length > 0 || activeCategory ? "" : placeholder
									}
									onFocus={handleInputFocus}
									onKeyDown={handleInputKeyDown}
								/>
							</>
						)}
					</ComboboxValue>
				</ComboboxChips>
				<InputGroupAddon
					align="inline-end"
					className="w-10 self-stretch border-0 border-l border-solid border-border p-0"
				>
					<InputGroupButton
						type="button"
						variant="subtle"
						aria-label="Toggle filters"
						aria-expanded={open}
						aria-haspopup="listbox"
						className="h-full min-h-10 w-10 min-w-10 shrink-0 rounded-none rounded-r-md px-0 [&>svg]:p-0"
						onMouseDown={(event) => {
							// Prevent the button from taking focus on pointer open.
							// toggleFilterMenu focuses the combobox input next so
							// aria-activedescendant keyboard navigation still works.
							event.preventDefault();
						}}
						onClick={toggleFilterMenu}
					>
						<ListFilterIcon aria-hidden className="size-icon-sm" />
					</InputGroupButton>
				</InputGroupAddon>
			</ComboboxInputGroup>
			<ComboboxContent>
				{/* Keep mounted so polite status announcements stay consistent. */}
				<ComboboxStatus>{statusMessage}</ComboboxStatus>
				{showTypeahead ? (
					<>
						{listedCategories.length === 0 &&
							!showValueSuggestions &&
							!showSearchSection &&
							!valueSuggestionsLoading &&
							!searchResultsLoading && (
								<ComboboxEmpty>No filters found.</ComboboxEmpty>
							)}
						<ComboboxList className="p-3 data-[empty]:p-3">
							{listedCategories.map((category) => (
								<ComboboxItem
									className="gap-2 px-2 py-2.5"
									key={category.key}
									value={category.key}
									showIndicator={false}
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
							{showValueSuggestions &&
								(valueSuggestionsLoading && valueSuggestions.length === 0 ? (
									<div
										className="flex items-center justify-center px-2 py-2.5"
										aria-hidden
									>
										<Spinner loading size="sm" />
									</div>
								) : (
									[...valueSuggestionsByCategory.entries()].map(
										([categoryLabel, suggestions]) => (
											<ComboboxGroup key={categoryLabel}>
												<ComboboxLabel>{categoryLabel}</ComboboxLabel>
												{suggestions.map((suggestion) => (
													<ComboboxItem
														className="gap-2 px-2 py-2.5"
														key={suggestion.token}
														value={suggestion.token}
														showIndicator={false}
													>
														{suggestion.option.startIcon ? (
															<span aria-hidden>
																{suggestion.option.startIcon}
															</span>
														) : null}
														<span className="text-content-primary">
															{suggestion.option.label}
														</span>
													</ComboboxItem>
												))}
											</ComboboxGroup>
										),
									)
								))}
							{showSearchSection && (
								<ComboboxGroup>
									<ComboboxLabel>{searchResultsLabel}</ComboboxLabel>
									{searchResultsLoading && searchResults.length === 0 ? (
										<div
											className="flex items-center justify-center px-2 py-2.5"
											aria-hidden
										>
											<Spinner loading size="sm" />
										</div>
									) : (
										searchResults.map((result) => (
											<ComboboxItem
												className="gap-2 px-2 py-2.5"
												key={result.value}
												value={searchResultToken(result.value)}
												showIndicator={false}
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
										))
									)}
								</ComboboxGroup>
							)}
						</ComboboxList>
					</>
				) : activeOptionsLoading || activeOptions === undefined ? (
					<div
						className="px-3 py-6 text-center text-sm text-content-secondary"
						aria-hidden
					>
						Loading…
					</div>
				) : (
					<>
						<ComboboxEmpty>No filters found.</ComboboxEmpty>
						<ComboboxList className="p-3 data-[empty]:p-3">
							{activeCategoryKey !== null && (
								<ComboboxGroup>
									{activeCategory && (
										<ComboboxLabel>{activeCategory.label}</ComboboxLabel>
									)}
									{activeOptions.map((option) => {
										const item = chipToken(activeCategoryKey, option.value);
										return (
											<ComboboxItem
												className="gap-2 px-2 py-2.5"
												key={item}
												value={item}
												showIndicator={false}
											>
												{option.startIcon ? (
													<span aria-hidden>{option.startIcon}</span>
												) : null}
												<span className="text-content-primary">
													{option.label}
												</span>
											</ComboboxItem>
										);
									})}
								</ComboboxGroup>
							)}
						</ComboboxList>
					</>
				)}
			</ComboboxContent>
		</Combobox>
	);
}
