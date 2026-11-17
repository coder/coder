import { Plugin } from "@dnd-kit/abstract";
import { type Draggable, type Droppable, Scroller } from "@dnd-kit/dom";
import type {
	DragDropManager,
	DragEndEvent,
	DragMoveEvent,
	DragOverEvent,
	DragStartEvent,
} from "@dnd-kit/react";
import { isSortable } from "@dnd-kit/react/sortable";
import type { Chat } from "#/api/typesGenerated";
import {
	canManageChat,
	getArchiveBlockedReason,
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

/** The dnd-kit type of a chat row drag, which moves the chat to a project. */
export const CHAT_ROW_DRAG_TYPE = "chat-row";

/** The dnd-kit data of a chat row or pinned row drag. */
export type ChatDragData = { readonly chat: Chat };

/** The dnd-kit data of a chat drop target. */
export type DropTargetData = { readonly dropTarget: DropTarget };

/** The dnd-kit data of the Projects section header. */
export type ProjectsSectionData = { readonly projectsSection: true };

/** A drop on the Projects section header moves nothing. */
export const isProjectsSection = (target: Droppable | null): boolean =>
	target !== null && "projectsSection" in target.data;

const isChatDragData = (data: Record<string, unknown>): data is ChatDragData =>
	"chat" in data;

const isDropTargetData = (
	data: Record<string, unknown>,
): data is DropTargetData => "dropTarget" in data;

const draggedChatOf = (source: Draggable | null): Chat | undefined =>
	source && isChatDragData(source.data) ? source.data.chat : undefined;

/** The chat of a row drag, or undefined for a pin drag. */
export const movedChatOf = (
	source: Draggable | null | undefined,
): Chat | undefined =>
	source?.type === CHAT_ROW_DRAG_TYPE ? draggedChatOf(source) : undefined;

const dropTargetOf = (target: Droppable | null): DropTarget | undefined =>
	target && isDropTargetData(target.data) ? target.data.dropTarget : undefined;

/** A row can start a drag only where its menu turns "Move to project" on. */
export const canDragChatToProject = (
	chat: Chat,
	currentUserId: string,
): boolean =>
	canManageChat(chat, currentUserId) &&
	!chat.archived &&
	getParentChatID(chat) === undefined &&
	getArchiveBlockedReason(chat.status, chat.children) !== "active";

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
	target: Droppable | null,
): string | null | undefined => projectIdForTarget(chat, dropTargetOf(target));

const announcedTarget = (target: Droppable | null): string | undefined => {
	if (isProjectsSection(target)) {
		return "Projects";
	}
	const dropTarget = dropTargetOf(target);
	if (!dropTarget) {
		return undefined;
	}
	return dropTarget.kind === "project"
		? `project ${dropTarget.label}`
		: "Chats";
};

/**
 * Screen reader announcements of a drag, by dnd-kit event. They name the
 * dragged chat and, for a row drag, the folder under it, or for a pin drag,
 * its place among the pins.
 */
export const chatDragAnnouncements = {
	dragstart: ({ operation: { source } }: DragStartEvent) => {
		const chat = draggedChatOf(source);
		return chat && `Picked up chat ${chat.title}.`;
	},
	dragover: (
		{ operation: { source, target } }: DragOverEvent,
		pinCount: number,
	) => {
		const chat = movedChatOf(source);
		if (!chat) {
			const pin = draggedChatOf(source);
			// The dragged pin takes the index of the pin under it.
			return pin && isSortable(target)
				? `Chat ${pin.title} is at position ${target.index + 1} of ${pinCount}.`
				: undefined;
		}
		const over = announcedTarget(target);
		return over
			? `Chat ${chat.title} is over ${over}.`
			: `Chat ${chat.title} is not over a project or Chats.`;
	},
	dragend: ({ operation: { source, target }, canceled }: DragEndEvent) => {
		const chat = draggedChatOf(source);
		if (!chat) {
			return undefined;
		}
		if (canceled) {
			return `Moving chat ${chat.title} was canceled.`;
		}
		if (!movedChatOf(source)) {
			return `Chat ${chat.title} was dropped.`;
		}
		const over = announcedTarget(target);
		return over && projectIdForDrop(chat, target) !== undefined
			? `Chat ${chat.title} was dropped on ${over}.`
			: `Chat ${chat.title} was dropped. It did not move.`;
	},
};

