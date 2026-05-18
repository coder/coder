import { ListFilterIcon, SearchIcon } from "lucide-react";
import { type FC, useCallback, useMemo, useState } from "react";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { RadioGroup, RadioGroupItem } from "#/components/RadioGroup/RadioGroup";
import { ScrollArea } from "#/components/ScrollArea/ScrollArea";
import { Separator } from "#/components/Separator/Separator";
import { cn } from "#/utils/cn";

export type GroupBy = "date" | "chat_status";

export type PRStatusFilter = "draft" | "open" | "merged" | "closed";

export type ChatStatusFilter =
	| "unread"
	| "running"
	| "idle_awaiting_feedback"
	| "archived";

export interface ChatFilterState {
	readonly groupBy: GroupBy;
	readonly prStatus: ReadonlySet<PRStatusFilter>;
	readonly chatStatus: ReadonlySet<ChatStatusFilter>;
}

const DEFAULT_FILTER_STATE: ChatFilterState = {
	groupBy: "date",
	prStatus: new Set<PRStatusFilter>(),
	chatStatus: new Set<ChatStatusFilter>(),
};

const PR_STATUS_OPTIONS: readonly {
	readonly value: PRStatusFilter;
	readonly label: string;
}[] = [
	{ value: "draft", label: "Draft" },
	{ value: "open", label: "Open" },
	{ value: "merged", label: "Merged" },
	{ value: "closed", label: "Closed" },
];

const CHAT_STATUS_OPTIONS: readonly {
	readonly value: ChatStatusFilter;
	readonly label: string;
}[] = [
	{ value: "unread", label: "Unread" },
	{ value: "running", label: "Running" },
	{ value: "idle_awaiting_feedback", label: "Idle/awaiting feedback" },
	{ value: "archived", label: "Archived" },
];

interface FilterDropdownProps {
	readonly filterState: ChatFilterState;
	readonly onFilterChange?: (state: ChatFilterState) => void;
}

/** True when any filter or grouping deviates from defaults. */
export function hasActiveFilters(state: ChatFilterState): boolean {
	return (
		state.groupBy !== DEFAULT_FILTER_STATE.groupBy ||
		state.prStatus.size > 0 ||
		state.chatStatus.size > 0
	);
}

export { DEFAULT_FILTER_STATE };

