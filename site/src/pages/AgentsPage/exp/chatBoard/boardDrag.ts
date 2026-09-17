import {
	type Active,
	type Collision,
	type CollisionDetection,
	pointerWithin,
} from "@dnd-kit/core";
import type { DragData, DropData } from "./BoardCard";
import {
	type BoardState,
	detachChat,
	joinCard,
	mergeCards,
	moveCard,
	moveColumn,
	moveNote,
	type NoteSlot,
	type Plan,
} from "./boardApi";
import type { BoardCard } from "./boardLabels";

// Over a card, the top and bottom quarters insert before/after it; the
// middle half merges.
const EDGE_ZONE = 0.25;

/** Where a drop would land, resolved from the pointer position. */
export type DropTarget =
	| { kind: "merge"; card: BoardCard }
	| { kind: "insert"; column: string; beforeCardId: string | null }
	| { kind: "column"; name: string; side: "before" | "after" }
	| { kind: "note"; card: BoardCard; slot: NoteSlot }
	| { kind: "noteCard"; card: BoardCard };

type CollisionArgs = Parameters<CollisionDetection>[0];

/** The board's drag payload; dnd-kit types `data` as an open record. */
export const dragDataOf = (active: Active): DragData | undefined =>
	active.data.current as DragData | undefined;

const dropDataOf = (hit: Collision): DropData | undefined =>
	hit.data?.droppableContainer?.data.current as DropData | undefined;

const cardsInColumn = (args: CollisionArgs, column: string) =>
	args.droppableContainers
		.flatMap((container) => {
			const data = container.data.current as DropData | undefined;
			const rect = args.droppableRects.get(container.id);
			return data?.type === "card" && data.card.column === column && rect
				? [{ card: data.card, rect }]
				: [];
		})
		.sort((a, b) => a.rect.top - b.rect.top);

const nextCardIdInColumn = (
	args: CollisionArgs,
	card: BoardCard,
): string | null => {
	const ordered = cardsInColumn(args, card.column);
	const index = ordered.findIndex((entry) => entry.card.id === card.id);
	return ordered[index + 1]?.card.id ?? null;
};

// A note lands before or after the note under the pointer, or at the end
// of another card's notes when over the card itself. Its own card, away
// from its notes, is not a target.
const noteCollision = (
	args: CollisionArgs,
	hits: readonly Collision[],
	drag: Extract<DragData, { type: "note" }>,
): Collision[] => {
	const pointer = args.pointerCoordinates;
	if (!pointer) return [];
	const noteHit = hits.find((hit) => {
		const data = dropDataOf(hit);
		return (
			data?.type === "note" &&
			!(data.card.id === drag.card.id && data.note.index === drag.note.index)
		);
	});
	if (noteHit) {
		const data = dropDataOf(noteHit);
		const rect = args.droppableRects.get(noteHit.id);
		if (data?.type !== "note" || !rect) return [];
		const target: DropTarget = {
			kind: "note",
			card: data.card,
			slot: {
				index: data.note.index,
				side: pointer.y < rect.top + rect.height / 2 ? "before" : "after",
			},
		};
		return [{ id: noteHit.id, data: { ...noteHit.data, target } }];
	}
	const cardHit = hits.find((hit) => {
		const data = dropDataOf(hit);
		return data?.type === "card" && data.card.id !== drag.card.id;
	});
	const cardData = cardHit && dropDataOf(cardHit);
	if (!cardHit || cardData?.type !== "card") return [];
	const target: DropTarget = { kind: "noteCard", card: cardData.card };
	return [{ id: cardHit.id, data: { ...cardHit.data, target } }];
};

/**
 * Resolves the pointer to one target. A column drag lands before or after
 * the column under the pointer. Over a card, see EDGE_ZONE (chats always
 * join). Over column background, insert before the first card whose middle
 * is below the pointer. The target rides along as collision data.
 */
export const boardCollision: CollisionDetection = (args) => {
	const pointer = args.pointerCoordinates;
	if (!pointer) return [];
	const hits = pointerWithin(args);
	const drag = dragDataOf(args.active);
	if (drag?.type === "note") return noteCollision(args, hits, drag);
	const columnHit = hits.find((hit) => dropDataOf(hit)?.type === "column");
	if (!columnHit) return [];
	const columnData = dropDataOf(columnHit);
	if (columnData?.type !== "column") return [];

	if (drag?.type === "column") {
		const rect = args.droppableRects.get(columnHit.id);
		if (!rect || columnData.name === drag.name) return [];
		const target: DropTarget = {
			kind: "column",
			name: columnData.name,
			side: pointer.x < rect.left + rect.width / 2 ? "before" : "after",
		};
		return [{ id: columnHit.id, data: { ...columnHit.data, target } }];
	}

	const cardHit = hits.find((hit) => {
		const data = dropDataOf(hit);
		return data?.type === "card" && data.card.id !== drag?.card.id;
	});
	let target: DropTarget;
	if (cardHit) {
		const cardData = dropDataOf(cardHit);
		const rect = args.droppableRects.get(cardHit.id);
		if (cardData?.type !== "card" || !rect) return [];
		const y = (pointer.y - rect.top) / rect.height;
		const card = cardData.card;
		if (drag?.type === "chat" || (y > EDGE_ZONE && y < 1 - EDGE_ZONE)) {
			target = { kind: "merge", card };
		} else if (y <= EDGE_ZONE) {
			target = { kind: "insert", column: card.column, beforeCardId: card.id };
		} else {
			target = {
				kind: "insert",
				column: card.column,
				beforeCardId: nextCardIdInColumn(args, card),
			};
		}
	} else {
		const below = cardsInColumn(args, columnData.name).find(
			({ rect }) => rect.top + rect.height / 2 > pointer.y,
		);
		target = {
			kind: "insert",
			column: columnData.name,
			beforeCardId: below?.card.id ?? null,
		};
	}
	const hit = cardHit ?? columnHit;
	return [{ id: hit.id, data: { ...hit.data, target } }];
};

/** The target boardCollision attached to the first collision, if any. */
export const targetOf = (
	collisions: readonly Collision[] | null | undefined,
): DropTarget | null =>
	(collisions?.[0]?.data as { target?: DropTarget } | undefined)?.target ??
	null;

/**
 * The board command a drop means, or null when that drag cannot land on
 * that target. Returned unapplied so the caller runs it against the full
 * model.
 */
export const dropCommand = (
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
