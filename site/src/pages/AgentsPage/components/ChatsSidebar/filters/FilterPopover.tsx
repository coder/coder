import {
	CalendarIcon,
	CircleOffIcon,
	DotIcon,
	FilterIcon,
	GitMergeIcon,
	GitPullRequestClosedIcon,
	GitPullRequestDraftIcon,
	GitPullRequestIcon,
	type LucideIcon,
	MailIcon,
	MailOpenIcon,
	MessagesSquareIcon,
	UserIcon,
	UsersIcon,
} from "lucide-react";
import type { FC, ReactNode } from "react";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
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
	{ value: "chat_status", label: "Chat status", icon: MessagesSquareIcon },
];

const CHAT_STATUS_LABELS: Record<AgentChatStatusFilter, string> = {
	unread: "Unread",
	read: "Read",
};

const SOURCE_LABELS: Record<AgentSourceFilter, string> = {
	created_by_me: "Created by me",
	shared_with_me: "Shared with me",
};

const PR_STATUS_ICONS: Record<AgentPRStatusFilter, LucideIcon> = {
	draft: GitPullRequestDraftIcon,
	open: GitPullRequestIcon,
	merged: GitMergeIcon,
	closed: GitPullRequestClosedIcon,
	none: CircleOffIcon,
};

const CHAT_STATUS_ICONS: Record<AgentChatStatusFilter, LucideIcon> = {
	unread: MailIcon,
	read: MailOpenIcon,
};

const SOURCE_ICONS: Record<AgentSourceFilter, LucideIcon> = {
	created_by_me: UserIcon,
	shared_with_me: UsersIcon,
};

const CHAT_STATUS_OPTIONS: readonly Readonly<{
	value: AgentChatStatusFilter;
	label: string;
	icon: LucideIcon;
}>[] = AGENT_CHAT_STATUS_ORDER.map((status) => ({
	value: status,
	label: CHAT_STATUS_LABELS[status],
	icon: CHAT_STATUS_ICONS[status],
}));

const SOURCE_OPTIONS: readonly Readonly<{
	value: AgentSourceFilter;
	label: string;
	icon: LucideIcon;
}>[] = AGENT_SOURCE_ORDER.map((source) => ({
	value: source,
	label: SOURCE_LABELS[source],
	icon: SOURCE_ICONS[source],
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

const hasActiveFilters = (filters: AgentSidebarFilters): boolean => {
	return (
		filters.archiveStatus !== DEFAULT_AGENT_SIDEBAR_FILTERS.archiveStatus ||
		filters.prStatuses.length > 0 ||
		!haveSameSelections(
			filters.chatStatuses,
			DEFAULT_AGENT_SIDEBAR_FILTERS.chatStatuses,
		) ||
		!haveSameSelections(filters.sources, DEFAULT_AGENT_SIDEBAR_FILTERS.sources)
	);
};

// Selecting a value would otherwise dismiss the menu before the next toggle.
const keepSubmenuOpen = (event: Event) => {
	event.preventDefault();
};

const FilterSubmenu: FC<{
	readonly label: string;
	readonly summary?: boolean;
	readonly children: ReactNode;
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

export const FilterPopover: FC<FilterPopoverProps> = ({
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
		if (selected.size === 0) {
			return;
		}
		onFiltersChange({
			...filters,
			chatStatuses: AGENT_CHAT_STATUS_ORDER.filter((value) =>
				selected.has(value),
			),
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

		if (nextSources.length === 0) {
			return;
		}

		onFiltersChange({ ...filters, sources: nextSources });
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
	const filtersActive = hasActiveFilters(filters);

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
				<DropdownMenuLabel>Grouping</DropdownMenuLabel>
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
								onSelect={keepSubmenuOpen}
							>
								<Icon />
								{option.label}
							</DropdownMenuRadioItem>
						);
					})}
				</DropdownMenuRadioGroup>

				<DropdownMenuSeparator />

				<DropdownMenuLabel className="flex items-center justify-between">
					<span>Filters</span>
					<Button
						variant="subtle"
						size="sm"
						disabled={!filtersActive}
						onClick={() =>
							onFiltersChange({
								...DEFAULT_AGENT_SIDEBAR_FILTERS,
								groupBy: filters.groupBy,
							})
						}
						className="min-w-auto py-0 pl-0 pr-1"
					>
						Reset
					</Button>
				</DropdownMenuLabel>
				<FilterSubmenu label="PR" summary={prSummary}>
					{AGENT_PR_STATUS_ORDER.map((status) => {
						const Icon = PR_STATUS_ICONS[status];
						return (
							<DropdownMenuCheckboxItem
								key={status}
								checked={filters.prStatuses.includes(status)}
								onCheckedChange={(checked) =>
									setPRStatus(status, checked === true)
								}
								onSelect={keepSubmenuOpen}
							>
								<Icon />
								{PR_STATUS_LABELS[status]}
							</DropdownMenuCheckboxItem>
						);
					})}
				</FilterSubmenu>

				<FilterSubmenu label="Chat status" summary={chatSummary}>
					{CHAT_STATUS_OPTIONS.map((option) => {
						const Icon = option.icon;
						return (
							<DropdownMenuCheckboxItem
								key={option.value}
								checked={filters.chatStatuses.includes(option.value)}
								onCheckedChange={(checked) =>
									setChatStatus(option.value, checked === true)
								}
								onSelect={keepSubmenuOpen}
							>
								<Icon />
								{option.label}
							</DropdownMenuCheckboxItem>
						);
					})}
				</FilterSubmenu>

				<FilterSubmenu label="Source" summary={sourceSummary}>
					{SOURCE_OPTIONS.map((option) => {
						const Icon = option.icon;
						return (
							<DropdownMenuCheckboxItem
								key={option.value}
								checked={filters.sources.includes(option.value)}
								onCheckedChange={(checked) =>
									setSource(option.value, checked === true)
								}
								onSelect={keepSubmenuOpen}
							>
								<Icon />
								{option.label}
							</DropdownMenuCheckboxItem>
						);
					})}
				</FilterSubmenu>

				<DropdownMenuCheckboxItem
					checked={filters.archiveStatus === "archived"}
					onCheckedChange={(checked) => setArchived(checked === true)}
					onSelect={keepSubmenuOpen}
					className="[&>span]:right-3.5"
				>
					Archived
				</DropdownMenuCheckboxItem>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
