import { CollisionPriority } from "@dnd-kit/abstract";
import { pointerIntersection } from "@dnd-kit/collision";
import { useDragOperation, useDroppable } from "@dnd-kit/react";
import { cn } from "cn";
import {
	type DropTarget,
	type DropTargetData,
	movedChatOf,
	projectIdForTarget,
} from "./projectDrag";

type ChatDropAreaProps = React.PropsWithChildren<{
	readonly target: DropTarget;
}>;

/**
 * Takes the drop of a chat that it would move. dnd-kit leaves disabled targets
 * out of the collision and of the keyboard stops; during a drag, headers mute
 * themselves with the group-data-[drop=unavailable]/drop variant, or /project-drop
 * in a folder. A folder that cannot take the chat stays enabled and takes its
 * drop as a no-op, so Chats around it cannot take the chat out of its project.
 */
export const ChatDropArea: React.FC<ChatDropAreaProps> = ({
	target,
	children,
}) => {
	const movedChat = movedChatOf(useDragOperation().source);
	const unavailable =
		movedChat !== undefined &&
		projectIdForTarget(movedChat, target) === undefined;
	const { ref, isDropTarget } = useDroppable({
		id: target.kind === "project" ? `project:${target.projectId}` : "unfiled",
		data: { dropTarget: target } satisfies DropTargetData,
		disabled: !movedChat || (unavailable && target.kind === "unfiled"),
		// The default detector falls back to the dragged row's overlap, which
		// would hand a drop on a muted folder to the target next to it.
		collisionDetector: pointerIntersection,
		// Pinned project folders sit inside Chats, and a drop on one goes to it.
		collisionPriority:
			target.kind === "unfiled" ? CollisionPriority.Low : undefined,
	});
	return (
		<div
			ref={ref}
			data-drop={unavailable ? "unavailable" : undefined}
			className={cn(
				"rounded-md",
				target.kind === "project" ? "group/project-drop" : "group/drop",
				isDropTarget &&
					!unavailable &&
					"outline-solid outline-1 -outline-offset-1 outline-content-link",
			)}
		>
			{children}
		</div>
	);
};
