import {
	BotIcon,
	ChevronDownIcon,
	ChevronLeftIcon,
	PencilIcon,
	SearchIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import type { EffortCount } from "./boardApi";
import { InlineEdit } from "./InlineEdit";

type BoardHeaderProps = {
	/** Undefined while the list loads; a zero would read as an empty board. */
	readonly chatCount: number | undefined;
	readonly cardCount: number | undefined;
	/** Cards left after the filter; undefined when no filter is active. */
	readonly visibleCount: number | undefined;
	readonly search: string;
	readonly onSearchChange: (value: string) => void;
	readonly onExit: () => void;
	/** Opens the board's own assistant; an empty board has nothing to organize. */
	readonly onAssistant: () => void;
	/** Every effort on the board; the effort menu is hidden when empty. */
	readonly efforts: readonly EffortCount[];
	/** The selected effort; null shows every card. */
	readonly effortFilter: string | null;
	readonly onEffortFilter: (name: string | null) => void;
	readonly onRenameEffort: (from: string, to: string) => void;
};

export const BoardHeader: FC<BoardHeaderProps> = ({
	chatCount,
	cardCount,
	visibleCount,
	search,
	onSearchChange,
	onExit,
	onAssistant,
	efforts,
	effortFilter,
	onEffortFilter,
	onRenameEffort,
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
		{efforts.length > 0 && (
			<EffortMenu
				efforts={efforts}
				cardCount={cardCount}
				value={effortFilter}
				onChange={onEffortFilter}
				onRename={onRenameEffort}
			/>
		)}
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
					{visibleCount} {visibleCount === 1 ? "card" : "cards"}
				</span>
			)}
		</div>
	</div>
);

type EffortMenuProps = {
	readonly efforts: readonly EffortCount[];
	readonly cardCount: number;
	readonly value: string | null;
	readonly onChange: (name: string | null) => void;
	readonly onRename: (from: string, to: string) => void;
};

// Radix radio items take strings; the empty name stands for "All", which
// no effort can be called because blank names are dropped on write.
const ALL = "";

/**
 * The effort filter as a menu in the header: the trigger reads "Efforts"
 * or the selected name, and the selected effort can be renamed in place.
 */
const EffortMenu: FC<EffortMenuProps> = ({
	efforts,
	cardCount,
	value,
	onChange,
	onRename,
}) => {
	const [renaming, setRenaming] = useState(false);
	if (renaming && value !== null) {
		return (
			<InlineEdit
				value={value}
				ariaLabel="Effort name"
				className="w-auto text-[11px] text-content-secondary"
				onSave={(to) => onRename(value, to)}
				onDone={() => setRenaming(false)}
			/>
		);
	}
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<button
					type="button"
					aria-label="Filter by effort"
					className="flex items-center gap-1 border-0 bg-transparent p-0 text-[11px] text-content-secondary hover:text-content-primary data-[state=open]:text-content-primary"
				>
					{value ?? "Efforts"}
					<ChevronDownIcon className="size-3.5" />
				</button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="end" className="min-w-44 text-xs">
				<DropdownMenuRadioGroup
					value={value ?? ALL}
					onValueChange={(next) => onChange(next === ALL ? null : next)}
				>
					{[{ name: ALL, count: cardCount }, ...efforts].map(
						({ name, count }) => (
							<DropdownMenuRadioItem
								key={name}
								value={name}
								className="text-xs"
							>
								{name === ALL ? "All" : name}
								<span className="ml-auto pl-3 text-content-secondary tabular-nums">
									{count}
								</span>
							</DropdownMenuRadioItem>
						),
					)}
				</DropdownMenuRadioGroup>
				{value !== null && (
					<>
						<DropdownMenuSeparator />
						<DropdownMenuItem onSelect={() => setRenaming(true)}>
							<PencilIcon className="size-3.5" />
							Rename effort
						</DropdownMenuItem>
					</>
				)}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
