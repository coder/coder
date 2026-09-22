import { PlusIcon } from "lucide-react";
import { type FC, useState } from "react";
import { toast } from "sonner";
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
} from "./boardApi";
import type { DropTarget } from "./boardDrag";
import type {
	BoardCard as BoardCardModel,
	BoardColumn as BoardColumnModel,
} from "./boardLabels";

type BoardColumnsProps = {
	/** Columns after the filter, drawn left to right. */
	readonly columns: readonly BoardColumnModel[];
	/** The unfiltered model that every command acts on. */
	readonly board: BoardState;
	readonly run: (plan: Plan | null) => Promise<boolean>;
	readonly openChatIds: ReadonlySet<string>;
	readonly dropTarget: DropTarget | null;
	readonly onAssistant: (card: BoardCardModel) => void;
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
	openChatIds,
	dropTarget,
	onAssistant,
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
					openChatIds={openChatIds}
					dropTarget={dropTarget}
					onRename={(to) => void run(renameColumn(board, column.name, to))}
					onDelete={() => void run(deleteColumn(board, column.name))}
					onSetCardTitle={(card, title) =>
						void run(renameCard(board, card.id, title))
					}
					onSetCardColor={(card, color) =>
						void run(setCardColor(board, card.id, color))
					}
					onRenameChat={(chat, title) =>
						void run(renameChat(board, chat.id, title))
					}
					onAssistant={onAssistant}
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
