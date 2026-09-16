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
} from "./boardLabels";
import { INBOX_COLUMN } from "./boardLabels";
import { InlineText } from "./InlineText";

const columnDropId = (name: string) => `column:${name}`;

interface BoardColumnProps {
	readonly column: BoardColumnModel;
	readonly activeChatId: string | undefined;
	readonly onRename: (to: string) => void;
	readonly onDelete: () => void;
	readonly onSetCardTitle: (card: BoardCardModel, title: string) => void;
	readonly onRenameChat: (chat: Chat, title: string) => void;
	readonly onAddComment: (card: BoardCardModel, text: string) => void;
	readonly onRemoveComment: (card: BoardCardModel, index: number) => void;
}

export const BoardColumn: FC<BoardColumnProps> = ({
	column,
	activeChatId,
	onRename,
	onDelete,
	onSetCardTitle,
	onRenameChat,
	onAddComment,
	onRemoveComment,
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
				"flex w-80 shrink-0 flex-col gap-2 rounded-lg bg-surface-primary p-2",
				isOver && "ring-1 ring-content-link",
			)}
		>
			<header className="flex items-center gap-2 px-1 text-sm font-medium text-content-primary">
				{isInbox ? (
					<span className="flex-1">{column.name}</span>
				) : (
					<InlineText
						value={column.name}
						onSave={onRename}
						ariaLabel={`${column.name} column name`}
						className="flex-1"
					/>
				)}
				<span className="text-xs text-content-secondary tabular-nums">
					{column.cards.length}
				</span>
				{!isInbox && (
					<Button
						variant="subtle"
						size="icon"
						aria-label={`Delete ${column.name} column`}
						className="size-6"
						onClick={onDelete}
					>
						<Trash2Icon className="size-3.5" />
					</Button>
				)}
			</header>
			<div className="flex min-h-16 flex-1 flex-col gap-2 overflow-y-auto">
				{column.cards.map((card) => (
					<BoardCard
						key={card.id}
						card={card}
						activeChatId={activeChatId}
						onSetTitle={(title) => onSetCardTitle(card, title)}
						onRenameChat={onRenameChat}
						onAddComment={(text) => onAddComment(card, text)}
						onRemoveComment={(index) => onRemoveComment(card, index)}
					/>
				))}
			</div>
		</section>
	);
};
