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
	const inputRef = useRef<HTMLInputElement>(null);

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
		}
	};

	const scrollToMessage = (messageId: number) => {
		const sentinel = document.querySelector(
			`[data-user-sentinel][data-message-id="${messageId}"]`,
		);
		if (!sentinel) return;

		const scroller = sentinel.closest(
			'[data-testid="scroll-container"]',
		) as HTMLElement | null;
		if (!scroller) return;

		handleOpenChange(false);

		setTimeout(() => {
			scroller.style.overflowAnchor = "none";
			scroller.setAttribute("data-scroll-lock", "");

			const offset =
				sentinel.getBoundingClientRect().top -
				scroller.getBoundingClientRect().top -
				scroller.clientHeight / 2;

			// Animate with direct scrollTop assignments (synchronous,
			// can't be cancelled by layout changes).
			const start = scroller.scrollTop;
			const duration = 450;
			const t0 = performance.now();

			// Ease-in-out cubic for a gentle start and gentle stop.
			const ease = (t: number) =>
				t < 0.5 ? 4 * t ** 3 : 1 - (-2 * t + 2) ** 3 / 2;

			const step = (now: number) => {
				const p = Math.min((now - t0) / duration, 1);
				scroller.scrollTop = start + offset * ease(p);
				if (p < 1) {
					requestAnimationFrame(step);
				} else {
					scroller.style.overflowAnchor = "";
					scroller.removeAttribute("data-scroll-lock");
				}
			};
			requestAnimationFrame(step);
		}, 80);
	};

	// Don't render anything if there are fewer than 2 user messages.
	if (entries.length < 2) {
		return null;
	}

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
							<span className="sr-only">Prompt history</span>
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
				<div className="flex items-center gap-2 border-0 border-b border-solid border-border px-3 py-2">
					<SearchIcon className="size-4 shrink-0 text-content-secondary" />
					<input
						ref={inputRef}
						type="text"
						value={search}
						onChange={(e) => setSearch(e.target.value)}
						placeholder="Search prompts..."
						className="w-full border-none bg-transparent text-sm text-content-primary shadow-none outline-none placeholder:text-content-secondary"
					/>
				</div>
				<div className="max-h-64 overflow-y-auto [scrollbar-width:thin]">
					{filtered.length === 0 ? (
						<div className="px-3 py-4 text-center text-xs text-content-secondary">
							No matching prompts
						</div>
					) : (
						filtered.map((entry) => (
							<button
								key={entry.id}
								type="button"
								onClick={() => scrollToMessage(entry.id)}
								className="flex w-full items-baseline gap-3 border-0 bg-transparent px-3 py-2.5 text-left transition-colors hover:bg-surface-secondary focus:bg-surface-secondary focus:outline-none"
							>
								<span className="min-w-[1.5rem] text-sm tabular-nums text-content-disabled">
									{entry.index}
								</span>
								<span className="min-w-0 truncate text-sm text-content-primary">
									{entry.text || "Empty message"}
								</span>
							</button>
						))
					)}
				</div>
			</PopoverContent>
		</Popover>
	);
};
