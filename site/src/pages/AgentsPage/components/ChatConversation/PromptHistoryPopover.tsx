import { SearchIcon, SquareStackIcon } from "lucide-react";
import { type FC, useMemo, useRef, useState } from "react";
import { Button } from "#/components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { scrollToUserSentinel } from "./scrollToUserSentinel";

export interface PromptHistoryEntry {
	/** The message ID used to locate the sentinel in the DOM. */
	id: number;
	/** 1-based index of this prompt in the conversation. */
	index: number;
	/** Plain-text preview of the user message. */
	text: string;
}

interface PromptHistoryPopoverProps {
	entries: readonly PromptHistoryEntry[];
	/** Called when the popover opens/closes so the parent action bar
	 *  can stay visible while the dropdown is active. */
	onOpenChange?: (open: boolean) => void;
}

/**
 * A popover that lists every user prompt in the conversation. Clicking an
 * entry scrolls the chat so that prompt becomes visible.
 */
export const PromptHistoryPopover: FC<PromptHistoryPopoverProps> = ({
	entries,
	onOpenChange,
}) => {
	const [open, setOpen] = useState(false);
	const [search, setSearch] = useState("");
	const [activeIndex, setActiveIndex] = useState(-1);
	const inputRef = useRef<HTMLInputElement>(null);
	const listRef = useRef<HTMLDivElement>(null);

	const filtered = useMemo(() => {
		if (!search) return entries;
		const lower = search.toLowerCase();
		return entries.filter((e) => e.text.toLowerCase().includes(lower));
	}, [entries, search]);

	const handleOpenChange = (next: boolean) => {
		setOpen(next);
		onOpenChange?.(next);
		if (next) {
			setSearch("");
			setActiveIndex(-1);
		}
	};

	const scrollToMessage = (messageId: number) => {
		handleOpenChange(false);
		setTimeout(() => scrollToUserSentinel(messageId), 80);
	};

	const handleKeyDown = (e: React.KeyboardEvent) => {
		if (filtered.length === 0) return;

		switch (e.key) {
			case "ArrowDown": {
				e.preventDefault();
				const next = activeIndex < filtered.length - 1 ? activeIndex + 1 : 0;
				setActiveIndex(next);
				scrollActiveIntoView(next);
				break;
			}
			case "ArrowUp": {
				e.preventDefault();
				const prev = activeIndex > 0 ? activeIndex - 1 : filtered.length - 1;
				setActiveIndex(prev);
				scrollActiveIntoView(prev);
				break;
			}
			case "Enter": {
				e.preventDefault();
				if (activeIndex >= 0 && activeIndex < filtered.length) {
					scrollToMessage(filtered[activeIndex].id);
				}
				break;
			}
			case "Home": {
				e.preventDefault();
				setActiveIndex(0);
				scrollActiveIntoView(0);
				break;
			}
			case "End": {
				e.preventDefault();
				const last = filtered.length - 1;
				setActiveIndex(last);
				scrollActiveIntoView(last);
				break;
			}
		}
	};

	const scrollActiveIntoView = (idx: number) => {
		const list = listRef.current;
		if (!list) return;
		const items = list.querySelectorAll('[role="option"]');
		items[idx]?.scrollIntoView({ block: "nearest" });
	};

	// Don't render anything if there are fewer than 2 user messages.
	if (entries.length < 2) {
		return null;
	}

	const activeDescendant =
		activeIndex >= 0 && activeIndex < filtered.length
			? `prompt-option-${filtered[activeIndex].id}`
			: undefined;

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<Tooltip>
				<TooltipTrigger asChild>
					<PopoverTrigger asChild>
						<Button
							size="icon"
							variant="subtle"
							className="size-6"
							aria-label="Prompt history"
						>
							<SquareStackIcon />
						</Button>
					</PopoverTrigger>
				</TooltipTrigger>
				<TooltipContent side="bottom">Prompt history</TooltipContent>
			</Tooltip>

			<PopoverContent
				align="end"
				side="bottom"
				className="w-80 p-0"
				onOpenAutoFocus={(e) => {
					e.preventDefault();
					inputRef.current?.focus();
				}}
			>
				<div
					role="combobox"
					aria-expanded={open}
					aria-haspopup="listbox"
					aria-owns="prompt-history-listbox"
					tabIndex={-1}
					onKeyDown={handleKeyDown}
				>
					<div className="flex items-center gap-2 border-0 border-b border-solid border-border px-3 py-2">
						<SearchIcon
							className="size-4 shrink-0 text-content-secondary"
							aria-hidden="true"
						/>
						<input
							ref={inputRef}
							type="text"
							role="searchbox"
							value={search}
							onChange={(e) => {
								setSearch(e.target.value);
								setActiveIndex(-1);
							}}
							placeholder="Search prompts..."
							aria-label="Search prompts"
							aria-autocomplete="list"
							aria-controls="prompt-history-listbox"
							aria-activedescendant={activeDescendant}
							className="w-full border-none bg-transparent text-sm text-content-primary shadow-none outline-none placeholder:text-content-secondary"
						/>
					</div>
					<div
						ref={listRef}
						id="prompt-history-listbox"
						role="listbox"
						aria-label="Prompt history"
						className="max-h-64 overflow-y-auto [scrollbar-width:thin]"
					>
						{filtered.length === 0 ? (
							<div
								className="px-3 py-4 text-center text-xs text-content-secondary"
								role="status"
								aria-live="polite"
							>
								No matching prompts
							</div>
						) : (
							filtered.map((entry, idx) => (
								<div
									key={entry.id}
									id={`prompt-option-${entry.id}`}
									role="option"
									aria-selected={idx === activeIndex}
									tabIndex={-1}
									onClick={() => scrollToMessage(entry.id)}
									onKeyDown={(e) => {
										if (e.key === "Enter" || e.key === " ") {
											e.preventDefault();
											scrollToMessage(entry.id);
										}
									}}
									className="flex w-full cursor-pointer items-baseline gap-3 border-0 bg-transparent px-3 py-2.5 text-left transition-colors hover:bg-surface-secondary focus:bg-surface-secondary focus:outline-none aria-selected:bg-surface-secondary"
								>
									<span className="min-w-[1.5rem] text-sm tabular-nums text-content-disabled">
										{entry.index}
									</span>
									<span className="min-w-0 truncate text-sm text-content-primary">
										{entry.text || "Empty message"}
									</span>
								</div>
							))
						)}
					</div>
				</div>
			</PopoverContent>
		</Popover>
	);
};
