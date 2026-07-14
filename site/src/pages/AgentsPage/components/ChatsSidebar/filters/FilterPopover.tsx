import { FilterIcon } from "lucide-react";
import {
	type ComponentProps,
	type FC,
	type ReactNode,
	useId,
	useState,
} from "react";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { SearchField } from "#/components/SearchField/SearchField";
import { cn } from "#/utils/cn";
import {
	AGENT_ARCHIVE_STATUS_ORDER,
	AGENT_CHAT_STATUS_ORDER,
	AGENT_PR_STATUS_ORDER,
	AGENT_SOURCE_ORDER,
	type AgentArchiveStatusFilter,
	type AgentChatStatusFilter,
	type AgentPRStatusFilter,
	type AgentSidebarFilters,
	type AgentSidebarGroupBy,
	type AgentSourceFilter,
	DEFAULT_AGENT_SIDEBAR_FILTERS,
} from "../../../utils/agentSidebarFilters";

const PR_STATUS_LABELS: Record<AgentPRStatusFilter, string> = {
	draft: "Draft",
	open: "Open",
	merged: "Merged",
	closed: "Closed",
};

const GROUP_OPTIONS: readonly Readonly<{
	value: AgentSidebarGroupBy;
	label: string;
}>[] = [
	{ value: "date", label: "Date" },
	{ value: "chat_status", label: "Chat status" },
];

const CHAT_STATUS_LABELS: Record<AgentChatStatusFilter, string> = {
	unread: "Unread",
	read: "Read",
};

const ARCHIVE_STATUS_LABELS: Record<AgentArchiveStatusFilter, string> = {
	active: "Active",
	archived: "Archived",
};

const SOURCE_LABELS: Record<AgentSourceFilter, string> = {
	created_by_me: "Created by me",
	shared_with_me: "Shared with me",
};

const CHAT_STATUS_OPTIONS: readonly Readonly<{
	value: AgentChatStatusFilter;
	label: string;
}>[] = AGENT_CHAT_STATUS_ORDER.map((status) => ({
	value: status,
	label: CHAT_STATUS_LABELS[status],
}));

const ARCHIVE_OPTIONS: readonly Readonly<{
	value: AgentArchiveStatusFilter;
	label: string;
}>[] = AGENT_ARCHIVE_STATUS_ORDER.map((status) => ({
	value: status,
	label: ARCHIVE_STATUS_LABELS[status],
}));

const SOURCE_OPTIONS: readonly Readonly<{
	value: AgentSourceFilter;
	label: string;
}>[] = AGENT_SOURCE_ORDER.map((source) => ({
	value: source,
	label: SOURCE_LABELS[source],
}));

const SectionHeading: FC<ComponentProps<"h2">> = ({ className, ...props }) => (
	<h2
		className={cn(
			"m-0 text-xs font-semibold leading-[18px] text-content-secondary",
			className,
		)}
		{...props}
	/>
);

const FilterGroupHeading: FC<ComponentProps<"h3">> = ({
	className,
	...props
}) => (
	<h3
		className={cn(
			"m-0 text-sm font-normal leading-[18px] text-content-disabled",
			className,
		)}
		{...props}
	/>
);

const OptionRow: FC<{ readonly children: ReactNode }> = ({ children }) => (
	<div className="flex h-6 items-center gap-2 rounded-sm">{children}</div>
);

interface FilterPopoverProps {
	readonly filters: AgentSidebarFilters;
	readonly onFiltersChange: (filters: AgentSidebarFilters) => void;
}

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
		filters.prStatuses.length > 0 ||
		!haveSameSelections(
			filters.chatStatuses,
			DEFAULT_AGENT_SIDEBAR_FILTERS.chatStatuses,
		) ||
		!haveSameSelections(filters.sources, DEFAULT_AGENT_SIDEBAR_FILTERS.sources)
	);
};

