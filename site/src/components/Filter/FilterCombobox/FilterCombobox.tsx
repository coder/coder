import { ListFilterIcon, SearchIcon } from "lucide-react";
import { Badge } from "#/components/Badge/Badge";
import type { UseFilterResult } from "#/components/Filter/Filter";
import {
	InputGroupAddon,
	InputGroupButton,
} from "#/components/InputGroup/InputGroup";
import {
	Combobox,
	ComboboxChip,
	ComboboxChips,
	ComboboxChipsInput,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInputGroup,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
	ComboboxValue,
} from "./Combobox";
import type { FilterFacet } from "./types";
import { useFilterCombobox } from "./useFilterCombobox";

type FilterComboboxProps<Id extends string = string> = Readonly<{
	filter: UseFilterResult;
	facets: readonly FilterFacet<Id>[];
	/** Stable chip key order for URL serialization. Defaults to facet ids. */
	chipKeys?: readonly Id[];
	placeholder?: string;
	className?: string;
}>;

export function FilterCombobox<Id extends string = string>({
	filter,
	facets,
	chipKeys,
	placeholder = "Search and filter…",
	className,
}: FilterComboboxProps<Id>) {
	const {
		open,
		inputValue,
		committedFreeText,
		activeFacet,
		activeFacetMeta,
		activeOptions,
		chipValues,
		optionItems,
		optionByToken,
		selectFacet,
		exitActiveFacet,
		handleInputValueChange,
		handleOpenChange,
		handleValueChange,
	} = useFilterCombobox({ filter, facets, chipKeys });

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
									onKeyDown={(event) => {
										if (
											event.key === "Backspace" &&
											inputValue === "" &&
											activeFacetMeta
										) {
											event.preventDefault();
											exitActiveFacet();
										}
									}}
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
				{activeFacet === null ? (
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
										className="px-2 py-2.5"
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
