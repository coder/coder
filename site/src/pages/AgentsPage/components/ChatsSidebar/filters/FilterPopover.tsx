import { cn } from "cn";
import {
	ArrowLeftIcon,
	CheckIcon,
	ListFilterIcon,
	RotateCcwIcon,
	XIcon,
} from "lucide-react";
import { DropdownMenu as DropdownMenuPrimitive } from "radix-ui";
import { Fragment, useRef, useState } from "react";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { menuItemClass } from "#/components/DropdownMenu/menuClasses";
import {
	AGENT_CHAT_STATUS_ORDER,
	AGENT_PR_STATUS_ORDER,
	type AgentChatStatusFilter,
	type AgentPRStatusFilter,
	type AgentSidebarFilters,
	type AgentSourceFilter,
	DEFAULT_AGENT_SIDEBAR_FILTERS,
} from "../../../utils/agentSidebarFilters";
import { getChatStatusDisplay } from "../tree/statusConfig";

const PR_STATUS_LABELS: Record<AgentPRStatusFilter, string> = {
	draft: "PR: draft",
	open: "PR: open",
	merged: "PR: merged",
	closed: "PR: closed",
	none: "No PR",
};

const OWNER_OPTIONS = [
	{ value: "mine", label: "Mine", sources: ["created_by_me"] },
	{
		value: "shared_with_me",
		label: "Shared with me",
		sources: ["shared_with_me"],
	},
	{ value: "all", label: "All", sources: ["created_by_me", "shared_with_me"] },
] as const satisfies readonly {
	value: string;
	label: string;
	sources: readonly AgentSourceFilter[];
}[];

// Submenus overlay the full-width menu on phones, where there is no room beside it.
const MOBILE_MENU_CLASS =
	"mobile-full-width-dropdown mobile-full-width-dropdown-top-below-header";

const keepMenuOpen = (event: Event) => event.preventDefault();

const FilterRow: React.FC<{ label: string; value: React.ReactNode }> = ({
	label,
	value,
}) => (
	<>
		<span className="min-w-0 flex-1">{label}</span>
		<span className="truncate text-content-primary">{value}</span>
	</>
);

const FilterSubmenu: React.FC<{
	label: string;
	value: React.ReactNode;
	children: React.ReactNode;
	triggerRef?: React.Ref<HTMLDivElement>;
}> = ({ label, value, children, triggerRef }) => {
	const [open, setOpen] = useState(false);
	return (
		<DropdownMenuSub open={open} onOpenChange={setOpen}>
			<DropdownMenuSubTrigger ref={triggerRef}>
				<FilterRow label={label} value={value} />
			</DropdownMenuSubTrigger>
			<DropdownMenuSubContent
				className={cn(
					MOBILE_MENU_CLASS,
					"min-w-48 max-h-[calc(100dvh-6rem)] overflow-y-auto",
				)}
			>
				<DropdownMenuItem
					className="md:hidden"
					onSelect={(event) => {
						keepMenuOpen(event);
						setOpen(false);
					}}
				>
					<ArrowLeftIcon /> Back to filters
				</DropdownMenuItem>
				{children}
			</DropdownMenuSubContent>
		</DropdownMenuSub>
	);
};

// A visual checkbox avoids nesting a second interactive control inside the menu item.
const MultiSelectMenuItem: React.FC<
	React.ComponentProps<typeof DropdownMenuPrimitive.CheckboxItem>
> = ({ children, className, ...props }) => (
	<DropdownMenuPrimitive.CheckboxItem
		className={cn(menuItemClass, "group gap-3", className)}
		{...props}
	>
		<span
			aria-hidden
			className="flex size-[18px] shrink-0 items-center justify-center rounded-xs border border-solid border-border bg-surface-primary group-data-[state=checked]:border-surface-invert-primary group-data-[state=checked]:bg-surface-invert-primary group-data-[state=checked]:text-content-invert"
		>
			<DropdownMenuPrimitive.ItemIndicator>
				<CheckIcon className="size-4" strokeWidth={2.5} />
			</DropdownMenuPrimitive.ItemIndicator>
		</span>
		{children}
	</DropdownMenuPrimitive.CheckboxItem>
);

