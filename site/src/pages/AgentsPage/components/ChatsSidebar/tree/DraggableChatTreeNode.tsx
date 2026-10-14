import { useDraggable } from "@dnd-kit/core";
import { cn } from "cn";
import type { Chat } from "#/api/typesGenerated";
import { canDragChatToProject } from "../chats/projectDrag";
import { useChatTree } from "./ChatTreeContext";
import { ChatTreeNode } from "./ChatTreeNode";

/** A root row in a folder or in Chats. Its owner can drag it to move the chat. */
export const DraggableChatTreeNode: React.FC<{ readonly chat: Chat }> = ({
	chat,
}) => {
	const { currentUserId } = useChatTree();
	// Rows that cannot move stay plain rows, with no drag role for screen readers.
	if (!canDragChatToProject(chat, currentUserId)) {
		return <ChatTreeNode chat={chat} />;
	}
	return <DraggableRow chat={chat} />;
};

const DraggableRow: React.FC<{ readonly chat: Chat }> = ({ chat }) => {
	const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
		id: `move:${chat.id}`,
		data: { chat },
	});
	return (
		<div
			ref={setNodeRef}
			className={cn(isDragging && "opacity-50")}
			{...attributes}
			{...listeners}
		>
			<ChatTreeNode chat={chat} />
		</div>
	);
};

/** The copy of a dragged row that follows the pointer. */
export const ChatDragPreview: React.FC<{ readonly chat: Chat }> = ({
	chat,
}) => (
	<div className="max-w-60 truncate rounded-md border border-solid border-border bg-surface-secondary px-2.5 py-1 text-sm text-content-primary shadow-md">
		{chat.title}
	</div>
);
