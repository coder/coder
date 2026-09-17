import { type FC, useState } from "react";
import type { ChatOpenHandlers } from "./BoardCard";
import { BoardColumn, NewColumn, PlusButton } from "./BoardColumn";
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
import type { DraftTarget } from "./boardStorage";

interface BoardColumnsProps extends ChatOpenHandlers {
	/** Columns after the filter, drawn left to right. */
	readonly columns: readonly BoardColumnModel[];
	/** The unfiltered model that every command acts on. */
	readonly board: BoardState;
	readonly run: (plan: Plan | null) => Promise<void>;
	readonly openChatIds: ReadonlySet<string>;
	readonly dropTarget: DropTarget | null;
	readonly onAssistant: (card: BoardCardModel) => void;
	/** Opens the create form for a chat born in a column or on a card. */
	readonly onNewChat: (target: DraftTarget) => void;
}

export const BoardColumns: FC<BoardColumnsProps> = ({
	columns,
	board,
	run,
	openChatIds,
	dropTarget,
	onAssistant,
	onNewChat,
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
					onNewChat={() => onNewChat({ column: column.name })}
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
					onNewChatInCard={(card) => onNewChat({ cardId: card.id })}
					onRemoveFromGroup={(chat) =>
						void run(removeFromGroup(board, chat.id))
					}
					onOpen={onOpen}
					onPreview={onPreview}
					onPreviewEnd={onPreviewEnd}
					onAddNote={(card, text) => void run(addNote(board, card.id, text))}
					onEditNote={(card, index, text) =>
						void run(editNote(board, card.id, index, text))
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
				<PlusButton
					label="Add column"
					title="Add column"
					// Sits on the column header line, matching header height.
					className="mt-0.5"
					onClick={() => setAddingColumn(true)}
				/>
			)}
		</div>
	);
};