type FilterOption = {
	key: string;
	label: string;
	checked: boolean;
	setChecked: (checked: boolean) => void;
};

type FilterPopoverProps = {
	readonly filters: AgentSidebarFilters;
	readonly onFiltersChange: (filters: AgentSidebarFilters) => void;
};

export const FilterPopover: React.FC<FilterPopoverProps> = ({
	filters,
	onFiltersChange,
}) => {
	const filterByTriggerRef = useRef<HTMLDivElement>(null);
	const owner =
		filters.sources.length === 2
			? "all"
			: filters.sources.includes("shared_with_me")
				? "shared_with_me"
				: "mine";
	const ownerLabel =
		OWNER_OPTIONS.find((option) => option.value === owner)?.label ?? "All";
	const hasStatusFilter =
		filters.chatStatuses.length !== AGENT_CHAT_STATUS_ORDER.length;
	const setPRStatus = (status: AgentPRStatusFilter, checked: boolean) => {
		onFiltersChange({
			...filters,
			prStatuses: AGENT_PR_STATUS_ORDER.filter((value) =>
				value === status ? checked : filters.prStatuses.includes(value),
			),
		});
	};
	const setChatStatus = (status: AgentChatStatusFilter, checked: boolean) => {
		const selected = new Set(hasStatusFilter ? filters.chatStatuses : []);
		if (checked) selected.add(status);
		else selected.delete(status);
		onFiltersChange({
			...filters,
			chatStatuses:
				selected.size === 0
					? AGENT_CHAT_STATUS_ORDER
					: AGENT_CHAT_STATUS_ORDER.filter((value) => selected.has(value)),
		});
	};
	const options: readonly FilterOption[] = [
		{
			key: "unread",
			label: "Unread",
			checked: filters.unread,
			setChecked: (unread) => onFiltersChange({ ...filters, unread }),
		},
		...AGENT_PR_STATUS_ORDER.map((status) => ({
			key: `pr-${status}`,
			label: PR_STATUS_LABELS[status],
			checked: filters.prStatuses.includes(status),
			setChecked: (checked: boolean) => setPRStatus(status, checked),
		})),
		...AGENT_CHAT_STATUS_ORDER.map((status) => ({
			key: `status-${status}`,
			label: `Status: ${getChatStatusDisplay(status).label}`,
			checked: hasStatusFilter && filters.chatStatuses.includes(status),
			setChecked: (checked: boolean) => setChatStatus(status, checked),
		})),
	];
	const selectedOptions = options.filter((option) => option.checked);
	const filtersActive =
		filters.archiveStatus !== "active" ||
		owner !== "all" ||
		selectedOptions.length > 0;

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					aria-label="Filter agents"
					className={cn("relative", filtersActive && "text-content-primary")}
				>
					<ListFilterIcon />
					{filtersActive && (
						<span className="absolute right-1 top-1 size-1.5 rounded-full bg-content-link" />
					)}
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				align="end"
				aria-label="Filter agents"
				className={cn(
					MOBILE_MENU_CLASS,
					"w-64 p-0 max-h-[calc(100dvh-6rem)] overflow-y-auto",
				)}
			>
				<div className="border-0 border-b border-solid border-border p-1">
					<DropdownMenuRadioGroup
						aria-label="State"
						value={filters.archiveStatus}
						onValueChange={(value) => {
							if (value === "active" || value === "archived")
								onFiltersChange({ ...filters, archiveStatus: value });
						}}
						className="mb-1 flex rounded-lg bg-surface-secondary p-1"
					>
						{(["active", "archived"] as const).map((status) => (
							<DropdownMenuPrimitive.RadioItem
								key={status}
								value={status}
								onSelect={keepMenuOpen}
								className="flex flex-1 cursor-default select-none items-center justify-center rounded-md py-1 text-sm text-content-secondary outline-hidden transition-colors focus:text-content-primary focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-content-link data-[state=checked]:bg-surface-tertiary data-[state=checked]:text-content-primary data-[state=checked]:shadow-sm"
							>
								{status === "active" ? "Active" : "Archived"}
							</DropdownMenuPrimitive.RadioItem>
						))}
					</DropdownMenuRadioGroup>
					<FilterSubmenu label="Owner" value={ownerLabel}>
						<DropdownMenuRadioGroup
							value={owner}
							onValueChange={(value) => {
								const option = OWNER_OPTIONS.find(
									(option) => option.value === value,
								);
								if (option)
									onFiltersChange({ ...filters, sources: option.sources });
							}}
						>
							{OWNER_OPTIONS.map((option) => (
								<DropdownMenuRadioItem
									key={option.value}
									value={option.value}
									onSelect={keepMenuOpen}
								>
									{option.label}
								</DropdownMenuRadioItem>
							))}
						</DropdownMenuRadioGroup>
					</FilterSubmenu>
					<DropdownMenuItem disabled>
						<FilterRow label="Time range" value="Coming soon" />
					</DropdownMenuItem>
					<FilterSubmenu
						label="Filter by"
						triggerRef={filterByTriggerRef}
						value={selectedOptions.length || "None"}
					>
						{options.map((option, index) => (
							<Fragment key={option.key}>
								{index === 1 + AGENT_PR_STATUS_ORDER.length && (
									<DropdownMenuSeparator />
								)}
								<MultiSelectMenuItem
									checked={option.checked}
									onCheckedChange={(checked) =>
										option.setChecked(checked === true)
									}
									onSelect={keepMenuOpen}
								>
									{option.label}
								</MultiSelectMenuItem>
							</Fragment>
						))}
					</FilterSubmenu>
					{selectedOptions.length > 0 && (
						<div className="flex flex-wrap gap-1 px-2 pt-1 pb-2">
							{selectedOptions.map((option) => (
								<DropdownMenuItem
									key={option.key}
									asChild
									className="rounded border-0 bg-transparent p-0 focus-visible:ring-2 focus-visible:ring-content-link"
									onSelect={(event) => {
										keepMenuOpen(event);
										filterByTriggerRef.current?.focus();
										option.setChecked(false);
									}}
								>
									<button
										type="button"
										aria-label={`Remove ${option.label} filter`}
									>
										<Badge size="xs" className="pr-0.5">
											{option.label}
											<XIcon className="size-3" />
										</Badge>
									</button>
								</DropdownMenuItem>
							))}
						</div>
					)}
				</div>
				<div className="border-0 border-b border-solid border-border p-1">
					<FilterSubmenu
						label="Grouped by"
						value={filters.groupBy === "date" ? "Date" : "Status"}
					>
						<DropdownMenuRadioGroup
							value={filters.groupBy}
							onValueChange={(value) => {
								if (value === "date" || value === "chat_status")
									onFiltersChange({ ...filters, groupBy: value });
							}}
						>
							<DropdownMenuRadioItem value="date" onSelect={keepMenuOpen}>
								Date
							</DropdownMenuRadioItem>
							<DropdownMenuRadioItem
								value="chat_status"
								onSelect={keepMenuOpen}
							>
								Status
							</DropdownMenuRadioItem>
						</DropdownMenuRadioGroup>
					</FilterSubmenu>
				</div>
				<div className="p-1">
					<DropdownMenuItem
						className="justify-center"
						onSelect={(event) => {
							keepMenuOpen(event);
							onFiltersChange(DEFAULT_AGENT_SIDEBAR_FILTERS);
						}}
					>
						<RotateCcwIcon /> Reset to defaults
					</DropdownMenuItem>
				</div>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
