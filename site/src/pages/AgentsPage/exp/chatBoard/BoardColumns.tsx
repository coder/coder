import { PlusIcon } from "lucide-react";
import { useState } from "react";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { toast } from "#/components/Toaster/toast";
import type { ChatOpenHandlers } from "./BoardCard";
import { BoardColumn, NewColumn } from "./BoardColumn";
import {
	addColumn,
	addNote,
	type BoardState,
	deleteColumn,
	editNote,
	type Plan,
	removeFromGroup,
	removeNote,
	renameCard,
	renameChat,
	renameColumn,
	setCardColor,
	setCardEfforts,
} from "./boardApi";
import type { DropTarget } from "./boardDrag";
import type {
	BoardCard as BoardCardModel,
	BoardColumn as BoardColumnModel,
} from "./boardLabels";
import type { DraftTarget } from "./boardStorage";

type BoardColumnsProps = ChatOpenHandlers & {
	/** Columns after the filter, drawn left to right. */
	readonly columns: readonly BoardColumnModel[];
	/** The project lane these columns are drawn in, see BoardLane. */
	readonly lane: string;
	/**
	 * "board" fills the page and scrolls; "lane" is as tall as its tallest
	 * column and leaves scrolling to the lanes around it.
	 */
	readonly layout: "board" | "lane";
	/** The add-column button; columns are shared, so one lane shows it. */
	readonly showAddColumn: boolean;
	/** The unfiltered model that every command acts on. */
	readonly board: BoardState;
	readonly run: (plan: Plan | null) => Promise<boolean>;
	/** Assistant chat by card id, for the cards that have one. */
	readonly assistants: ReadonlyMap<string, Chat>;
	readonly openChatIds: ReadonlySet<string>;
	readonly dropTarget: DropTarget | null;
	/** Every effort on the board, for the card editors. */
	readonly knownEfforts: readonly string[];
	readonly onCardAssistant: (card: BoardCardModel) => void;
	/** Opens the create form for a chat born in a column or on a card. */
	readonly onNewChat: (target: DraftTarget) => void;
	/** A card's effort tag narrows the board to that effort. */
	readonly onFilterEffort: (name: string) => void;
	readonly onArchiveCard: (card: BoardCardModel) => void;
	readonly onArchiveCards: (cards: readonly BoardCardModel[]) => void;
	readonly onArchiveChat: (chat: Chat, card: BoardCardModel) => void;
};

// The composer clears and the editor closes before the write settles, and
// the refetch drops the optimistic note; this keeps the text on screen,
// selectable, until the user closes it. The mutation's own toast names the
// server error.
const keepNote = (text: string) =>
	toast("Note not saved", {
		description: text,
		duration: Number.POSITIVE_INFINITY,
		closeButton: true,
	});

export const BoardColumns: React.FC<BoardColumnsProps> = ({
	columns,
	lane,
	layout,
	showAddColumn,
	board,
	run,
	assistants,
	openChatIds,
	dropTarget,
	knownEfforts,
	onCardAssistant,
	onNewChat,
	onFilterEffort,
	onArchiveCard,
	onArchiveCards,
	onArchiveChat,
	onOpen,
	onPreview,
	onPreviewEnd,
}) => {
	const [addingColumn, setAddingColumn] = useState(false);

	return (
		<div
			className={
				layout === "board"
					? "flex min-h-0 flex-1 gap-4 overflow-x-auto bg-surface-secondary px-5 pt-4 pb-3"
					: "flex w-max gap-4 px-5 pb-3"
			}
		>
			{columns.map((column) => (
				<BoardColumn
					key={column.name}
					column={column}
					lane={lane}
					assistants={assistants}
					openChatIds={openChatIds}
					dropTarget={dropTarget}
					knownEfforts={knownEfforts}
					onRename={(to) => void run(renameColumn(board, column.name, to))}
					onDelete={() => void run(deleteColumn(board, column.name))}
					onNewChat={() =>
						onNewChat(
							layout === "lane"
								? { column: column.name, lane }
								: { column: column.name },
						)
					}
					onSetCardTitle={(card, title) =>
						void run(renameCard(board, card.id, title))
					}
					onSetCardColor={(card, color) =>
						void run(setCardColor(board, card.id, color))
					}
					onSetCardEfforts={(card, names) =>
						void run(setCardEfforts(board, card.id, names))
					}
					onFilterEffort={onFilterEffort}
					onRenameChat={(chat, title) =>
						void run(renameChat(board, chat.id, title))
					}
					onCardAssistant={onCardAssistant}
					onNewChatInCard={(card) => onNewChat({ cardId: card.id })}
					onArchiveCard={onArchiveCard}
					onArchiveCards={onArchiveCards}
					onArchiveChat={onArchiveChat}
					onRemoveFromGroup={(chat) =>
						void run(removeFromGroup(board, chat.id))
					}
					onOpen={onOpen}
					onPreview={onPreview}
					onPreviewEnd={onPreviewEnd}
					onAddNote={(card, text) =>
						void run(addNote(board, card.id, text)).then((saved) => {
							if (!saved) keepNote(text);
						})
					}
					onEditNote={(card, index, text) =>
						void run(editNote(board, card.id, index, text)).then((saved) => {
							if (!saved) keepNote(text);
						})
					}
					onRemoveNote={(card, index) =>
						void run(removeNote(board, card.id, index))
					}
				/>
			))}
			{!showAddColumn ? null : addingColumn ? (
				<NewColumn
					onCreate={(name) => void run(addColumn(board, name))}
					onCancel={() => setAddingColumn(false)}
				/>
			) : (
				<Button
					variant="subtle"
					size="icon"
					aria-label="Add column"
					// Sits on the column header line, matching header height.
					className="mt-0.5 size-7 min-w-0 border border-dashed border-content-secondary/40 p-0 hover:border-content-link hover:text-content-link [&>svg]:size-3.5! [&>svg]:p-0"
					onClick={() => setAddingColumn(true)}
				>
					<PlusIcon />
				</Button>
			)}
		</div>
	);
};
