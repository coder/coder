import { cn } from "cn";
import { CheckIcon, ListFilterIcon, RotateCcwIcon, XIcon } from "lucide-react";
import { DropdownMenu as DropdownMenuPrimitive } from "radix-ui";
import type { ComponentProps, FC, ReactNode } from "react";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { menuItemClass } from "#/components/DropdownMenu/menuClasses";
import {
	AGENT_ARCHIVE_STATUS_ORDER,
	AGENT_CHAT_STATUS_ORDER,
	AGENT_PR_STATUS_ORDER,
	AGENT_TIME_RANGE_ORDER,
	type AgentArchiveStatusFilter,
	type AgentPRStatusFilter,
	type AgentSidebarFilters,
	type AgentSidebarGroupBy,
	type AgentSourceFilter,
	type AgentTimeRangeFilter,
	DEFAULT_AGENT_SIDEBAR_FILTERS,
} from "../../../utils/agentSidebarFilters";

const GROUP_BY_ORDER: readonly AgentSidebarGroupBy[] = ["date", "chat_status"];

// Submenus cannot open beside a full-width mobile menu, so on small
// screens they overlay it at the same position instead.
const MOBILE_MENU_CLASS =
	"mobile-full-width-dropdown mobile-full-width-dropdown-top-below-header";

const GROUP_BY_LABELS: Record<AgentSidebarGroupBy, string> = {
	date: "Date",
	chat_status: "Status",
};

const ARCHIVE_STATUS_LABELS: Record<AgentArchiveStatusFilter, string> = {
	active: "Active",
	archived: "Archived",
};

type OwnerFilter = "mine" | "shared_with_me" | "all";

const OWNER_ORDER: readonly OwnerFilter[] = ["mine", "shared_with_me", "all"];

const OWNER_LABELS: Record<OwnerFilter, string> = {
	mine: "Mine",
	shared_with_me: "Shared with me",
	all: "All",
};

const OWNER_SOURCES: Record<OwnerFilter, readonly AgentSourceFilter[]> = {
	mine: ["created_by_me"],
	shared_with_me: ["shared_with_me"],
	all: ["created_by_me", "shared_with_me"],
};

const TIME_RANGE_LABELS: Record<AgentTimeRangeFilter, string> = {
	"1d": "1 day",
	"7d": "7 days",
	"15d": "15 days",
	"30d": "30 days",
	all: "All",
};

const PR_STATUS_LABELS: Record<AgentPRStatusFilter, string> = {
	draft: "PR: draft",
	open: "PR: open",
	merged: "PR: merged",
	closed: "PR: closed",
};

const ownerFromSources = (
	sources: readonly AgentSourceFilter[],
): OwnerFilter => {
	const ownsChats = sources.includes("created_by_me");
	const sharedChats = sources.includes("shared_with_me");
	if (ownsChats && sharedChats) {
		return "all";
	}
	return sharedChats ? "shared_with_me" : "mine";
};

const isGroupBy = (value: string): value is AgentSidebarGroupBy =>
	GROUP_BY_ORDER.some((groupBy) => groupBy === value);

const isArchiveStatus = (value: string): value is AgentArchiveStatusFilter =>
	AGENT_ARCHIVE_STATUS_ORDER.some((status) => status === value);

const isOwner = (value: string): value is OwnerFilter =>
	OWNER_ORDER.some((owner) => owner === value);

const isTimeRange = (value: string): value is AgentTimeRangeFilter =>
	AGENT_TIME_RANGE_ORDER.some((range) => range === value);

const haveSameSelections = <T extends string>(
	left: readonly T[],
	right: readonly T[],
): boolean => {
	return (
		left.length === right.length && left.every((value) => right.includes(value))
	);
};

const hasActiveFilters = (filters: AgentSidebarFilters): boolean => {
	return (
		filters.archiveStatus !== DEFAULT_AGENT_SIDEBAR_FILTERS.archiveStatus ||
		filters.groupBy !== DEFAULT_AGENT_SIDEBAR_FILTERS.groupBy ||
		filters.timeRange !== DEFAULT_AGENT_SIDEBAR_FILTERS.timeRange ||
		filters.prStatuses.length > 0 ||
		!haveSameSelections(
			filters.chatStatuses,
			DEFAULT_AGENT_SIDEBAR_FILTERS.chatStatuses,
		) ||
		!haveSameSelections(filters.sources, DEFAULT_AGENT_SIDEBAR_FILTERS.sources)
	);
};

// Radix closes the menu on select; keep it open so several filters can be
// changed in one pass.
const keepMenuOpen = (event: Event) => {
	event.preventDefault();
};

