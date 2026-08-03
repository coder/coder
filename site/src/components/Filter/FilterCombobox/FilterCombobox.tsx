import { ListFilterIcon, SearchIcon } from "lucide-react";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import type { UseFilterResult } from "#/components/Filter/Filter";
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
	ComboboxTrigger,
	ComboboxValue,
} from "./Combobox";
import type { FilterFacet, FilterSearchResult } from "./types";
import { searchResultToken } from "./types";
import { useFilterCombobox } from "./useFilterCombobox";

type FilterComboboxProps<Id extends string = string> = Readonly<{
	filter: UseFilterResult;
	facets: readonly FilterFacet<Id>[];
	/** Stable chip key order for URL serialization. Defaults to facet ids. */
	chipKeys?: readonly Id[];
	placeholder?: string;
	className?: string;
	/** Debounced free-text resource previews (e.g. matching workspaces). */
	getSearchResults?: (query: string) => Promise<FilterSearchResult[]>;
	onSearchResultSelect?: (result: FilterSearchResult) => void;
	searchResultsLabel?: string;
}>;

export function FilterCombobox<Id extends string = string>({
	filter,
	facets,
	chipKeys,
	placeholder = "Search and filter…",
	className,
	getSearchResults,
	onSearchResultSelect,
	searchResultsLabel = "Results",
}: FilterComboboxProps<Id>) {
	const {
		open,
		browseMode,
		inputValue,
		committedFreeText,
		activeFacet,
		activeFacetMeta,
		activeOptions,
		listedFacets,
		searchResults,
		searchResultsLoading,
		chipValues,
		optionItems,
		optionByToken,
		selectFacet,
		handleInputFocus,
		handleInputKeyDown,
		handleInputValueChange,
		handleOpenChange,
		handleValueChange,
	} = useFilterCombobox({
		filter,
		facets,
		chipKeys,
		getSearchResults,
		onSearchResultSelect,
	});

	const showTypeahead = activeFacet === null && browseMode === "typeahead";
	const showSearchSection =
		showTypeahead &&
		inputValue.trim().length > 0 &&
		Boolean(getSearchResults) &&
		(searchResultsLoading || searchResults.length > 0);

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
					<SearchIcon className="size-icon-sm" />
				</InputGroupAddon>
				<ComboboxChips>
					<ComboboxValue>
						{(selected: string[]) => (
							<>
								{selected.map((token) => (
									<ComboboxChip key={token}>{token}</ComboboxChip>
								))}
								{activeFacetMeta && committedFreeText.length > 0 && (
									<Badge
										variant="outline"
										size="md"
										data-slot="combobox-chip-search"
										className="font-medium"
									>
										{committedFreeText}
									</Badge>
								)}
								{activeFacetMeta && (
									<Badge
										variant="dashed"
										size="md"
										data-slot="combobox-chip-draft"
										className="font-medium"
									>
										{activeFacetMeta.id}:
									</Badge>
								)}
								<ComboboxChipsInput
									placeholder={
										selected.length > 0 || activeFacetMeta ? "" : placeholder
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
					<ComboboxTrigger
						render={
							<InputGroupButton
								variant="subtle"
								aria-label="Toggle filters"
								className="h-full min-h-10 w-10 min-w-10 shrink-0 rounded-none rounded-r-md px-0 [&>svg]:p-0"
							/>
						}
					>
						<ListFilterIcon className="size-icon-sm" />
					</ComboboxTrigger>
				</InputGroupAddon>
			</ComboboxInputGroup>
			<ComboboxContent>
				{activeFacet === null && browseMode === "trigger" ? (
					<div className="flex flex-col gap-2 p-4">
						<span className="text-sm font-normal text-content-secondary">
							Filter by
						</span>
						<div className="flex flex-wrap gap-2">
							{facets.map((facet) => {
								const Icon = facet.icon;
								return (
									<Badge
										key={facet.id}
										asChild
										variant="outline"
										size="md"
										svgSize="sm"
										hover
										className="px-1.5 py-0.5 text-xs font-medium"
									>
										<button
											type="button"
											onMouseDown={(event) => {
												event.preventDefault();
											}}
											onClick={() => selectFacet(facet.id)}
										>
											<Icon />
											{facet.label}
										</button>
									</Badge>
								);
							})}
						</div>
					</div>
				) : showTypeahead ? (
					<>
						{listedFacets.length === 0 &&
							!showSearchSection &&
							!searchResultsLoading && (
								<ComboboxEmpty>No filters found.</ComboboxEmpty>
							)}
						<ComboboxList className="p-3 data-[empty]:p-3">
							{listedFacets.map((facet) => {
								const Icon = facet.icon;
								return (
									<ComboboxItem
										className="gap-2 px-2 py-2.5"
										key={facet.id}
										value={facet.id}
										showIndicator={false}
									>
										<Icon className="size-icon-sm text-content-secondary" />
										<span className="text-content-primary">{facet.label}</span>
									</ComboboxItem>
								);
							})}
							{showSearchSection && (
								<ComboboxGroup>
									<ComboboxLabel>{searchResultsLabel}</ComboboxLabel>
									{searchResultsLoading && searchResults.length === 0 ? (
										<div className="flex items-center justify-center px-2 py-2.5">
											<Spinner loading size="sm" />
										</div>
									) : (
										searchResults.map((result) => (
											<ComboboxItem
												className="gap-2 px-2 py-2.5"
												key={result.id}
												value={searchResultToken(result.id)}
												showIndicator={false}
											>
												{result.startIcon ??
													(result.imageUrl !== undefined ? (
														<Avatar
															src={result.imageUrl}
															fallback={result.label}
															size="md"
														/>
													) : null)}
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
				) : activeOptions === undefined ? (
					<div className="px-3 py-6 text-center text-sm text-content-secondary">
						Loading…
					</div>
				) : (
					<>
						<ComboboxEmpty>No filters found.</ComboboxEmpty>
						<ComboboxList className="p-3">
							{(item) => {
								const option = optionByToken.get(item);
								return (
									<ComboboxItem
										className="gap-2 px-2 py-2.5"
										key={item}
										value={item}
										showIndicator={false}
									>
										{option?.startIcon}
										<span className="text-content-primary">
											{option?.label ?? item}
										</span>
									</ComboboxItem>
								);
							}}
						</ComboboxList>
					</>
				)}
			</ComboboxContent>
		</Combobox>
	);
}
