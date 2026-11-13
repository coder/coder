import type { Plugins } from "@dnd-kit/abstract";
import { AutoScroller, PointerActivationConstraints } from "@dnd-kit/dom";
import {
	type DragDropManager,
	KeyboardSensor,
	PointerSensor,
} from "@dnd-kit/react";
import type { Chat } from "#/api/typesGenerated";
import { canDragChatToProject } from "../../components/ChatsSidebar/chats/projectDrag";
import type { DragData, DropData } from "./BoardCard";
import {
	type BoardState,
	detachChat,
	joinCard,
	mergeCards,
	mergeKeeper,
	moveCard,
	moveColumn,
	moveNote,
	type NoteSlot,
	type Plan,
} from "./boardApi";
import {
	BOARD_LANE,
	type BoardCard,
	laneOf,
	laneProjectId,
	NO_PROJECT_LANE,
} from "./boardLabels";

// Over a card, the top and bottom quarters insert before/after it; the
// middle half merges.
const EDGE_ZONE = 0.25;

// Small enough that a drag starts promptly, large enough that a click on a
// card header does not.
const DRAG_ACTIVATION_PX = 4;

/** The board's sensors; tests render with them so a click stays a click. */
export const dragSensors = [
	PointerSensor.configure({
		activationConstraints: [
			new PointerActivationConstraints.Distance({ value: DRAG_ACTIVATION_PX }),
		],
	}),
	KeyboardSensor,
];

/** dnd-kit's plugins, with auto-scroll slowed to the speed it had in v6. */
export const dragPlugins = (defaults: Plugins): Plugins => [
	...defaults.filter((plugin) => plugin !== AutoScroller),
	AutoScroller.configure({ acceleration: 16 }),
];

/** Where a drop would land, resolved from the pointer position. */
export type DropTarget =
	| { kind: "merge"; card: BoardCard; lane: string }
	| {
			kind: "insert";
			column: string;
			lane: string;
			beforeCardId: string | null;
	  }
	| { kind: "column"; name: string; side: "before" | "after" }
	| { kind: "note"; card: BoardCard; slot: NoteSlot }
	| { kind: "noteCard"; card: BoardCard };

type Point = { readonly x: number; readonly y: number };
type Rect = {
	readonly top: number;
	readonly left: number;
	readonly width: number;
	readonly height: number;
};

/** A registered drop zone and where it is on screen. */
export type BoardDroppable = { readonly data: DropData; readonly rect: Rect };

const contains = (rect: Rect, { x, y }: Point) =>
	x >= rect.left &&
	x <= rect.left + rect.width &&
	y >= rect.top &&
	y <= rect.top + rect.height;

// A column is drawn once per lane, so its cards are the ones in that lane.
const cardsInColumn = (
	droppables: readonly BoardDroppable[],
	column: string,
	lane: string,
) =>
	droppables
		.flatMap(({ data, rect }) =>
			data.type === "card" && data.card.column === column && data.lane === lane
				? [{ card: data.card, rect }]
				: [],
		)
		.sort((a, b) => a.rect.top - b.rect.top);

const nextCardIdInColumn = (
	droppables: readonly BoardDroppable[],
	card: BoardCard,
	lane: string,
): string | null => {
	const ordered = cardsInColumn(droppables, card.column, lane);
	const index = ordered.findIndex((entry) => entry.card.id === card.id);
	return ordered[index + 1]?.card.id ?? null;
};

// A note lands before or after the note under the pointer, or at the end
// of another card's notes when over the card itself. Its own card, away
// from its notes, is not a target.
const noteTarget = (
	hits: readonly BoardDroppable[],
	pointer: Point,
	drag: Extract<DragData, { type: "note" }>,
): DropTarget | null => {
	for (const { data, rect } of hits) {
		if (
			data.type === "note" &&
			!(data.card.id === drag.card.id && data.note.index === drag.note.index)
		) {
			return {
				kind: "note",
				card: data.card,
				slot: {
					index: data.note.index,
					side: pointer.y < rect.top + rect.height / 2 ? "before" : "after",
				},
			};
		}
	}
	for (const { data } of hits) {
		if (data.type === "card" && data.card.id !== drag.card.id) {
			return { kind: "noteCard", card: data.card };
		}
	}
	return null;
};

