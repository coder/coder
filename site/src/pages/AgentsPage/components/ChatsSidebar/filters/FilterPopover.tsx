import { cn } from "cn";
import {
	CalendarIcon,
	DotIcon,
	FilterIcon,
	type LucideIcon,
	MessagesSquareIcon,
} from "lucide-react";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	AGENT_CHAT_STATUS_ORDER,
	AGENT_PR_STATUS_ORDER,
	AGENT_SOURCE_ORDER,
	type AgentChatStatusFilter,
	type AgentPRStatusFilter,
	type AgentSidebarFilters,
	type AgentSidebarGroupBy,
	type AgentSourceFilter,
	DEFAULT_AGENT_SIDEBAR_FILTERS,
} from "../../../utils/agentSidebarFilters";
import { getChatStatusDisplay } from "../tree/statusConfig";

const PR_STATUS_LABELS: Record<AgentPRStatusFilter, string> = {
	draft: "PR Draft",
	open: "PR Open",
	merged: "PR Merged",
	closed: "PR Closed",
	none: "No PR",
};

const GROUP_OPTIONS: readonly Readonly<{
	value: AgentSidebarGroupBy;
	label: string;
	icon: LucideIcon;
}>[] = [
	{ value: "date", label: "Date", icon: CalendarIcon },
	{ value: "chat_status", label: "Status", icon: MessagesSquareIcon },
];

const SOURCE_LABELS: Record<AgentSourceFilter, string> = {
	created_by_me: "Created by me",
	shared_with_me: "Shared with me",
};

const CHAT_STATUS_OPTIONS = AGENT_CHAT_STATUS_ORDER.map((status) => ({
	value: status,
	label: getChatStatusDisplay(status).label,
}));

const SOURCE_OPTIONS = AGENT_SOURCE_ORDER.map((source) => ({
	value: source,
	label: SOURCE_LABELS[source],
}));

type FilterPopoverProps = {
	readonly filters: AgentSidebarFilters;
	readonly onFiltersChange: (filters: AgentSidebarFilters) => void;
};

const haveSameSelections = <T extends string>(
	left: readonly T[],
	right: readonly T[],
): boolean => {
	return (
		left.length === right.length && left.every((value) => right.includes(value))
	);
};

// Selecting a value would otherwise dismiss the menu before the next toggle.
const keepMenuOpen = (event: Event) => {
	event.preventDefault();
};

const FilterSubmenuCheckboxItem: React.FC<
	React.ComponentProps<typeof DropdownMenuCheckboxItem>
> = ({ className, ...props }) => (
	<DropdownMenuCheckboxItem
		className={cn(
			"pl-9 pr-2 [&>span]:left-2 [&>span]:right-auto [&>span]:size-[18px] [&>span]:rounded-xs [&>span]:border [&>span]:border-solid [&>span]:border-border-control [&>span]:bg-surface-primary",
			"data-[state=checked]:[&>span]:border-surface-invert-primary data-[state=checked]:[&>span]:bg-surface-invert-primary data-[state=checked]:[&>span]:text-content-invert",
			"[&>span_svg]:stroke-[2.5]",
			className,
		)}
		{...props}
	/>
);

const FilterSubmenu: React.FC<{
	readonly label: string;
	readonly summary?: boolean;
	readonly children: React.ReactNode;
}> = ({ label, summary, children }) => (
	<DropdownMenuSub>
		<DropdownMenuSubTrigger>
			<span className="min-w-0 flex-1 truncate">{label}</span>
			{summary ? (
				<span className="max-w-36 truncate font-normal -mr-2">
					<DotIcon />
				</span>
			) : null}
		</DropdownMenuSubTrigger>
		<DropdownMenuSubContent className="min-w-48">
			{children}
		</DropdownMenuSubContent>
	</DropdownMenuSub>
);

