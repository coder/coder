import { PlusIcon } from "lucide-react";
import { type FC, useState } from "react";
import { toast } from "sonner";
import type { Chat } from "#/api/typesGenerated";
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

type BoardColumnsProps = {
	/** Columns after the filter, drawn left to right. */
	readonly columns: readonly BoardColumnModel[];
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
} & ChatOpenHandlers;

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

export const BoardColumns: FC<BoardColumnsProps> = ({
	columns,
	board,
	run,
	assistants,
	openChatIds,
	dropTarget,
	knownEfforts,
	onCardAssistant,
	onNewChat,
	onFilterEffort,
	onOpen,
	onPreview,
	onPreviewEnd,
}) => {
	const [addingColumn, setAddingColumn] = useState(false);

	return (
		<div className="flex min-h-0 flex-1 gap-4 overflow-x-auto bg-surface-secondary px-5 pt-4 pb-3">
			{columns.map((column) => (
				<BoardColumn
					key={column.name}
					column={column}
					assistants={assistants}
					openChatIds={openChatIds}
					dropTarget={dropTarget}
					knownEfforts={knownEfforts}
					onRename={(to) => void run(renameColumn(board, column.name, to))}
					onDelete={() => void run(deleteColumn(board, column.name))}
					onNewChat={() => onNewChat({ column: column.name })}
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
			{addingColumn ? (
				<NewColumn
					onCreate={(name) => void run(addColumn(board, name))}
					onCancel={() => setAddingColumn(false)}
				/>
			) : (
				<button
					type="button"
					aria-label="Add column"
					// Sits on the column header line, matching header height.
					className="mt-0.5 grid size-7 shrink-0 place-items-center rounded-md border border-dashed border-content-secondary/40 bg-transparent text-content-secondary hover:border-content-link hover:text-content-link"
					onClick={() => setAddingColumn(true)}
				>
					<PlusIcon className="size-3.5" />
				</button>
			)}
		</div>
	);
};