type AdvancedFilterOption = Readonly<{
	key: string;
	label: string;
	checked: boolean;
	setChecked: (checked: boolean) => void;
}>;

// Draws the checkbox visually instead of rendering Checkbox, which would
// nest a second interactive control inside the menu item.
const MultiSelectMenuItem: FC<
	ComponentProps<typeof DropdownMenuPrimitive.CheckboxItem>
> = ({ className, children, ...props }) => (
	<DropdownMenuPrimitive.CheckboxItem
		className={cn(menuItemClass, "group gap-3", className)}
		{...props}
	>
		<span
			className={cn(
				"flex size-[18px] shrink-0 items-center justify-center rounded-xs border border-solid border-border bg-surface-primary",
				"group-data-[state=checked]:border-surface-invert-primary group-data-[state=checked]:bg-surface-invert-primary group-data-[state=checked]:text-content-invert",
			)}
		>
			<DropdownMenuPrimitive.ItemIndicator>
				<CheckIcon className="size-4" strokeWidth={2.5} />
			</DropdownMenuPrimitive.ItemIndicator>
		</span>
		{children}
	</DropdownMenuPrimitive.CheckboxItem>
);

const FilterRow: FC<{
	readonly label: string;
	readonly value: ReactNode;
}> = ({ label, value }) => (
	<>
		<span className="flex-1">{label}</span>
		<span className="truncate text-content-primary">{value}</span>
	</>
);

interface FilterMenuProps {
	readonly filters: AgentSidebarFilters;
	readonly onFiltersChange: (filters: AgentSidebarFilters) => void;
}

