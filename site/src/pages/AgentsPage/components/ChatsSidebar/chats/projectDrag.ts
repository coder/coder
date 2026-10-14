import {
	type Active,
	type ClientRect,
	type CollisionDetection,
	closestCenter,
	type Data,
	type DataRef,
	type KeyboardCoordinateGetter,
	pointerWithin,
	rectIntersection,
} from "@dnd-kit/core";
import { sortableKeyboardCoordinates } from "@dnd-kit/sortable";
import type { Chat } from "#/api/typesGenerated";
import {
	canManageChat,
	chatFamilyAllowsArchive,
} from "../../ChatActionsMenuItems";
import { getParentChatID } from "../tree/chatTree";

/** A drop target of a chat drag: a project folder, or Chats. */
export type DropTarget =
	| {
			readonly kind: "project";
			readonly projectId: string;
			readonly organizationId: string;
	  }
	| { readonly kind: "unfiled" };

/** The dnd-kit data of a chat row drag. */
export type ChatDragData = { readonly chat: Chat };

/** The dnd-kit data of a chat drop target. */
export type DropTargetData = { readonly dropTarget: DropTarget };

const isChatDragData = (data: Data | undefined): data is Data<ChatDragData> =>
	data !== undefined && "chat" in data;

const isDropTargetData = (
	data: Data | undefined,
): data is Data<DropTargetData> => data !== undefined && "dropTarget" in data;

/** The chat of a row drag, or undefined for a pin drag. */
export const draggedChatOf = (active: Active | null): Chat | undefined => {
	const data = active?.data.current;
	return isChatDragData(data) ? data.chat : undefined;
};

const dropTargetOf = (
	target: { readonly data: DataRef } | null,
): DropTarget | undefined => {
	const data = target?.data.current;
	return isDropTargetData(data) ? data.dropTarget : undefined;
};

/**
 * A row can start a drag only where its menu turns "Move to project" on. A
 * pinned row keeps its drag for the pin order, and its menu can still move it.
 */
export const canDragChatToProject = (
	chat: Chat,
	currentUserId: string,
): boolean =>
	canManageChat(chat, currentUserId) &&
	!chat.archived &&
	getParentChatID(chat) === undefined &&
	chatFamilyAllowsArchive(chat.status, chat.children) &&
	chat.pin_order === 0;

/**
 * The project_id that a drop on target sets: a project ID, null to remove the
 * chat from its project, or undefined when the drop changes nothing.
 */
export const projectIdForTarget = (
	chat: Chat,
	target: DropTarget | undefined,
): string | null | undefined => {
	if (target?.kind === "project") {
		const isNewProject =
			target.organizationId === chat.organization_id &&
			target.projectId !== chat.project_id;
		return isNewProject ? target.projectId : undefined;
	}
	return target?.kind === "unfiled" && chat.project_id ? null : undefined;
};

/**
 * As projectIdForTarget, for the target under a drop. A drop outside every
 * target changes nothing.
 */
export const projectIdForDrop = (
	chat: Chat,
	over: { readonly data: DataRef } | null,
): string | null | undefined => projectIdForTarget(chat, dropTargetOf(over));

/** A pin drag sorts among the pins. A chat drag sees only the drop targets. */
export const collideByDragKind: CollisionDetection = (args) => {
	if (!draggedChatOf(args.active)) {
		return closestCenter(args);
	}
	const droppableContainers = args.droppableContainers.filter(
		(container) => dropTargetOf(container) !== undefined,
	);
	// A keyboard drag has no pointer, so it uses the rectangles.
	return args.pointerCoordinates
		? pointerWithin({ ...args, droppableContainers })
		: rectIntersection({ ...args, droppableContainers });
};

/** Up and Down take a chat to the next drop target, and a pin to the next pin. */
export const dragKeyboardCoordinates: KeyboardCoordinateGetter = (
	event,
	args,
) => {
	const { active, collisionRect, droppableContainers, droppableRects } =
		args.context;
	if (!draggedChatOf(active)) {
		return sortableKeyboardCoordinates(event, args);
	}
	const isArrowKey = event.code === "ArrowDown" || event.code === "ArrowUp";
	if (!isArrowKey || !collisionRect) {
		return undefined;
	}
	event.preventDefault();
	const step = event.code === "ArrowDown" ? 1 : -1;
	const next = droppableContainers
		.getEnabled()
		.filter((target) => dropTargetOf(target) !== undefined)
		.map((target) => droppableRects.get(target.id))
		.filter((rect): rect is ClientRect => rect !== undefined)
		.filter((rect) => (rect.top - collisionRect.top) * step > 0)
		.sort((a, b) => (a.top - b.top) * step)[0];
	return next ? { x: next.left, y: next.top } : undefined;
};
