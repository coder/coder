import { FilterIcon, SearchIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import { cn } from "#/utils/cn";

type GroupByOption = "date" | "chat-status";
export type PRStatusFilter = "draft" | "open" | "merged" | "closed";
export type ChatStatusFilter =
	| "unread"
	| "running"
	| "awaiting-feedback"
	| "idle"
	| "error"
	| "archived";

type ArchivedFilter = "active" | "archived";

export interface SidebarFilterState {
	groupBy: GroupByOption;
	prStatus: ReadonlySet<PRStatusFilter>;
	chatStatus: ReadonlySet<ChatStatusFilter>;
}

export const DEFAULT_FILTER_STATE: SidebarFilterState = {
	groupBy: "date",
	prStatus: new Set<PRStatusFilter>(),
	chatStatus: new Set<ChatStatusFilter>(),
};

export const hasActiveFilters = (filterState: SidebarFilterState): boolean => {
	return filterState.prStatus.size > 0 || filterState.chatStatus.size > 0;
};

type FilterOption<T extends string> = { value: T; label: string };

const PR_STATUS_OPTIONS = [
	{ value: "draft", label: "Draft" },
	{ value: "open", label: "Open" },
	{ value: "merged", label: "Merged" },
	{ value: "closed", label: "Closed" },
] satisfies readonly FilterOption<PRStatusFilter>[];

const CHAT_STATUS_OPTIONS = [
	{ value: "unread", label: "Unread" },
	{ value: "running", label: "Running" },
	{ value: "awaiting-feedback", label: "Awaiting feedback" },
	{ value: "idle", label: "Idle" },
	{ value: "error", label: "Error" },
	{ value: "archived", label: "Archived" },
] satisfies readonly FilterOption<ChatStatusFilter>[];

const isGroupByOption = (value: string): value is GroupByOption => {
	return value === "date" || value === "chat-status";
};

const isArchivedFilter = (value: string): value is ArchivedFilter => {
	return value === "active" || value === "archived";
};

interface FilterDropdownProps {
	readonly archivedFilter: ArchivedFilter;
	readonly onArchivedFilterChange?: (filter: ArchivedFilter) => void;
	readonly filterState: SidebarFilterState;
	readonly onFilterStateChange: (state: SidebarFilterState) => void;
	readonly filteredCount: number;
	readonly totalRootCount: number;
	readonly prStatusCounts: ReadonlyMap<string, number>;
	readonly chatStatusCounts: ReadonlyMap<string, number>;
}

export const FilterDropdown: FC<FilterDropdownProps> = ({
	archivedFilter,
	onArchivedFilterChange,
	filterState,
	onFilterStateChange,
	filteredCount,
	totalRootCount,
	prStatusCounts,
	chatStatusCounts,
}) => {
	const [filterSearch, setFilterSearch] = useState("");
	const showFilterCount = hasActiveFilters(filterState) && totalRootCount > 0;

	return (
		<Popover>
			<PopoverTrigger asChild>
				<button
					type="button"
					aria-label="Filter agents"
					className={cn(
						"inline-flex cursor-pointer items-center gap-1 rounded-md border-0 bg-transparent p-0 text-content-secondary hover:text-content-primary",
						(archivedFilter === "archived" ||
							hasActiveFilters(filterState) ||
							filterState.groupBy !== "date") &&
							"text-content-primary",
					)}
				>
					<span
						className={cn(
							"text-xs text-content-secondary",
							!showFilterCount && "invisible",
						)}
					>
						({filteredCount} of {totalRootCount})
					</span>
					<FilterIcon className="h-3.5 w-3.5" />
				</button>
			</PopoverTrigger>
			<PopoverContent
				align="end"
				sideOffset={2}
				alignOffset={6}
				className="w-[225px] overflow-y-hidden p-0"
				onPointerDownOutside={() => setFilterSearch("")}
				onEscapeKeyDown={() => setFilterSearch("")}
			>
				<div className="flex flex-col">
					<div className="border-0 border-b border-solid border-border-default px-3 py-2.5">
						<span className="mb-1.5 block text-xs font-medium text-content-secondary">
							Show
						</span>
						<RadioGroup
							value={archivedFilter}
							onValueChange={(value) => {
								if (isArchivedFilter(value)) {
									onArchivedFilterChange?.(value);
								}
							}}
							className="gap-1.5"
						>
							<div className="flex items-center gap-2">
								<RadioGroupItem value="active" id="filter-active-agents" />
								<label
									htmlFor="filter-active-agents"
									className="cursor-pointer text-[13px] text-content-primary"
								>
									Active agents
								</label>
							</div>
							<div className="flex items-center gap-2">
								<RadioGroupItem value="archived" id="filter-archived-agents" />
								<label
									htmlFor="filter-archived-agents"
									className="cursor-pointer text-[13px] text-content-primary"
								>
									Archived agents
								</label>
							</div>
						</RadioGroup>
					</div>

					<div className="border-0 border-b border-solid border-border-default px-3 py-2.5">
						<span className="mb-1.5 block text-xs font-medium text-content-secondary">
							Group
						</span>
						<RadioGroup
							value={filterState.groupBy}
							onValueChange={(value) => {
								if (isGroupByOption(value)) {
									onFilterStateChange({
										...filterState,
										groupBy: value,
									});
								}
							}}
							className="gap-1.5"
						>
							<div className="flex items-center gap-2">
								<RadioGroupItem value="date" id="group-date" />
								<label
									htmlFor="group-date"
									className="cursor-pointer text-[13px] text-content-primary"
								>
									Date
								</label>
							</div>
							<div className="flex items-center gap-2">
								<RadioGroupItem value="chat-status" id="group-status" />
								<label
									htmlFor="group-status"
									className="cursor-pointer text-[13px] text-content-primary"
								>
									Chat status
								</label>
							</div>
						</RadioGroup>
					</div>

					<div className="px-3 pt-2.5 pb-0">
						<span className="mb-1.5 block text-xs font-medium text-content-secondary">
							Filter by
						</span>
						<div className="relative mb-2">
							<SearchIcon className="pointer-events-none absolute top-1/2 left-2 h-3.5 w-3.5 -translate-y-1/2 text-content-secondary" />
							<input
								type="text"
								placeholder="Search filters..."
								value={filterSearch}
								onChange={(e) => setFilterSearch(e.target.value)}
								className="h-7 w-full rounded-md border border-solid border-border bg-transparent pl-7 pr-2 text-xs text-content-primary placeholder:text-content-disabled focus:border-border-hover focus:outline-none"
							/>
						</div>
						<div className="mt-1 max-h-40 overflow-y-auto pb-2 [scrollbar-color:initial] [&::-webkit-scrollbar]:w-[6px] [&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-surface-quaternary [&::-webkit-scrollbar-track]:rounded-full [&::-webkit-scrollbar-track]:bg-surface-secondary">
							<FilterCheckboxSection
								title="PR status"
								options={PR_STATUS_OPTIONS}
								selected={filterState.prStatus}
								counts={prStatusCounts}
								searchTerm={filterSearch}
								onChange={(value, checked) => {
									const next = new Set(filterState.prStatus);
									if (checked) {
										next.add(value);
									} else {
										next.delete(value);
									}
									onFilterStateChange({ ...filterState, prStatus: next });
								}}
							/>
							<FilterCheckboxSection
								title="Chat status"
								options={CHAT_STATUS_OPTIONS}
								selected={filterState.chatStatus}
								counts={chatStatusCounts}
								searchTerm={filterSearch}
								onChange={(value, checked) => {
									const next = new Set(filterState.chatStatus);
									if (checked) {
										next.add(value);
									} else {
										next.delete(value);
									}
									onFilterStateChange({ ...filterState, chatStatus: next });
								}}
							/>
						</div>
					</div>

					<div className="border-0 border-t border-solid border-border-default px-3 py-2">
						<button
							type="button"
							className="cursor-pointer border-0 bg-transparent p-0 text-xs text-content-secondary hover:text-content-primary"
							onClick={() => {
								setFilterSearch("");
								onFilterStateChange(DEFAULT_FILTER_STATE);
							}}
						>
							Clear all
						</button>
					</div>
				</div>
			</PopoverContent>
		</Popover>
	);
};

interface FilterCheckboxSectionProps<T extends string> {
	readonly title: string;
	readonly options: readonly FilterOption<T>[];
	readonly selected: ReadonlySet<T>;
	readonly counts?: ReadonlyMap<string, number>;
	readonly searchTerm: string;
	readonly onChange: (value: T, checked: boolean) => void;
}

function FilterCheckboxSection<T extends string>({
	title,
	options,
	selected,
	counts,
	searchTerm,
	onChange,
}: FilterCheckboxSectionProps<T>) {
	const normalizedTerm = searchTerm.toLowerCase();
	const filtered = normalizedTerm
		? options.filter((option) =>
				option.label.toLowerCase().includes(normalizedTerm),
			)
		: options;
	if (filtered.length === 0) return null;
	return (
		<div className="mb-2 last:mb-0">
			<span className="mb-1 block text-xs text-content-secondary">{title}</span>
			<div className="flex flex-col gap-1.5">
				{filtered.map((option) => (
					<div
						key={option.value}
						className="flex cursor-pointer items-center gap-2 text-[13px] text-content-primary"
					>
						<Checkbox
							checked={selected.has(option.value)}
							onCheckedChange={(checked) =>
								onChange(option.value, checked === true)
							}
						/>
						{option.label}{" "}
						{counts && (
							<span className="text-content-disabled">
								({counts.get(option.value) ?? 0})
							</span>
						)}
					</div>
				))}
			</div>
		</div>
	);
}
