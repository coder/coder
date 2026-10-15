import { useDroppable } from "@dnd-kit/core";
import { cn } from "cn";
import type { Chat } from "#/api/typesGenerated";
import {
	type DropTarget,
	type DropTargetData,
	projectIdForTarget,
} from "./projectDrag";

type ChatDropAreaProps = React.PropsWithChildren<{
	readonly target: DropTarget;
	readonly draggedChat: Chat | undefined;
}>;

/**
 * Takes the drop of a chat that it would move. dnd-kit leaves disabled targets
 * out of the collision and of the keyboard stops; during a drag, headers mute
 * themselves with the group-data-[drop=unavailable]/drop variant.
 */
export const ChatDropArea: React.FC<ChatDropAreaProps> = ({
	target,
	draggedChat,
	children,
}) => {
	const unavailable =
		draggedChat !== undefined &&
		projectIdForTarget(draggedChat, target) === undefined;
	const { setNodeRef, isOver } = useDroppable({
		id: target.kind === "project" ? `project:${target.projectId}` : "unfiled",
		data: { dropTarget: target } satisfies DropTargetData,
		disabled: !draggedChat || unavailable,
	});
	return (
		<div
			ref={setNodeRef}
			data-drop={unavailable ? "unavailable" : undefined}
			className={cn(
				"group/drop rounded-md",
				isOver &&
					"outline-solid outline-1 -outline-offset-1 outline-content-link",
			)}
		>
			{children}
		</div>
	);
};