/** The lane a dragged card or chat is in; undefined for notes and columns. */
const dragLane = (drag: DragData | undefined): string | undefined => {
	if (drag?.type === "card") return laneOf(drag.card);
	if (drag?.type === "chat") return drag.chat.project_id ?? NO_PROJECT_LANE;
	return undefined;
};

/** The lane a card or chat lands in when dropped on target; undefined when none is drawn. */
const targetLane = (target: DropTarget): string | undefined =>
	(target.kind === "insert" || target.kind === "merge") &&
	target.lane !== BOARD_LANE
		? target.lane
		: undefined;

/**
 * Resolves the pointer to one target. A column drag lands before or after
 * the column under the pointer. Over a card, see EDGE_ZONE (chats always
 * join). Over column background, insert before the first card whose middle
 * is below the pointer. A drag that cannot change project has no target in
 * another lane, so nothing lights up there.
 */
export const boardTarget = (
	drag: DragData | undefined,
	droppables: readonly BoardDroppable[],
	pointer: Point | null,
	canChangeLane = true,
): DropTarget | null => {
	const target = pointerTarget(drag, droppables, pointer);
	if (!target || canChangeLane) return target;
	const from = dragLane(drag);
	const to = targetLane(target);
	return from === undefined || to === undefined || from === to ? target : null;
};

const pointerTarget = (
	drag: DragData | undefined,
	droppables: readonly BoardDroppable[],
	pointer: Point | null,
): DropTarget | null => {
	if (!pointer) return null;
	const hits = droppables.filter(({ rect }) => contains(rect, pointer));
	if (drag?.type === "note") return noteTarget(hits, pointer, drag);
	const columnHit = hits.find(({ data }) => data.type === "column");
	if (columnHit?.data.type !== "column") return null;
	const column = columnHit.data;

	if (drag?.type === "column") {
		if (column.name === drag.name) return null;
		const { rect } = columnHit;
		return {
			kind: "column",
			name: column.name,
			side: pointer.x < rect.left + rect.width / 2 ? "before" : "after",
		};
	}

	const cardHit = hits.find(
		({ data }) => data.type === "card" && data.card.id !== drag?.card.id,
	);
	if (cardHit?.data.type === "card") {
		const y = (pointer.y - cardHit.rect.top) / cardHit.rect.height;
		const card = cardHit.data.card;
		if (drag?.type === "chat" || (y > EDGE_ZONE && y < 1 - EDGE_ZONE)) {
			return { kind: "merge", card, lane: cardHit.data.lane };
		}
		return {
			kind: "insert",
			column: card.column,
			lane: cardHit.data.lane,
			beforeCardId:
				y <= EDGE_ZONE
					? card.id
					: nextCardIdInColumn(droppables, card, cardHit.data.lane),
		};
	}
	const below = cardsInColumn(droppables, column.name, column.lane).find(
		({ rect }) => rect.top + rect.height / 2 > pointer.y,
	);
	return {
		kind: "insert",
		column: column.name,
		lane: column.lane,
		beforeCardId: below?.card.id ?? null,
	};
};

/** The board's drag payload; dnd-kit types `data` as an open record. */
export const dragDataOf = (manager: DragDropManager): DragData | undefined =>
	manager.dragOperation.source?.data as DragData | undefined;

/**
 * The target under the drag's current position. Rects are read from the
 * elements, not dnd-kit's shapes, because dnd-kit drops the shape of an
 * element scrolled out of view and the column order needs every card.
 */