export const FilterDropdown: FC<FilterDropdownProps> = ({
	filterState,
	onFilterChange,
}) => {
	const [open, setOpen] = useState(false);

	// Pending state: local copy that is committed only on Apply.
	const [pending, setPending] = useState<ChatFilterState>(filterState);
	const [search, setSearch] = useState("");

	// Reset pending state when the popover opens.
	const handleOpenChange = useCallback(
		(next: boolean) => {
			if (next) {
				setPending(filterState);
				setSearch("");
			}
			setOpen(next);
		},
		[filterState],
	);

	const normalizedSearch = search.toLowerCase();

	const filteredPROptions = useMemo(
		() =>
			normalizedSearch
				? PR_STATUS_OPTIONS.filter((o) =>
						o.label.toLowerCase().includes(normalizedSearch),
					)
				: PR_STATUS_OPTIONS,
		[normalizedSearch],
	);

	const filteredChatStatusOptions = useMemo(
		() =>
			normalizedSearch
				? CHAT_STATUS_OPTIONS.filter((o) =>
						o.label.toLowerCase().includes(normalizedSearch),
					)
				: CHAT_STATUS_OPTIONS,
		[normalizedSearch],
	);

	const togglePRStatus = useCallback((value: PRStatusFilter) => {
		setPending((prev) => {
			const next = new Set(prev.prStatus);
			if (next.has(value)) {
				next.delete(value);
			} else {
				next.add(value);
			}
			return { ...prev, prStatus: next };
		});
	}, []);

	const toggleChatStatus = useCallback((value: ChatStatusFilter) => {
		setPending((prev) => {
			const next = new Set(prev.chatStatus);
			if (next.has(value)) {
				next.delete(value);
			} else {
				next.add(value);
			}
			return { ...prev, chatStatus: next };
		});
	}, []);

	const handleClearAll = useCallback(() => {
		setPending(DEFAULT_FILTER_STATE);
	}, []);

	const handleApply = useCallback(() => {
		onFilterChange?.(pending);
		setOpen(false);
	}, [onFilterChange, pending]);

	const isActive = hasActiveFilters(filterState);

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					aria-label="Filter agents"
					className={cn(
						"h-7 w-7 min-w-0 justify-end rounded-none px-0 text-content-secondary hover:text-content-primary",
						isActive && "text-content-primary",
					)}
				>
					<ListFilterIcon className="size-4" />
				</Button>
			</PopoverTrigger>

			<PopoverContent
				align="end"
				className="w-64 p-0"
				onKeyDown={(e) => {
					if (e.key === "Enter") {
						e.preventDefault();
						handleApply();
					}
				}}
			>
				{/* ── Group section ── */}
				<div className="px-3 pt-3 pb-2">
					<span className="text-xs font-medium text-content-primary">
						Group
					</span>
					<RadioGroup
						value={pending.groupBy}
						onValueChange={(v) =>
							setPending((prev) => ({ ...prev, groupBy: v as GroupBy }))
						}
						className="mt-2 gap-2"
					>
						<div className="flex items-center gap-2">
							<RadioGroupItem value="date" id="group-date" />
							<Label htmlFor="group-date" className="text-sm font-normal">
								Date
							</Label>
						</div>
						<div className="flex items-center gap-2">
							<RadioGroupItem value="chat_status" id="group-chat-status" />
							<Label
								htmlFor="group-chat-status"
								className="text-sm font-normal"
							>
								Chat status
							</Label>
						</div>
					</RadioGroup>
				</div>

				<Separator />

				{/* ── Filter by section ── */}
				<div className="px-3 pt-2 pb-1">
					<span className="text-xs font-medium text-content-primary">
						Filter by
					</span>
					<div className="relative mt-2">
						<SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-content-secondary" />
						<Input
							placeholder="Search filters..."
							value={search}
							onChange={(e) => setSearch(e.target.value)}
							className="h-8 pl-8 text-xs"
						/>
					</div>
				</div>

				{/* ── Scrollable filter list ── */}
				<ScrollArea className="max-h-64">
					<div className="px-3 pt-1 pb-2">
						{/* PR status */}
						{filteredPROptions.length > 0 && (
							<div className="mt-1">
								<span className="text-xs text-content-secondary">
									PR status
								</span>
								<div className="mt-1.5 flex flex-col gap-2">
									{filteredPROptions.map((option) => (
										<label
											key={option.value}
											htmlFor={`pr-${option.value}`}
											className="flex cursor-pointer items-center gap-2"
										>
											<Checkbox
												id={`pr-${option.value}`}
												checked={pending.prStatus.has(option.value)}
												onCheckedChange={() => togglePRStatus(option.value)}
											/>
											<span className="text-sm">{option.label}</span>
										</label>
									))}
								</div>
							</div>
						)}

						{/* Chat status */}
						{filteredChatStatusOptions.length > 0 && (
							<div className="mt-3">
								<span className="text-xs text-content-secondary">
									Chat status
								</span>
								<div className="mt-1.5 flex flex-col gap-2">
									{filteredChatStatusOptions.map((option) => (
										<label
											key={option.value}
											htmlFor={`chat-${option.value}`}
											className="flex cursor-pointer items-center gap-2"
										>
											<Checkbox
												id={`chat-${option.value}`}
												checked={pending.chatStatus.has(option.value)}
												onCheckedChange={() => toggleChatStatus(option.value)}
											/>
											<span className="text-sm">{option.label}</span>
										</label>
									))}
								</div>
							</div>
						)}
					</div>
				</ScrollArea>

				<Separator />

				{/* ── Footer ── */}
				<div className="flex items-center justify-between px-3 py-2">
					<button
						type="button"
						onClick={handleClearAll}
						className="cursor-pointer border-0 bg-transparent p-0 text-xs text-content-secondary hover:text-content-primary"
					>
						Clear all
					</button>
					<Button size="sm" onClick={handleApply} className="h-7 text-xs">
						Apply
					</Button>
				</div>
			</PopoverContent>
		</Popover>
	);
};
