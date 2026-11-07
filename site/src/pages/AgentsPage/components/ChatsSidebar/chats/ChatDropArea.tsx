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
 * themselves with the group-data-[drop=unavailable]/drop variant.
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
		disabled: !movedChat || unavailable,
		// The default detector falls back to the dragged row's overlap, which
		// would hand a drop on a muted folder to the target next to it.
		collisionDetector: pointerIntersection,
	});
	return (
		<div
			ref={ref}
			data-drop={unavailable ? "unavailable" : undefined}
			className={cn(
				"group/drop rounded-md",
				isDropTarget &&
					"outline-solid outline-1 -outline-offset-1 outline-content-link",
			)}
		>
			{children}
		</div>
	);
};