const dropTargetsOf = (
	manager: DragDropManager,
	source: Draggable,
): Droppable[] =>
	[...manager.registry.droppables].filter(
		(target) => !target.disabled && target.accepts(source),
	);

const isKeyboardRowDrag = ({ dragOperation }: DragDropManager): boolean =>
	dragOperation.activatorEvent instanceof KeyboardEvent &&
	movedChatOf(dragOperation.source) !== undefined;

/**
 * Checks the drag target again on each scrolled frame, since dnd-kit skips
 * the check at an unchanged drag position. A keyboard row drag turns off
 * Scroller, so dnd-kit's own scrolling does not move the list off the target.
 */
export class SyncDragTargetOnScroll extends Plugin<DragDropManager> {
	constructor(manager: DragDropManager) {
		super(manager);
		this.registerEffect(() => {
			const { source, status } = manager.dragOperation;
			if (!source || !status.initialized) {
				return;
			}
			let frame: number | undefined;
			const checkTargets = () => {
				frame = undefined;
				for (const target of dropTargetsOf(manager, source)) {
					target.refreshShape();
				}
				manager.collisionObserver.forceUpdate();
			};
			const handleScroll = () => {
				frame ??= requestAnimationFrame(checkTargets);
			};
			const options = { capture: true, passive: true };
			document.addEventListener("scroll", handleScroll, options);
			return () => {
				document.removeEventListener("scroll", handleScroll, options);
				if (frame !== undefined) {
					cancelAnimationFrame(frame);
				}
			};
		});
		this.registerEffect(() => {
			const scroller = manager.registry.plugins.get(Scroller);
			if (
				!scroller ||
				!manager.dragOperation.status.initialized ||
				!isKeyboardRowDrag(manager)
			) {
				return;
			}
			scroller.disable();
			return () => scroller.enable();
		});
	}
}

/**
 * Up and Down take a keyboard row drag to the next drop target that would
 * move the chat, where the keyboard sensor would step 10 pixels. Pin drags
 * keep dnd-kit's sortable steps.
 */
export const moveKeyboardChatDrag = (
	event: DragMoveEvent,
	manager: DragDropManager,
): void => {
	const { source, position, shape } = manager.dragOperation;
	if (
		!source ||
		!(event.nativeEvent instanceof KeyboardEvent) ||
		!movedChatOf(source)
	) {
		return;
	}
	event.preventDefault();
	const step = Math.sign(event.by?.y ?? 0);
	if (step === 0) {
		return;
	}
	const { x, y } = position.current;
	const targets = dropTargetsOf(manager, source);
	const next = targets
		.map((target) => target.element)
		.filter((element) => element !== undefined)
		.map((element) => ({ element, rect: element.getBoundingClientRect() }))
		.filter(({ rect }) => (step > 0 ? rect.top > y : rect.bottom < y))
		.sort((a, b) => (a.rect.top - b.rect.top) * step)[0];
	if (!next) {
		return;
	}

	// A check made before the move can change the target, and dnd-kit then
	// drops collisions until React renders, so the checks stay off until the
	// move's queued position update has run.
	const { collisionObserver } = manager;
	collisionObserver.disable();
	next.element.scrollIntoView({ block: "nearest" });
	// dnd-kit measures the scrolled targets only later, so the final check
	// would see them where they were before the scroll.
	for (const target of targets) {
		target.refreshShape();
	}
	const rect = next.element.getBoundingClientRect();
	const rowHeight = shape?.current.boundingRectangle.height ?? rect.height;
	// Put the row's top on the target's top, or the row's middle on the middle
	// of a shorter target, so that the pointer is inside the target.
	manager.actions.move({
		to: { x, y: rect.top + Math.min(rect.height, rowHeight) / 2 },
	});

	// Queued after the move's position update. Enabling alone does not make
	// dnd-kit act on the collisions again.
	queueMicrotask(() => {
		collisionObserver.enable();
		collisionObserver.forceUpdate();
	});
};

/**
 * dnd-kit sorts the pins by moving the dragged pin's element once the drag
 * renders, which takes the focus from a keyboard drag, so the handle takes it
 * back after that move.
 */
export const refocusKeyboardPinDrag = (manager: DragDropManager): void => {
	const { source, activatorEvent } = manager.dragOperation;
	const handle = source?.handle;
	if (
		!(activatorEvent instanceof KeyboardEvent) ||
		!isSortable(source) ||
		!(handle instanceof HTMLElement)
	) {
		return;
	}
	void manager.renderer.rendering.then(() => setTimeout(() => handle.focus()));
};
