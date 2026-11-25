import { useDraggable } from "@dnd-kit/react";
import { cn } from "cn";
import type { Chat } from "#/api/typesGenerated";
import {
	CHAT_ROW_DRAG_TYPE,
	type ChatDragData,
	canDragChatToProject,
} from "../chats/projectDrag";
import { useChatTree } from "./ChatTreeContext";
import { ChatTreeNode } from "./ChatTreeNode";

type DraggableChatTreeNodeProps = {
	readonly chat: Chat;
};

/**
 * A row that its owner can drag to move the chat. The wrapper stays mounted
 * when the chat stops being movable, so a status change does not remount the
 * row and close its open menu.
 */
export const DraggableChatTreeNode: React.FC<DraggableChatTreeNodeProps> = ({
	chat,
}) => {
	const { currentUserId, dragInstructionsId } = useChatTree();
	const canDrag = canDragChatToProject(chat, currentUserId);
	const { ref, handleRef, isDragging } = useDraggable({
		id: `move:${chat.id}`,
		type: CHAT_ROW_DRAG_TYPE,
		data: { chat } satisfies ChatDragData,
		disabled: !canDrag,
	});
	return (
		<div ref={ref} className={cn(isDragging && "opacity-50")}>
			<ChatTreeNode
				chat={chat}
				dragHandleProps={{
					ref: handleRef,
					"aria-describedby": canDrag ? dragInstructionsId : undefined,
				}}
			/>
		</div>
	);
};

type ChatDragPreviewProps = {
	readonly chat: Chat;
};

/** The copy of a dragged row that follows the pointer. */
export const ChatDragPreview: React.FC<ChatDragPreviewProps> = ({ chat }) => (
	<div className="max-w-60 truncate rounded-md border border-solid border-border bg-surface-secondary px-2.5 py-1 text-sm text-content-primary shadow-md">
		{chat.title}
	</div>
);
