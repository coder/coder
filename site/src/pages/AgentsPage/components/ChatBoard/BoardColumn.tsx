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
import { columnColor, INBOX_COLUMN } from "./boardLabels";
import type { DropTarget } from "./ChatBoardPage";
import { EditableText, InlineInput } from "./InlineText";

const columnDropId = (name: string) => `column:${name}`;

const columnClass = "flex w-[300px] shrink-0 flex-col min-h-0";
// The header is its own hover group so its controls do not light up while
// hovering cards below it.
const columnHeaderClass =
	"group/column flex items-center gap-2 px-1.5 pt-0.5 pb-2.5 text-[13px] font-medium text-content-primary";

interface BoardColumnProps {
	readonly column: BoardColumnModel;
	readonly openChatIds: ReadonlySet<string>;
	readonly dropTarget: DropTarget | null;
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
	openChatIds,
	dropTarget,
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
	const { setNodeRef } = useDroppable({
		id: columnDropId(column.name),
		data: dropData,
	});
	const isInbox = column.name === INBOX_COLUMN;
	const insertBefore =
		dropTarget?.kind === "insert" && dropTarget.column === column.name
			? dropTarget.beforeCardId
			: undefined;
	const mergeTargetId =
		dropTarget?.kind === "merge" ? dropTarget.card.id : undefined;

	return (
		<section
			ref={setNodeRef}
			aria-label={`${column.name} column`}
			className={columnClass}
		>
			<header className={columnHeaderClass}>
				<ColumnDot name={column.name} />
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
				<span className="font-mono text-[11px] text-content-secondary/70 tabular-nums">
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
			<div className="flex min-h-16 flex-1 flex-col overflow-y-auto pb-2">
				{column.cards.map((card) => (
					<div key={card.id} className="flex flex-col">
						<InsertionLine visible={insertBefore === card.id} />
						<BoardCard
							card={card}
							openChatIds={openChatIds}
							isMergeTarget={mergeTargetId === card.id}
							onSetTitle={(title) => onSetCardTitle(card, title)}
							onSetColor={(color) => onSetCardColor(card, color)}
							onRenameChat={onRenameChat}
							onAddNote={(text) => onAddNote(card, text)}
							onEditNote={(index, text) => onEditNote(card, index, text)}
							onRemoveNote={(index) => onRemoveNote(card, index)}
						/>
					</div>
				))}
				<InsertionLine visible={insertBefore === null} />
			</div>
		</section>
	);
};

const ColumnDot: FC<{ readonly name: string }> = ({ name }) => (
	<span
		className={cn("size-2 shrink-0 rounded-[2px]", columnColor(name).dot)}
	/>
);

// Occupies the gap between cards, so showing it does not shift layout.
const InsertionLine: FC<{ readonly visible: boolean }> = ({ visible }) => (
	<div className="flex h-2.5 items-center">
		<div
			className={cn(
				"h-0.5 w-full rounded bg-content-link",
				!visible && "invisible",
			)}
		/>
	</div>
);

interface NewColumnProps {
	readonly onCreate: (name: string) => void;
	readonly onCancel: () => void;
}

/** A column shell with its title in edit mode, so creating looks like renaming. */
export const NewColumn: FC<NewColumnProps> = ({ onCreate, onCancel }) => (
	<section aria-label="New column" className={cn(columnClass, "min-h-24")}>
		<header className={columnHeaderClass}>
			<span className="size-2 shrink-0 rounded-[2px] bg-content-secondary/40" />
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