export const FilterPopover: FC<FilterPopoverProps> = ({
	filters,
	onFiltersChange,
}) => {
	const id = useId();
	const [open, setOpen] = useState(false);
	const [stagedFilters, setStagedFilters] =
		useState<AgentSidebarFilters>(filters);
	const [optionSearch, setOptionSearch] = useState("");

	const handleOpenChange = (nextOpen: boolean) => {
		if (nextOpen) {
			setStagedFilters(filters);
			setOptionSearch("");
		}
		setOpen(nextOpen);
	};

	const normalizedOptionSearch = optionSearch.trim().toLowerCase();
	const matchesOption = (...labels: readonly string[]) =>
		normalizedOptionSearch === "" ||
		labels.some((label) =>
			label.toLowerCase().includes(normalizedOptionSearch),
		);

	const visiblePRStatuses = AGENT_PR_STATUS_ORDER.filter((status) =>
		matchesOption("PR status", PR_STATUS_LABELS[status]),
	);
	const visibleChatStatusOptions = CHAT_STATUS_OPTIONS.filter((option) =>
		matchesOption("Chat status", option.label),
	);
	const visibleSourceOptions = SOURCE_OPTIONS.filter((option) =>
		matchesOption("Source", option.label),
	);
	const visibleArchiveOptions = ARCHIVE_OPTIONS.filter((option) =>
		matchesOption("Archive status", option.label),
	);
	const showFilterOptions =
		visiblePRStatuses.length > 0 ||
		visibleChatStatusOptions.length > 0 ||
		visibleSourceOptions.length > 0 ||
		visibleArchiveOptions.length > 0;

	const setGroupBy = (value: string) => {
		if (value !== "date" && value !== "chat_status") {
			return;
		}
		const groupBy: AgentSidebarGroupBy = value;
		setStagedFilters({ ...stagedFilters, groupBy });
	};

	const setPRStatus = (status: AgentPRStatusFilter, checked: boolean) => {
		const selected = new Set(stagedFilters.prStatuses);
		if (checked) {
			selected.add(status);
		} else {
			selected.delete(status);
		}
		setStagedFilters({
			...stagedFilters,
			prStatuses: AGENT_PR_STATUS_ORDER.filter((value) => selected.has(value)),
		});
	};

	const setChatStatus = (status: AgentChatStatusFilter, checked: boolean) => {
		const selected = new Set(stagedFilters.chatStatuses);
		if (checked) {
			selected.add(status);
		} else {
			selected.delete(status);
		}
		if (selected.size === 0) {
			return;
		}
		setStagedFilters({
			...stagedFilters,
			chatStatuses: AGENT_CHAT_STATUS_ORDER.filter((value) =>
				selected.has(value),
			),
		});
	};

	const setArchiveStatus = (value: string) => {
		if (value !== "active" && value !== "archived") {
			return;
		}
		setStagedFilters({ ...stagedFilters, archiveStatus: value });
	};

	const setSource = (source: AgentSourceFilter, checked: boolean) => {
		const nextSources = checked
			? AGENT_SOURCE_ORDER.filter(
					(value) => value === source || stagedFilters.sources.includes(value),
				)
			: stagedFilters.sources.filter((value) => value !== source);

		if (nextSources.length === 0) {
			return;
		}

		setStagedFilters({ ...stagedFilters, sources: nextSources });
	};

	const applyFilters = () => {
		onFiltersChange(stagedFilters);
		setOpen(false);
	};

	const clearFilters = () => {
		setStagedFilters(DEFAULT_AGENT_SIDEBAR_FILTERS);
		setOptionSearch("");
	};

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					aria-label="Filter agents"
					className={cn(
						"size-7 min-w-0 -mr-0.5 justify-end px-0 text-content-secondary hover:text-content-primary",
						hasActiveFilters(filters) && "text-content-primary",
					)}
				>
					<FilterIcon />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				align="end"
				aria-label="Filter agents"
				role="dialog"
				className="mobile-full-width-dropdown mobile-full-width-dropdown-top-below-header w-64 overflow-hidden p-0 text-sm"
			>
				<div className="border-0 border-b border-solid border-border px-3 py-2">
					<section className="space-y-2">
						<SectionHeading id={`${id}-group-heading`}>Group</SectionHeading>
						<RadioGroup
							aria-labelledby={`${id}-group-heading`}
							value={stagedFilters.groupBy}
							onValueChange={setGroupBy}
							className="gap-2"
						>
							{GROUP_OPTIONS.map((option) => {
								const optionId = `${id}-group-${option.value}`;
								return (
									<OptionRow key={option.value}>
										<RadioGroupItem
											id={optionId}
											value={option.value}
											className="m-0 my-1"
										/>
										<label
											className="flex flex-1 cursor-pointer items-center text-sm font-normal leading-5 text-content-primary"
											htmlFor={optionId}
										>
											{option.label}
										</label>
									</OptionRow>
								);
							})}
						</RadioGroup>
					</section>
				</div>

				<div className="px-3 pt-2">
					<section>
						<SectionHeading>Filter by</SectionHeading>
						<SearchField
							value={optionSearch}
							onChange={setOptionSearch}
							placeholder="Search filters..."
							aria-label="Search filters"
							className="mt-2 h-9 [&_input]:h-9 [&_input]:text-xs [&_input]:font-normal [&_svg]:size-4"
						/>
						<ScrollArea
							type="always"
							className="mt-5 h-[240px] [&_[data-radix-scroll-area-viewport]>div]:!block"
							scrollBarClassName="w-1.5"
							viewportClassName="pr-3"
						>
							<div className="space-y-4">
								{visiblePRStatuses.length > 0 && (
									<div className="space-y-1.5">
										<FilterGroupHeading>PR status</FilterGroupHeading>
										<div className="space-y-2">
											{visiblePRStatuses.map((status) => {
												const checked =
													stagedFilters.prStatuses.includes(status);
												const checkboxId = `${id}-pr-${status}`;
												return (
													<OptionRow key={status}>
														<Checkbox
															id={checkboxId}
															checked={checked}
															onCheckedChange={(nextChecked) =>
																setPRStatus(status, nextChecked === true)
															}
															className="m-0 my-[3px]"
														/>
														<label
															htmlFor={checkboxId}
															className="flex flex-1 cursor-pointer items-center text-sm font-normal leading-5 text-content-primary"
														>
															{PR_STATUS_LABELS[status]}
														</label>
													</OptionRow>
												);
											})}
										</div>
									</div>
								)}

								{visibleChatStatusOptions.length > 0 && (
									<div className="space-y-1.5">
										<FilterGroupHeading>Chat status</FilterGroupHeading>
										<div className="space-y-2">
											{visibleChatStatusOptions.map((option) => {
												const optionId = `${id}-chat-status-${option.value}`;
												return (
													<OptionRow key={option.value}>
														<Checkbox
															id={optionId}
															checked={stagedFilters.chatStatuses.includes(
																option.value,
															)}
															onCheckedChange={(nextChecked) =>
																setChatStatus(
																	option.value,
																	nextChecked === true,
																)
															}
															className="m-0 my-[3px]"
														/>
														<label
															htmlFor={optionId}
															className="flex flex-1 cursor-pointer items-center text-sm font-normal leading-5 text-content-primary"
														>
															{option.label}
														</label>
													</OptionRow>
												);
											})}
										</div>
									</div>
								)}

								{visibleSourceOptions.length > 0 && (
									<div className="space-y-1.5">
										<FilterGroupHeading>Source</FilterGroupHeading>
										<div className="space-y-2">
											{visibleSourceOptions.map((option) => {
												const optionId = `${id}-source-${option.value}`;
												return (
													<OptionRow key={option.value}>
														<Checkbox
															id={optionId}
															checked={stagedFilters.sources.includes(
																option.value,
															)}
															onCheckedChange={(nextChecked) =>
																setSource(option.value, nextChecked === true)
															}
															className="m-0 my-[3px]"
														/>
														<label
															htmlFor={optionId}
															className="flex flex-1 cursor-pointer items-center text-sm font-normal leading-5 text-content-primary"
														>
															{option.label}
														</label>
													</OptionRow>
												);
											})}
										</div>
									</div>
								)}

								{visibleArchiveOptions.length > 0 && (
									<div className="space-y-1.5">
										<FilterGroupHeading id={`${id}-archive-heading`}>
											Archive status
										</FilterGroupHeading>
										<RadioGroup
											aria-labelledby={`${id}-archive-heading`}
											value={stagedFilters.archiveStatus}
											onValueChange={setArchiveStatus}
											className="gap-2"
										>
											{visibleArchiveOptions.map((option) => {
												const optionId = `${id}-archive-${option.value}`;
												return (
													<OptionRow key={option.value}>
														<RadioGroupItem
															id={optionId}
															value={option.value}
															className="m-0 my-1"
														/>
														<label
															htmlFor={optionId}
															className="flex flex-1 cursor-pointer items-center text-sm font-normal leading-5 text-content-primary"
														>
															{option.label}
														</label>
													</OptionRow>
												);
											})}
										</RadioGroup>
									</div>
								)}

								{!showFilterOptions && (
									<p className="m-0 py-5 text-sm text-content-secondary">
										No filters found
									</p>
								)}
							</div>
						</ScrollArea>
					</section>
				</div>

				<div className="flex items-center justify-between gap-2 px-3 py-3">
					<Button
						variant="subtle"
						size="sm"
						onClick={clearFilters}
						className="h-8 min-w-0 px-0 text-xs font-normal"
					>
						Clear all
					</Button>
					<Button
						variant="default"
						size="sm"
						onClick={applyFilters}
						className="h-8 min-w-[64px] px-3 text-xs font-normal"
					>
						Apply
					</Button>
				</div>
			</PopoverContent>
		</Popover>
	);
};
