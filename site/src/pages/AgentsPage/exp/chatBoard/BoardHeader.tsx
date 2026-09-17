import { BotIcon, ChevronLeftIcon, SearchIcon } from "lucide-react";
import type { FC } from "react";
import { Button } from "#/components/Button/Button";

interface BoardHeaderProps {
	readonly chatCount: number;
	readonly cardCount: number;
	/** Cards left after the filter; undefined when no filter is active. */
	readonly visibleCount: number | undefined;
	readonly search: string;
	readonly onSearchChange: (value: string) => void;
	readonly onExit: () => void;
	/** Opens the board's own assistant; an empty board has nothing to organize. */
	readonly onAssistant: () => void;
}

export const BoardHeader: FC<BoardHeaderProps> = ({
	chatCount,
	cardCount,
	visibleCount,
	search,
	onSearchChange,
	onExit,
	onAssistant,
}) => (
	<div className="flex h-12 shrink-0 items-center gap-3 border-b border-border pr-4 pl-3">
		<Button
			variant="subtle"
			size="icon"
			aria-label="Exit board"
			className="size-7 text-content-secondary"
			onClick={onExit}
		>
			<ChevronLeftIcon className="size-4" />
		</Button>
		<h1 className="m-0 text-sm font-medium tracking-[-0.01em] text-content-primary">
			Board
		</h1>
		<span className="pl-1 text-[11px] text-content-secondary/70">
			{chatCount} chats · {cardCount} cards
		</span>
		<Button
			variant="subtle"
			size="icon"
			aria-label="Board assistant"
			title="Board assistant"
			className="ml-auto size-7 text-content-secondary"
			disabled={chatCount === 0}
			onClick={onAssistant}
		>
			<BotIcon className="size-4" />
		</Button>
		<div className="relative flex h-[30px] w-[260px] items-center gap-2 rounded-[7px] border border-border bg-surface-primary px-2.5 focus-within:border-content-link">
			<SearchIcon className="size-3.5 shrink-0 text-content-secondary" />
			<input
				aria-label="Filter cards"
				placeholder="Filter cards"
				value={search}
				onChange={(e) => onSearchChange(e.target.value)}
				className="min-w-0 flex-1 border-0 bg-transparent p-0 text-[13px] text-content-primary outline-none placeholder:text-content-secondary/60"
			/>
			{visibleCount !== undefined && (
				<span className="text-[11px] text-content-secondary">
					{visibleCount}
				</span>
			)}
		</div>
	</div>
);
