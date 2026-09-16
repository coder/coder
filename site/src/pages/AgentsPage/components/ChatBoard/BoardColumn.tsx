import { useDroppable } from "@dnd-kit/core";
import { cn } from "cn";
import { Trash2Icon } from "lucide-react";
import type { FC } from "react";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { BoardCard, type DropData } from "./BoardCard";
import type {
	BoardCard as BoardCardModel,
	BoardColumn as BoardColumnModel,
	CardColor,
} from "./boardLabels";
import { INBOX_COLUMN } from "./boardLabels";
import { EditableText, InlineInput } from "./InlineText";

const columnDropId = (name: string) => `column:${name}`;

// Lanes have no fill; cards are the objects. A hairline separates lanes.
const columnClass =
	"group/column flex w-80 shrink-0 flex-col gap-2 border-r border-border pr-3 last:border-r-0";
const columnHeaderClass =
	"flex h-7 items-center gap-1 px-1 text-sm font-medium text-content-primary";

interface BoardColumnProps {
	readonly column: BoardColumnModel;
	readonly activeChatId: string | undefined;
	readonly onRename: (to: string) => void;
	readonly onDelete: () => void;
	readonly onSetCardTitle: (card: BoardCardModel, title: string) => void;
	readonly onSetCardColor: (
		card: BoardCardModel,
		color: CardColor | undefined,
	) => void;
	readonly onRenameChat: (chat: Chat, title: string) => void;
	readonly onAddNote: (card: BoardCardModel, text: string) => void;
	readonly onEditNote: (
		card: BoardCardModel,
		index: number,
		text: string,
	) => void;
	readonly onRemoveNote: (card: BoardCardModel, index: number) => void;
}

export const BoardColumn: FC<BoardColumnProps> = ({
	column,
	activeChatId,
	onRename,
	onDelete,
	onSetCardTitle,
	onSetCardColor,
	onRenameChat,
	onAddNote,
	onEditNote,
	onRemoveNote,
}) => {
	const dropData: DropData = { type: "column", name: column.name };
	const { setNodeRef, isOver } = useDroppable({
		id: columnDropId(column.name),
		data: dropData,
	});
	const isInbox = column.name === INBOX_COLUMN;

	return (
		<section
			ref={setNodeRef}
			aria-label={`${column.name} column`}
			className={cn(
				columnClass,
				isOver && "rounded-lg bg-surface-secondary/50 ring-1 ring-content-link",
			)}
		>
			<header className={columnHeaderClass}>
				{isInbox ? (
					<span className="min-w-0 flex-1 truncate">{column.name}</span>
				) : (
					<EditableText
						value={column.name}
						onSave={onRename}
						ariaLabel={`${column.name} column name`}
						revealOn="column"
					/>
				)}
				<span className="px-1 text-xs text-content-secondary tabular-nums">
					{column.cards.length}
				</span>
				{!isInbox && (
					<Button
						variant="subtle"
						size="icon"
						aria-label={`Delete ${column.name} column`}
						className="size-6 text-content-secondary opacity-0 group-hover/column:opacity-100 focus-visible:opacity-100"
						onClick={onDelete}
					>
						<Trash2Icon className="size-3.5" />
					</Button>
				)}
			</header>
			<div className="flex min-h-16 flex-1 flex-col gap-2 overflow-y-auto pb-2">
				{column.cards.map((card) => (
					<BoardCard
						key={card.id}
						card={card}
						activeChatId={activeChatId}
						onSetTitle={(title) => onSetCardTitle(card, title)}
						onSetColor={(color) => onSetCardColor(card, color)}
						onRenameChat={onRenameChat}
						onAddNote={(text) => onAddNote(card, text)}
						onEditNote={(index, text) => onEditNote(card, index, text)}
						onRemoveNote={(index) => onRemoveNote(card, index)}
					/>
				))}
			</div>
		</section>
	);
};

interface NewColumnProps {
	readonly onCreate: (name: string) => void;
	readonly onCancel: () => void;
}

/** A column shell with its title in edit mode, so creating looks like renaming. */
export const NewColumn: FC<NewColumnProps> = ({ onCreate, onCancel }) => (
	<section aria-label="New column" className={cn(columnClass, "min-h-24")}>
		<header className={columnHeaderClass}>
			<InlineInput
				value=""
				ariaLabel="New column name"
				className="flex-1"
				onSave={onCreate}
				onDone={onCancel}
			/>
		</header>
	</section>
);
