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
import type { Chat, ChatProject } from "#/api/typesGenerated";
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

/** The dnd-kit type of a pinned project drag, which only reorders the pins. */
export const PROJECT_PIN_DRAG_TYPE = "project-pin";

/**
 * Carries the whole project, not its ID: the drop sends the pin request with
 * its organization and the announcements read the folder's label, even if the
 * summaries change during the drag.
 */
export type ProjectPinDragData = {
	readonly project: ChatProject;
	readonly label: string;
};

const isProjectPinDragData = (
	data: Record<string, unknown>,
): data is ProjectPinDragData => "project" in data;

/** The data of a pinned project drag, or undefined for any other drag. */
export const pinnedProjectOf = (
	source: Draggable | null,
): ProjectPinDragData | undefined =>
	source?.type === PROJECT_PIN_DRAG_TYPE && isProjectPinDragData(source.data)
		? source.data
		: undefined;

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
 * dragged chat or pinned project and, for a row drag, the folder under it, or
 * for a pin drag, its place among the pins.
 */
export const chatDragAnnouncements = {
	dragstart: ({ operation: { source } }: DragStartEvent) => {
		const project = pinnedProjectOf(source)?.label;
		if (project) {
			return `Picked up project ${project}.`;
		}
		const chat = draggedChatOf(source);
		return chat && `Picked up chat ${chat.title}.`;
	},
	dragover: (
		{ operation: { source, target } }: DragOverEvent,
		chatPinCount: number,
		projectPinCount: number,
	) => {
		const project = pinnedProjectOf(source)?.label;
		if (project) {
			return isSortable(target)
				? `Project ${project} is at position ${target.index + 1} of ${projectPinCount}.`
				: undefined;
		}
		const chat = movedChatOf(source);
		if (!chat) {
			const pin = draggedChatOf(source);
			// The dragged pin takes the index of the pin under it.
			return pin && isSortable(target)
				? `Chat ${pin.title} is at position ${target.index + 1} of ${chatPinCount}.`
				: undefined;
		}
		const over =
			projectIdForTarget(chat, dropTargetOf(target)) === undefined
				? undefined
				: announcedTarget(target);
		return over
			? `Chat ${chat.title} is over ${over}.`
			: `Chat ${chat.title} is not over a project or Chats.`;
	},
	dragend: ({ operation: { source, target }, canceled }: DragEndEvent) => {
		const project = pinnedProjectOf(source)?.label;
		if (project) {
			return canceled
				? `Moving project ${project} was canceled.`
				: `Project ${project} was dropped.`;
		}
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
	const chat = movedChatOf(source);
	if (!source || !(event.nativeEvent instanceof KeyboardEvent) || !chat) {
		return;
	}
	event.preventDefault();
	const step = Math.sign(event.by?.y ?? 0);
	if (step === 0) {
		return;
	}
	const { x, y } = position.current;
	// The keyboard stops skip the folders that would move nothing.
	const targets = dropTargetsOf(manager, source).filter(
		(target) => projectIdForTarget(chat, dropTargetOf(target)) !== undefined,
	);
	// The drag stops on a target's top row, so Chats, which holds the
	// folders, is a stop above them.
	const topRowBottom = (rect: DOMRect) =>
		rect.top +
		Math.min(
			rect.height,
			shape?.current.boundingRectangle.height ?? rect.height,
		);
	const next = targets
		.map((target) => target.element)
		.filter((element) => element !== undefined)
		.map((element) => ({ element, rect: element.getBoundingClientRect() }))
		.filter(({ rect }) => (step > 0 ? rect.top > y : topRowBottom(rect) < y))
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

/**
 * Orders pins by the order a drop set until the server's order arrives, or
 * keeps the given order when no drop is waiting on the server.
 */
export const applyLocalPinOrder = <T>(
	pins: readonly T[],
	localOrder: readonly string[] | null,
	idOf: (pin: T) => string,
): readonly T[] =>
	localOrder
		? localOrder
				.map((id) => pins.find((pin) => idOf(pin) === id))
				.filter((pin): pin is T => pin !== undefined)
		: pins;

/**
 * The pin IDs after a drop moved id to index `to`, or undefined when the drop
 * changed nothing.
 */
export const movePinnedId = (
	ids: readonly string[],
	id: string,
	to: number,
): string[] | undefined => {
	const from = ids.indexOf(id);
	return from === -1 || from === to
		? undefined
		: ids.toSpliced(from, 1).toSpliced(to, 0, id);
};
