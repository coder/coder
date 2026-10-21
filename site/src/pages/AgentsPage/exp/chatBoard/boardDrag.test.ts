import { describe, expect, it } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import type { DragData, DropData } from "./BoardCard";
import type { BoardState } from "./boardApi";
import {
	type BoardDroppable,
	boardTarget,
	dragCanChangeLane,
	dropCommand,
} from "./boardDrag";
import {
	BOARD_LANE,
	buildCards,
	buildColumns,
	NO_PROJECT_LANE,
} from "./boardLabels";

const chat = (id: string, labels: Record<string, string> = {}): Chat => ({
	...MockChat,
	id,
	title: `Chat ${id}`,
	labels,
});

const droppable = (
	data: DropData,
	left: number,
	top: number,
	width: number,
	height: number,
): BoardDroppable => ({ data, rect: { left, top, width, height } });

// One column "Doing" at x 0..300 with two cards stacked: a at y 0..100, b at y 110..210.
const state: BoardState = (() => {
	const cards = buildCards([
		chat("a", { "board/column": "Doing", "board/pos": "300000" }),
		chat("b", { "board/column": "Doing", "board/pos": "100000" }),
		chat("c"),
	]);
	const storage = {
		columnOrder: [],
		emptyColumns: [],
		windows: [],
		effortFilter: null,
		collapsedLanes: [],
	};
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
	{ type: "column", name: "Doing", lane: BOARD_LANE },
	0,
	0,
	300,
	600,
);
const dropA = droppable(
	{ type: "card", card: cardA, lane: BOARD_LANE },
	0,
	0,
	300,
	100,
);
const dropB = droppable(
	{ type: "card", card: cardB, lane: BOARD_LANE },
	0,
	110,
	300,
	100,
);
const board = [column, dropA, dropB];
const dragC: DragData = { type: "card", card: cardC };

describe("boardTarget", () => {
	it("inserts before a card near its top, merges in the middle, inserts after near its bottom", () => {
		expect(boardTarget(dragC, board, { x: 10, y: 10 })).toEqual({
			kind: "insert",
			column: "Doing",
			lane: BOARD_LANE,
			beforeCardId: "a",
		});
		expect(boardTarget(dragC, board, { x: 10, y: 50 })).toEqual({
			kind: "merge",
			card: cardA,
			lane: BOARD_LANE,
		});
		expect(boardTarget(dragC, board, { x: 10, y: 95 })).toEqual({
			kind: "insert",
			column: "Doing",
			lane: BOARD_LANE,
			beforeCardId: "b",
		});
	});

	it("inserts at the end over empty column background, and a chat always joins", () => {
		expect(boardTarget(dragC, board, { x: 10, y: 400 })).toEqual({
			kind: "insert",
			column: "Doing",
			lane: BOARD_LANE,
			beforeCardId: null,
		});
		const dragChat: DragData = {
			type: "chat",
			chat: cardC.primary,
			card: cardC,
		};
		expect(boardTarget(dragChat, board, { x: 10, y: 10 })).toEqual({
			kind: "merge",
			card: cardA,
			lane: BOARD_LANE,
		});
	});

	it("ignores the card being dragged and lands a column before or after another column", () => {
		const dragA: DragData = { type: "card", card: cardA };
		// Over its own card there is no card hit; the column background puts it before the next card, its own slot.
		expect(boardTarget(dragA, board, { x: 10, y: 50 })).toEqual({
			kind: "insert",
			column: "Doing",
			lane: BOARD_LANE,
			beforeCardId: "b",
		});
		const dragColumn: DragData = { type: "column", name: "Inbox" };
		expect(boardTarget(dragColumn, board, { x: 10, y: 50 })).toEqual({
			kind: "column",
			name: "Doing",
			side: "before",
		});
		expect(boardTarget(dragColumn, board, { x: 250, y: 50 })).toEqual({
			kind: "column",
			name: "Doing",
			side: "after",
		});
		expect(boardTarget(dragColumn, board, { x: 500, y: 50 })).toBeNull();
	});
});

describe("dropCommand", () => {
	it("maps a drop to the board command and refuses impossible pairs", () => {
		const insert = dropCommand(dragC, {
			kind: "insert",
			column: "Doing",
			lane: BOARD_LANE,
			beforeCardId: "b",
		});
		expect(insert?.(state)?.writes.map((w) => w.chat.id)).toEqual(["c"]);
		// Without lanes a drop never changes a project.
		expect(insert?.(state)?.projectMoves).toBeUndefined();

		const merge = dropCommand(dragC, {
			kind: "merge",
			card: cardA,
			lane: BOARD_LANE,
		});
		expect(merge?.(state)?.undo).toBe('Merged into "Chat a"');

		expect(
			dropCommand(dragC, { kind: "column", name: "Doing", side: "after" }),
		).toBeNull();
		expect(
			dropCommand(
				{ type: "column", name: "Inbox" },
				{ kind: "merge", card: cardA, lane: BOARD_LANE },
			),
		).toBeNull();
	});
});

describe("lanes", () => {
	// Lane "p" at y 0..300 holds card a; the no-project lane at y 300..600
	// holds card b in the same column. Card c is in no project.
	const inProject = (
		id: string,
		projectId: string | undefined,
		status: Chat["status"] = "waiting",
	): Chat => ({
		...chat(id, { "board/column": "Doing" }),
		project_id: projectId,
		status,
	});
	const laneCards = buildCards([
		inProject("a", "p"),
		inProject("b", undefined),
		inProject("c", undefined),
	]);
	const laneState: BoardState = {
		...state,
		cards: laneCards,
		columns: buildColumns(laneCards, [], []),
	};
	const card = (id: string) => {
		const found = laneCards.find((c) => c.id === id);
		if (!found) throw new Error(`card ${id} missing`);
		return found;
	};
	const lanes = [
		droppable({ type: "column", name: "Doing", lane: "p" }, 0, 0, 300, 300),
		droppable({ type: "card", card: card("a"), lane: "p" }, 0, 0, 300, 100),
		droppable(
			{ type: "column", name: "Doing", lane: NO_PROJECT_LANE },
			0,
			300,
			300,
			300,
		),
		droppable(
			{ type: "card", card: card("b"), lane: NO_PROJECT_LANE },
			0,
			300,
			300,
			100,
		),
	];
	const dragC: DragData = { type: "card", card: card("c") };

	it("places a drop on column background among that lane's cards only", () => {
		expect(boardTarget(dragC, lanes, { x: 10, y: 200 })).toEqual({
			kind: "insert",
			column: "Doing",
			lane: "p",
			beforeCardId: null,
		});
	});

	it("moves a card and its assistant into the lane's project, and only the chats when merging", () => {
		const assistant = inProject("x", undefined);
		const insert = dropCommand(
			dragC,
			{ kind: "insert", column: "Doing", lane: "p", beforeCardId: null },
			() => assistant,
		);
		expect(
			insert?.(laneState)?.projectMoves?.map((m) => [m.chat.id, m.projectId]),
		).toEqual([
			["c", "p"],
			["x", "p"],
		]);
		const merge = dropCommand(
			dragC,
			{ kind: "merge", card: card("a"), lane: "p" },
			() => assistant,
		);
		expect(
			merge?.(laneState)?.projectMoves?.map((m) => [m.chat.id, m.projectId]),
		).toEqual([["c", "p"]]);
		// A titled source card is kept by the merge, so its assistant follows.
		const titled = buildCards([
			{ ...inProject("t", undefined), labels: { "board/title": "Epic" } },
		])[0];
		if (!titled) throw new Error("card t missing");
		const keepSource = dropCommand(
			{ type: "card", card: titled },
			{ kind: "merge", card: card("a"), lane: "p" },
			() => assistant,
		);
		expect(
			keepSource?.(laneState)?.projectMoves?.map((m) => m.chat.id),
		).toEqual(["t", "x"]);
	});

	it("keeps a drag with a working chat in its own lane", () => {
		const busy: DragData = {
			type: "card",
			card: buildCards([inProject("c", undefined, "running")])[0] ?? card("c"),
		};
		const canChange = dragCanChangeLane(
			busy,
			MockChat.owner_id,
			() => undefined,
		);
		expect(canChange).toBe(false);
		// The card's assistant moves with it, so a working assistant blocks too.
		expect(
			dragCanChangeLane(dragC, MockChat.owner_id, () =>
				inProject("x", undefined, "running"),
			),
		).toBe(false);
		expect(boardTarget(busy, lanes, { x: 10, y: 200 }, canChange)).toBeNull();
		expect(boardTarget(busy, lanes, { x: 10, y: 500 }, canChange)).toEqual({
			kind: "insert",
			column: "Doing",
			lane: NO_PROJECT_LANE,
			beforeCardId: null,
		});
	});
});
