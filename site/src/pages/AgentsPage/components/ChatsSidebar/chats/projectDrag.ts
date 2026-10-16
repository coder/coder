import {
	type Active,
	type Announcements,
	type ClientRect,
	type CollisionDetection,
	closestCenter,
	type Data,
	type DataRef,
	defaultAnnouncements,
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
			/** The folder's accessible name, for screen reader announcements. */
			readonly label: string;
	  }
	| {
			readonly kind: "unfiled";
			/** The projects whose chats a drop here takes out of their folder. */
			readonly folderProjectIds: ReadonlySet<string>;
	  };

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

/** A row can start a drag only where its menu turns "Move to project" on. */
export const canDragChatToProject = (
	chat: Chat,
	currentUserId: string,
): boolean =>
	canManageChat(chat, currentUserId) &&
	!chat.archived &&
	getParentChatID(chat) === undefined &&
	chatFamilyAllowsArchive(chat.status, chat.children);

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
	if (target?.kind !== "unfiled" || !chat.project_id) {
		return undefined;
	}
	return target.folderProjectIds.has(chat.project_id) ? null : undefined;
};

/**
 * As projectIdForTarget, for the target under a drop. A drop outside every
 * target changes nothing.
 */
export const projectIdForDrop = (
	chat: Chat,
	over: { readonly data: DataRef } | null,
): string | null | undefined => projectIdForTarget(chat, dropTargetOf(over));

const announcedTarget = (
	over: { readonly data: DataRef } | null,
): string | undefined => {
	const target = dropTargetOf(over);
	if (!target) {
		return undefined;
	}
	return target.kind === "project" ? `project ${target.label}` : "Chats";
};

/**
 * Screen reader announcements that name the dragged chat and the folder under
 * it, where the dnd-kit defaults read out internal IDs. A pin drag keeps the
 * defaults.
 */
export const chatDragAnnouncements: Announcements = {
	onDragStart: ({ active }) => {
		const chat = draggedChatOf(active);
		return chat
			? `Picked up chat ${chat.title}.`
			: defaultAnnouncements.onDragStart({ active });
	},
	onDragOver: ({ active, over }) => {
		const chat = draggedChatOf(active);
		if (!chat) {
			return defaultAnnouncements.onDragOver({ active, over });
		}
		const target = announcedTarget(over);
		return target
			? `Chat ${chat.title} is over ${target}.`
			: `Chat ${chat.title} is not over a project or Chats.`;
	},
	onDragEnd: ({ active, over }) => {
		const chat = draggedChatOf(active);
		if (!chat) {
			return defaultAnnouncements.onDragEnd({ active, over });
		}
		const target = announcedTarget(over);
		return target
			? `Chat ${chat.title} was dropped on ${target}.`
			: `Chat ${chat.title} was dropped. It did not move.`;
	},
	onDragCancel: ({ active, over }) => {
		const chat = draggedChatOf(active);
		return chat
			? `Moving chat ${chat.title} was canceled.`
			: defaultAnnouncements.onDragCancel({ active, over });
	},
};

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