export const FilterMenu: FC<FilterMenuProps> = ({
	filters,
	onFiltersChange,
}) => {
	const setGroupBy = (value: string) => {
		if (isGroupBy(value)) {
			onFiltersChange({ ...filters, groupBy: value });
		}
	};

	const setArchiveStatus = (value: string) => {
		if (isArchiveStatus(value)) {
			onFiltersChange({ ...filters, archiveStatus: value });
		}
	};

	const setOwner = (value: string) => {
		if (isOwner(value)) {
			onFiltersChange({ ...filters, sources: OWNER_SOURCES[value] });
		}
	};

	const setTimeRange = (value: string) => {
		if (isTimeRange(value)) {
			onFiltersChange({ ...filters, timeRange: value });
		}
	};

	const setPRStatus = (status: AgentPRStatusFilter, checked: boolean) => {
		const selected = new Set(filters.prStatuses);
		if (checked) {
			selected.add(status);
		} else {
			selected.delete(status);
		}
		onFiltersChange({
			...filters,
			prStatuses: AGENT_PR_STATUS_ORDER.filter((value) => selected.has(value)),
		});
	};

	const setUnreadOnly = (checked: boolean) => {
		onFiltersChange({
			...filters,
			chatStatuses: checked ? ["unread"] : AGENT_CHAT_STATUS_ORDER,
		});
	};

	const advancedOptions: readonly AdvancedFilterOption[] = [
		{
			key: "unread",
			label: "Unread",
			checked:
				filters.chatStatuses.length === 1 &&
				filters.chatStatuses[0] === "unread",
			setChecked: setUnreadOnly,
		},
		...AGENT_PR_STATUS_ORDER.map((status) => ({
			key: `pr-${status}`,
			label: PR_STATUS_LABELS[status],
			checked: filters.prStatuses.includes(status),
			setChecked: (checked: boolean) => setPRStatus(status, checked),
		})),
	];
	const selectedAdvancedOptions = advancedOptions.filter(
		(option) => option.checked,
	);

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					aria-label="Filter agents"
					className={cn(
						"size-7 min-w-0 -mr-0.5 justify-end px-0 text-content-secondary hover:text-content-primary",
						hasActiveFilters(filters) && "text-content-primary",
					)}
				>
					<ListFilterIcon />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				align="end"
				aria-label="Filter agents"
				className={cn(MOBILE_MENU_CLASS, "w-64 p-0")}
			>
				<div className="border-0 border-b border-solid border-border p-1">
					<DropdownMenuPrimitive.RadioGroup
						aria-label="State"
						value={filters.archiveStatus}
						onValueChange={setArchiveStatus}
						className="mb-1 flex rounded-lg bg-surface-secondary p-1"
					>
						{AGENT_ARCHIVE_STATUS_ORDER.map((status) => (
							<DropdownMenuPrimitive.RadioItem
								key={status}
								value={status}
								onSelect={keepMenuOpen}
								className={cn(
									"flex flex-1 cursor-default select-none items-center justify-center rounded-md py-1 text-sm text-content-secondary outline-hidden transition-colors",
									"focus:text-content-primary focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link",
									"data-[state=checked]:bg-surface-tertiary data-[state=checked]:text-content-primary data-[state=checked]:shadow-sm",
								)}
							>
								{ARCHIVE_STATUS_LABELS[status]}
							</DropdownMenuPrimitive.RadioItem>
						))}
					</DropdownMenuPrimitive.RadioGroup>

					<DropdownMenuSub>
						<DropdownMenuSubTrigger>
							<FilterRow
								label="Owner"
								value={OWNER_LABELS[ownerFromSources(filters.sources)]}
							/>
						</DropdownMenuSubTrigger>
						<DropdownMenuSubContent className={MOBILE_MENU_CLASS}>
							<DropdownMenuRadioGroup
								value={ownerFromSources(filters.sources)}
								onValueChange={setOwner}
							>
								{OWNER_ORDER.map((owner) => (
									<DropdownMenuRadioItem
										key={owner}
										value={owner}
										onSelect={keepMenuOpen}
									>
										{OWNER_LABELS[owner]}
									</DropdownMenuRadioItem>
								))}
							</DropdownMenuRadioGroup>
						</DropdownMenuSubContent>
					</DropdownMenuSub>

					<DropdownMenuSub>
						<DropdownMenuSubTrigger>
							<FilterRow
								label="Time range"
								value={TIME_RANGE_LABELS[filters.timeRange]}
							/>
						</DropdownMenuSubTrigger>
						<DropdownMenuSubContent className={MOBILE_MENU_CLASS}>
							<DropdownMenuRadioGroup
								value={filters.timeRange}
								onValueChange={setTimeRange}
							>
								{AGENT_TIME_RANGE_ORDER.map((range) => (
									<DropdownMenuRadioItem
										key={range}
										value={range}
										onSelect={keepMenuOpen}
									>
										{TIME_RANGE_LABELS[range]}
									</DropdownMenuRadioItem>
								))}
							</DropdownMenuRadioGroup>
						</DropdownMenuSubContent>
					</DropdownMenuSub>

					<DropdownMenuSub>
						<DropdownMenuSubTrigger>
							<FilterRow
								label="Filter by"
								value={
									selectedAdvancedOptions.length > 0
										? selectedAdvancedOptions.length
										: "None"
								}
							/>
						</DropdownMenuSubTrigger>
						<DropdownMenuSubContent
							className={cn(MOBILE_MENU_CLASS, "w-64 p-1")}
						>
							{advancedOptions.map((option) => (
								<MultiSelectMenuItem
									key={option.key}
									checked={option.checked}
									onCheckedChange={option.setChecked}
									onSelect={keepMenuOpen}
								>
									{option.label}
								</MultiSelectMenuItem>
							))}
						</DropdownMenuSubContent>
					</DropdownMenuSub>

					{selectedAdvancedOptions.length > 0 && (
						<div className="flex flex-wrap gap-1 px-2 pt-1 pb-2">
							{selectedAdvancedOptions.map((option) => (
								<Badge key={option.key} size="xs" className="pr-0.5">
									{option.label}
									<button
										type="button"
										aria-label={`Remove ${option.label} filter`}
										onClick={() => option.setChecked(false)}
										className="flex size-4 cursor-pointer items-center justify-center rounded border-0 bg-transparent p-0 text-content-secondary hover:text-content-primary"
									>
										<XIcon className="size-3" />
									</button>
								</Badge>
							))}
						</div>
					)}
				</div>

				<div className="border-0 border-b border-solid border-border p-1">
					<DropdownMenuSub>
						<DropdownMenuSubTrigger>
							<FilterRow
								label="Grouped by"
								value={GROUP_BY_LABELS[filters.groupBy]}
							/>
						</DropdownMenuSubTrigger>
						<DropdownMenuSubContent className={MOBILE_MENU_CLASS}>
							<DropdownMenuRadioGroup
								value={filters.groupBy}
								onValueChange={setGroupBy}
							>
								{GROUP_BY_ORDER.map((groupBy) => (
									<DropdownMenuRadioItem
										key={groupBy}
										value={groupBy}
										onSelect={keepMenuOpen}
									>
										{GROUP_BY_LABELS[groupBy]}
									</DropdownMenuRadioItem>
								))}
							</DropdownMenuRadioGroup>
						</DropdownMenuSubContent>
					</DropdownMenuSub>
				</div>

				<div className="p-1">
					<DropdownMenuItem
						className="justify-center"
						onSelect={(event) => {
							keepMenuOpen(event);
							onFiltersChange(DEFAULT_AGENT_SIDEBAR_FILTERS);
						}}
					>
						<RotateCcwIcon />
						Reset to defaults
					</DropdownMenuItem>
				</div>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