export const dragTargetOf = (
	manager: DragDropManager,
	currentUserId: string,
	assistantOf: AssistantOf,
): DropTarget | null => {
	const droppables = [...manager.registry.droppables].flatMap((droppable) => {
		const data = droppable.data as DropData | undefined;
		const element = droppable.element;
		return data && element && !droppable.disabled
			? [{ data, rect: element.getBoundingClientRect() }]
			: [];
	});
	return boardTarget(
		dragDataOf(manager),
		droppables,
		manager.dragOperation.position.current,
		dragCanChangeLane(dragDataOf(manager), currentUserId, assistantOf),
	);
};

/**
 * Whether a drag may land in another lane, which moves its chats to that
 * project. The server refuses to move a chat while it or a subagent works,
 * so the sidebar's rule for dragging a chat onto a project applies.
 */
export const dragCanChangeLane = (
	drag: DragData | undefined,
	currentUserId: string,
	assistantOf: AssistantOf,
): boolean => {
	let chats: readonly (Chat | undefined)[] = [];
	if (drag?.type === "chat") chats = [drag.chat];
	// The card's assistant can move with it, so it must be idle too.
	if (drag?.type === "card") {
		chats = [...drag.card.members, assistantOf(drag.card.id)];
	}
	return chats
		.filter((chat): chat is Chat => chat !== undefined)
		.filter((chat) => !chat.parent_chat_id)
		.every((chat) => canDragChatToProject(chat, currentUserId));
};

/** The assistant chat of a card, by card id. */
type AssistantOf = (cardId: string) => Chat | undefined;

/**
 * The board command a drop means, or null when that drag cannot land on
 * that target. Returned unapplied so the caller runs it against the full
 * model.
 */
export const dropCommand = (
	drag: DragData,
	target: DropTarget,
	assistantOf: AssistantOf = () => undefined,
): ((state: BoardState) => Plan | null) | null => {
	const command = labelCommand(drag, target);
	const from = dragLane(drag);
	const to = targetLane(target);
	if (!command || from === undefined || to === undefined || from === to) {
		return command;
	}
	const projectId = laneProjectId(to);
	// Only root chats move; subagents follow their root on the server. The
	// assistant goes wherever its card's primary goes: along with a moved
	// card, and with a merge that keeps the dragged card's primary.
	let chats: readonly (Chat | undefined)[] = [];
	if (drag.type === "chat") chats = [drag.chat];
	if (drag.type === "card") {
		chats = [
			...drag.card.members,
			...(target.kind === "insert" ||
			(target.kind === "merge" &&
				mergeKeeper(drag.card, target.card) === drag.card)
				? [assistantOf(drag.card.id)]
				: []),
		];
	}
	const projectMoves = chats
		.filter((chat): chat is Chat => chat !== undefined)
		.filter((chat) => !chat.parent_chat_id)
		.map((chat) => ({ chat, projectId }));
	// The labels may already say this column and place; the project still moves.
	return (state) => ({ writes: [], ...command(state), projectMoves });
};

const labelCommand = (
	drag: DragData,
	target: DropTarget,
): ((state: BoardState) => Plan | null) | null => {
	switch (target.kind) {
		case "column":
			return drag.type === "column"
				? (state) => moveColumn(state, drag.name, target)
				: null;
		case "note":
			return drag.type === "note"
				? (state) =>
						moveNote(
							state,
							drag.card.id,
							drag.note.index,
							target.card.id,
							target.slot,
						)
				: null;
		case "noteCard":
			return drag.type === "note"
				? (state) =>
						moveNote(state, drag.card.id, drag.note.index, target.card.id, null)
				: null;
		case "merge":
			if (drag.type === "card") {
				return (state) => mergeCards(state, drag.card.id, target.card.id);
			}
			if (drag.type === "chat") {
				return (state) => joinCard(state, drag.chat.id, target.card.id);
			}
			return null;
		case "insert":
			if (drag.type === "card") {
				return (state) =>
					moveCard(state, drag.card.id, target.column, target.beforeCardId);
			}
			if (drag.type === "chat") {
				return (state) =>
					detachChat(state, drag.chat.id, target.column, target.beforeCardId);
			}
			return null;
	}
};
