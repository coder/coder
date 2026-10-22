import { useSortable } from "@dnd-kit/react/sortable";
import { cn } from "cn";
import type { Chat } from "#/api/typesGenerated";
import type { ChatDragData } from "../chats/projectDrag";
import { useChatTree } from "./ChatTreeContext";
import { ChatTreeNode } from "./ChatTreeNode";

/** The dnd-kit type of a pinned row drag, which only reorders the pins. */
const PIN_DRAG_TYPE = "pin";

type SortableChatTreeNodeProps = {
	readonly chat: Chat;
	readonly index: number;
};

export const SortableChatTreeNode: React.FC<SortableChatTreeNodeProps> = ({
	chat,
	index,
}) => {
	const { dragInstructionsId } = useChatTree();
	const { ref, handleRef, isDragging } = useSortable({
		id: chat.id,
		index,
		type: PIN_DRAG_TYPE,
		accept: PIN_DRAG_TYPE,
		data: { chat } satisfies ChatDragData,
	});

	return (
		<div ref={ref} className={cn(isDragging && "opacity-50")}>
			<ChatTreeNode
				chat={chat}
				dragHandleProps={{
					ref: handleRef,
					"aria-describedby": dragInstructionsId,
				}}
			/>
		</div>
	);
};