export const FilterPopover: React.FC<FilterPopoverProps> = ({
	filters,
	onFiltersChange,
}) => {
	const setGroupBy = (value: string) => {
		if (value !== "date" && value !== "chat_status") {
			return;
		}
		onFiltersChange({ ...filters, groupBy: value });
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

	const setChatStatus = (status: AgentChatStatusFilter, checked: boolean) => {
		const selected = new Set(filters.chatStatuses);
		if (checked) {
			selected.add(status);
		} else {
			selected.delete(status);
		}
		onFiltersChange({
			...filters,
			chatStatuses:
				selected.size === 0
					? DEFAULT_AGENT_SIDEBAR_FILTERS.chatStatuses
					: AGENT_CHAT_STATUS_ORDER.filter((value) => selected.has(value)),
		});
	};

	const setArchived = (checked: boolean) => {
		onFiltersChange({
			...filters,
			archiveStatus: checked ? "archived" : "active",
		});
	};

	const setSource = (source: AgentSourceFilter, checked: boolean) => {
		const nextSources = checked
			? AGENT_SOURCE_ORDER.filter(
					(value) => value === source || filters.sources.includes(value),
				)
			: filters.sources.filter((value) => value !== source);

		onFiltersChange({
			...filters,
			sources:
				nextSources.length === 0
					? DEFAULT_AGENT_SIDEBAR_FILTERS.sources
					: nextSources,
		});
	};

	const prSummary = !haveSameSelections(
		filters.prStatuses,
		DEFAULT_AGENT_SIDEBAR_FILTERS.prStatuses,
	);
	const chatSummary = !haveSameSelections(
		filters.chatStatuses,
		DEFAULT_AGENT_SIDEBAR_FILTERS.chatStatuses,
	);
	const sourceSummary = !haveSameSelections(
		filters.sources,
		DEFAULT_AGENT_SIDEBAR_FILTERS.sources,
	);
	const modifiedFilterCount = [
		prSummary,
		chatSummary,
		sourceSummary,
		filters.unread,
		filters.archiveStatus !== DEFAULT_AGENT_SIDEBAR_FILTERS.archiveStatus,
	].filter(Boolean).length;

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant="subtle" size="icon" aria-label="Filter agents">
					<FilterIcon />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				align="start"
				side="bottom"
				aria-label="Filter agents"
				className="min-w-56"
			>
				<DropdownMenuSub>
					<DropdownMenuSubTrigger>
						<div className="flex items-center justify-between gap-2 flex-1">
							<span>Group by</span>
							<span className="text-content-primary">
								{GROUP_OPTIONS.find((o) => o.value === filters.groupBy)?.label}
							</span>
						</div>
					</DropdownMenuSubTrigger>
					<DropdownMenuSubContent>
						<DropdownMenuRadioGroup
							value={filters.groupBy}
							onValueChange={setGroupBy}
						>
							{GROUP_OPTIONS.map((option) => {
								const Icon = option.icon;
								return (
									<DropdownMenuRadioItem
										key={option.value}
										value={option.value}
										className="gap-2 [&>svg]:size-icon-sm"
										onSelect={keepMenuOpen}
									>
										<Icon />
										{option.label}
									</DropdownMenuRadioItem>
								);
							})}
						</DropdownMenuRadioGroup>
					</DropdownMenuSubContent>
				</DropdownMenuSub>

				<DropdownMenuSeparator />

				<DropdownMenuLabel className="flex items-center justify-between gap-2 pr-1">
					<span>
						{modifiedFilterCount === 1 ? "Filter" : "Filters"}
						{modifiedFilterCount > 0 && ` (${modifiedFilterCount})`}
					</span>
					<DropdownMenuItem
						disabled={modifiedFilterCount === 0}
						onSelect={(event) => {
							keepMenuOpen(event);
							onFiltersChange({
								...DEFAULT_AGENT_SIDEBAR_FILTERS,
								groupBy: filters.groupBy,
							});
						}}
						className="px-1 py-0 text-xs font-medium text-content-secondary"
					>
						Reset
					</DropdownMenuItem>
				</DropdownMenuLabel>
				<FilterSubmenu label="PR" summary={prSummary}>
					{AGENT_PR_STATUS_ORDER.map((status) => (
						<FilterSubmenuCheckboxItem
							key={status}
							checked={filters.prStatuses.includes(status)}
							onCheckedChange={(checked) =>
								setPRStatus(status, checked === true)
							}
							onSelect={keepMenuOpen}
						>
							{PR_STATUS_LABELS[status]}
						</FilterSubmenuCheckboxItem>
					))}
				</FilterSubmenu>

				<FilterSubmenu label="Status" summary={chatSummary}>
					{CHAT_STATUS_OPTIONS.map((option) => (
						<FilterSubmenuCheckboxItem
							key={option.value}
							checked={filters.chatStatuses.includes(option.value)}
							onCheckedChange={(checked) =>
								setChatStatus(option.value, checked === true)
							}
							onSelect={keepMenuOpen}
						>
							{option.label}
						</FilterSubmenuCheckboxItem>
					))}
				</FilterSubmenu>

				<FilterSubmenu label="Source" summary={sourceSummary}>
					{SOURCE_OPTIONS.map((option) => (
						<FilterSubmenuCheckboxItem
							key={option.value}
							checked={filters.sources.includes(option.value)}
							onCheckedChange={(checked) =>
								setSource(option.value, checked === true)
							}
							onSelect={keepMenuOpen}
						>
							{option.label}
						</FilterSubmenuCheckboxItem>
					))}
				</FilterSubmenu>

				<DropdownMenuCheckboxItem
					checked={filters.unread}
					onCheckedChange={(checked) =>
						onFiltersChange({ ...filters, unread: checked === true })
					}
					onSelect={keepMenuOpen}
					className="[&>span]:right-3.5"
				>
					Unread
				</DropdownMenuCheckboxItem>
				<DropdownMenuCheckboxItem
					checked={filters.archiveStatus === "archived"}
					onCheckedChange={(checked) => setArchived(checked === true)}
					onSelect={keepMenuOpen}
					className="[&>span]:right-3.5"
				>
					Archived
				</DropdownMenuCheckboxItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
