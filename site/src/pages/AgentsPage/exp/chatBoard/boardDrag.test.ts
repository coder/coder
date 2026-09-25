import type {
	ClientRect,
	CollisionDetection,
	DroppableContainer,
} from "@dnd-kit/core";
import { describe, expect, it } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import type { DragData, DropData } from "./BoardCard";
import type { BoardState } from "./boardApi";
import { boardCollision, dropCommand, targetOf } from "./boardDrag";
import { buildCards, buildColumns } from "./boardLabels";

const chat = (id: string, labels: Record<string, string> = {}): Chat => ({
	...MockChat,
	id,
	title: `Chat ${id}`,
	labels,
});

const rect = (
	left: number,
	top: number,
	width: number,
	height: number,
): ClientRect => ({
	left,
	top,
	width,
	height,
	right: left + width,
	bottom: top + height,
});

const droppable = (
	id: string,
	data: DropData,
	bounds: ClientRect,
): DroppableContainer => ({
	id,
	key: id,
	data: { current: data },
	disabled: false,
	node: { current: null },
	rect: { current: bounds },
});

type Args = Parameters<CollisionDetection>[0];

const argsFor = (
	drag: DragData,
	containers: readonly DroppableContainer[],
	pointer: { x: number; y: number },
): Args => ({
	active: {
		id: "active",
		data: { current: drag },
		rect: { current: { initial: null, translated: null } },
	},
	collisionRect: rect(pointer.x, pointer.y, 1, 1),
	droppableRects: new Map(
		containers.map((c) => [c.id, c.rect.current ?? rect(0, 0, 0, 0)]),
	),
	droppableContainers: [...containers],
	pointerCoordinates: pointer,
});

// One column "Doing" at x 0..300 with two cards stacked: a at y 0..100, b at y 110..210.
const state: BoardState = (() => {
	const cards = buildCards([
		chat("a", { "board/column": "Doing", "board/pos": "300000" }),
		chat("b", { "board/column": "Doing", "board/pos": "100000" }),
		chat("c"),
	]);
	const storage = { columnOrder: [], emptyColumns: [], windows: [] };
	return { cards, columns: buildColumns(cards, [], []), storage };
})();
const cardOf = (id: string) => {
	const card = state.cards.find((c) => c.id === id);
	if (!card) throw new Error(`card ${id} missing`);
	return card;
};
const cardA = cardOf("a");
const cardB = cardOf("b");
const cardC = cardOf("c");
const column = droppable(
	"column:Doing",
	{ type: "column", name: "Doing" },
	rect(0, 0, 300, 600),
);
const dropA = droppable(
	"drop-card:a",
	{ type: "card", card: cardA },
	rect(0, 0, 300, 100),
);
const dropB = droppable(
	"drop-card:b",
	{ type: "card", card: cardB },
	rect(0, 110, 300, 100),
);
const board = [column, dropA, dropB];
const dragC: DragData = { type: "card", card: cardC };

describe("boardCollision", () => {
	it("inserts before a card near its top, merges in the middle, inserts after near its bottom", () => {
		expect(
			targetOf(boardCollision(argsFor(dragC, board, { x: 10, y: 10 }))),
		).toEqual({
			kind: "insert",
			column: "Doing",
			beforeCardId: "a",
		});
		expect(
			targetOf(boardCollision(argsFor(dragC, board, { x: 10, y: 50 }))),
		).toEqual({
			kind: "merge",
			card: cardA,
		});
		expect(
			targetOf(boardCollision(argsFor(dragC, board, { x: 10, y: 95 }))),
		).toEqual({
			kind: "insert",
			column: "Doing",
			beforeCardId: "b",
		});
	});

	it("inserts at the end over empty column background, and a chat always joins", () => {
		expect(
			targetOf(boardCollision(argsFor(dragC, board, { x: 10, y: 400 }))),
		).toEqual({
			kind: "insert",
			column: "Doing",
			beforeCardId: null,
		});
		const dragChat: DragData = {
			type: "chat",
			chat: cardC.primary,
			card: cardC,
		};
		expect(
			targetOf(boardCollision(argsFor(dragChat, board, { x: 10, y: 10 }))),
		).toEqual({
			kind: "merge",
			card: cardA,
		});
	});

	it("ignores the card being dragged and lands a column before or after another column", () => {
		const dragA: DragData = { type: "card", card: cardA };
		// Over its own card there is no card hit; the column background puts it before the next card, its own slot.
		expect(
			targetOf(boardCollision(argsFor(dragA, board, { x: 10, y: 50 }))),
		).toEqual({
			kind: "insert",
			column: "Doing",
			beforeCardId: "b",
		});
		const dragColumn: DragData = { type: "column", name: "Inbox" };
		expect(
			targetOf(boardCollision(argsFor(dragColumn, board, { x: 10, y: 50 }))),
		).toEqual({
			kind: "column",
			name: "Doing",
			side: "before",
		});
		expect(
			targetOf(boardCollision(argsFor(dragColumn, board, { x: 250, y: 50 }))),
		).toEqual({
			kind: "column",
			name: "Doing",
			side: "after",
		});
		expect(
			boardCollision(argsFor(dragColumn, board, { x: 500, y: 50 })),
		).toEqual([]);
	});
});

describe("dropCommand", () => {
	it("maps a drop to the board command and refuses impossible pairs", () => {
		const insert = dropCommand(dragC, {
			kind: "insert",
			column: "Doing",
			beforeCardId: "b",
		});
		expect(insert?.(state)?.writes.map((w) => w.chat.id)).toEqual(["c"]);

		const merge = dropCommand(dragC, { kind: "merge", card: cardA });
		expect(merge?.(state)?.undo).toBe('Merged into "Chat a"');

		expect(
			dropCommand(dragC, { kind: "column", name: "Doing", side: "after" }),
		).toBeNull();
		expect(
			dropCommand(
				{ type: "column", name: "Inbox" },
				{ kind: "merge", card: cardA },
			),
		).toBeNull();
	});
});
