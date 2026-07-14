import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import type { FC } from "react";
import type { Chat } from "#/api/typesGenerated";
import { cn } from "#/utils/cn";
import { ChatTreeNode } from "./ChatTreeNode";

export const SortableChatTreeNode: FC<{
	chat: Chat;
}> = ({ chat }) => {
	const {
		attributes,
		listeners,
		setNodeRef,
		transform,
		transition,
		isDragging,
	} = useSortable({
		id: chat.id,
		// Skip the derived-transform measurement after drop.
		// localPinOrder already repositions items in the DOM,
		// so the two-frame snap-back dance produces stale deltas
		// and a visible jitter. This makes items snap directly.
		animateLayoutChanges: () => false,
	});

	// Strip scaleX/scaleY that dnd-kit adds by default.
	const adjustedTransform = transform
		? { ...transform, scaleX: 1, scaleY: 1 }
		: null;

	const style = {
		transform: CSS.Transform.toString(adjustedTransform),
		transition: isDragging ? "opacity 200ms" : transition,
	};

	return (
		<div
			ref={setNodeRef}
			style={style}
			className={cn(isDragging && "opacity-50")}
			{...attributes}
			{...listeners}
		>
			<ChatTreeNode chat={chat} isChildNode={false} />
		</div>
	);
};
