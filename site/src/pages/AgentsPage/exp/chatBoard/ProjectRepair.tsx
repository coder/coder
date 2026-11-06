import { TriangleAlertIcon } from "lucide-react";
import { useState } from "react";
import { Button } from "#/components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import type { BoardCard } from "./boardLabels";

/** A project a card can be moved into; null id for no project. */
type ProjectOption = Readonly<{ id: string | null; name: string }>;

type ProjectRepairProps = {
	/** Cards whose chats are in different projects. */
	readonly cards: readonly BoardCard[];
	/** The projects a card's chats are in, the only places it can go. */
	readonly optionsOf: (card: BoardCard) => readonly ProjectOption[];
	/** Why the card cannot move there now, or undefined when it can. */
	readonly blockedReason: (
		card: BoardCard,
		projectId: string | null,
	) => string | undefined;
	readonly onMove: (card: BoardCard, option: ProjectOption) => void;
};

/**
 * A warning in the board header while any card spans projects; it lists
 * those cards and moves each one whole into one of its chats' projects.
 */
export const ProjectRepair: React.FC<ProjectRepairProps> = ({
	cards,
	optionsOf,
	blockedReason,
	onMove,
}) => {
	const [open, setOpen] = useState(false);
	if (cards.length === 0) return null;
	const label =
		cards.length === 1
			? "1 card has chats in different projects"
			: `${cards.length} cards have chats in different projects`;
	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<Button
					variant="subtle"
					size="icon"
					aria-label={label}
					title={label}
					className="h-7 w-auto min-w-0 gap-1 px-1.5 text-xs text-content-warning tabular-nums [&>svg]:size-3.5! [&>svg]:p-0"
				>
					<TriangleAlertIcon />
					{cards.length}
				</Button>
			</PopoverTrigger>
			<PopoverContent align="end" className="w-80 p-0 text-sm">
				<p className="m-0 border-0 border-b border-solid border-border px-3 py-2.5 text-xs text-content-secondary">
					A card's chats should share a project. Move each card into one of
					them.
				</p>
				<ul className="m-0 max-h-96 list-none overflow-y-auto p-1">
					{cards.map((card) => (
						<li key={card.id} className="px-2 py-1.5">
							<div className="truncate pb-1 font-medium text-content-primary">
								{card.title}
							</div>
							{optionsOf(card).map((option) => {
								const reason = blockedReason(card, option.id);
								return (
									<button
										key={option.id ?? "none"}
										type="button"
										disabled={reason !== undefined}
										title={reason}
										className="flex w-full cursor-pointer items-center rounded border-0 bg-transparent px-2 py-1 text-left text-xs text-content-secondary hover:bg-surface-secondary hover:text-content-primary disabled:cursor-not-allowed disabled:opacity-50"
										onClick={() => {
											setOpen(false);
											onMove(card, option);
										}}
									>
										<span className="truncate">Move all to {option.name}</span>
									</button>
								);
							})}
						</li>
					))}
				</ul>
			</PopoverContent>
		</Popover>
	);
};
