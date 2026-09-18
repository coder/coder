import { afterEach, describe, expect, it, vi } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	addColumn,
	addNote,
	type BoardState,
	cardContext,
	deleteColumn,
	detachChat,
	effortsOf,
	joinCard,
	mergeCards,
	moveCard,
	moveColumn,
	moveNote,
	newChatLabels,
	type Plan,
	removeFromGroup,
	renameCard,
	renameChat,
	renameColumn,
	renameEffort,
	setCardColor,
	setCardEfforts,
} from "./boardApi";
import { addCommentLabels, buildCards, buildColumns } from "./boardLabels";
import type { BoardStorage } from "./boardStorage";

const chat = (id: string, labels: Record<string, string> = {}): Chat => ({
	...MockChat,
	id,
	title: `Chat ${id}`,
	labels,
});

const stateOf = (
	chats: readonly Chat[],
	storage: Partial<BoardStorage> = {},
): BoardState => {
	const cards = buildCards(chats);
	const full: BoardStorage = {
		columnOrder: [],
		emptyColumns: [],
		windows: [],
		effortFilter: null,
		...storage,
	};
	return {
		cards,
		columns: buildColumns(cards, full.columnOrder, full.emptyColumns),
		storage: full,
	};
};

/** Label maps a plan would write, keyed by chat id. */
const written = (plan: Plan | null) =>
	Object.fromEntries((plan?.writes ?? []).map((w) => [w.chat.id, w.labels]));

describe("boardApi", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("moveCard sets the column on every member and places the primary between its neighbours", () => {
		const state = stateOf([
			chat("a", { "board/column": "Doing", "board/pos": "300000" }),
			chat("b", { "board/column": "Doing", "board/pos": "100000" }),
			chat("p"),
			chat("m", { "board/group": "p", "other/x": "1" }),
		]);

		expect(written(moveCard(state, "p", "Doing", "b"))).toEqual({
			p: { "board/column": "Doing", "board/pos": "200000" },
			m: { "board/group": "p", "board/column": "Doing", "other/x": "1" },
		});
		expect(written(moveCard(state, "p", "Doing", null)).p).toEqual({
			"board/column": "Doing",
			"board/pos": "40000",
		});
	});

	it("moveCard into its own slot or with an unknown id is a no-op", () => {
		const state = stateOf([
			chat("a", { "board/column": "Doing", "board/pos": "300000" }),
			chat("b", { "board/column": "Doing", "board/pos": "100000" }),
		]);

		expect(moveCard(state, "a", "Doing", "a")).toBeNull();
		expect(moveCard(state, "a", "Doing", "b")).toBeNull();
		expect(moveCard(state, "b", "Doing", null)).toBeNull();
		expect(moveCard(state, "nope", "Doing", null)).toBeNull();
	});

	it("mergeCards keeps a group dropped onto a single chat and moves it into the target slot", () => {
		const state = stateOf([
			chat("t", {
				"board/pos": "200",
				"board/column": "Done",
				...addCommentLabels({}, "kept", 1),
			}),
			chat("s", {
				"board/pos": "100",
				"board/title": "Source",
				"board/color": "sky",
				...addCommentLabels({}, "moved", 2),
			}),
			chat("sm", { "board/group": "s" }),
		]);

		const plan = mergeCards(state, "s", "t");

		expect(plan?.undo).toBe('Merged into "Source"');
		expect(written(plan)).toEqual({
			s: {
				"board/pos": "200",
				"board/column": "Done",
				"board/title": "Source",
				"board/color": "sky",
				"board/comment.0.timestamp": "2",
				"board/comment.0.0": "moved",
				"board/comment.1.timestamp": "1",
				"board/comment.1.0": "kept",
			},
			t: { "board/group": "s", "board/column": "Done" },
			sm: { "board/group": "s", "board/column": "Done" },
		});
	});

	it("mergeCards of two titled groups keeps the target and records the lost title as a note", () => {
		vi.spyOn(Date, "now").mockReturnValue(5);
		const state = stateOf([
			chat("t", { "board/pos": "200", "board/title": "Target" }),
			chat("tm", { "board/group": "t" }),
			chat("s", { "board/pos": "100", "board/title": "Source" }),
			chat("sm", { "board/group": "s" }),
		]);

		expect(written(mergeCards(state, "s", "t"))).toEqual({
			t: {
				"board/pos": "200",
				"board/title": "Target",
				"board/comment.0.timestamp": "5",
				"board/comment.0.0": "Merged card: Source",
			},
			s: { "board/group": "t" },
			sm: { "board/group": "t" },
		});
		expect(mergeCards(state, "s", "s")).toBeNull();
		expect(mergeCards(state, "s", "nope")).toBeNull();
	});

	it("joinCard moves one chat out of its card into the target", () => {
		const state = stateOf([
			chat("p", { "board/column": "Doing" }),
			chat("m", { "board/group": "p", "board/column": "Doing" }),
			chat("t", { "board/column": "Done" }),
		]);

		const plan = joinCard(state, "m", "t");

		expect(plan?.undo).toBe('Added "Chat m" to "Chat t"');
		expect(written(plan)).toEqual({
			m: { "board/group": "t", "board/column": "Done" },
		});
		expect(joinCard(state, "m", "p")).toBeNull();
	});

	it("detachChat makes the chat its own card at the slot", () => {
		const state = stateOf([
			chat("p"),
			chat("m", { "board/group": "p" }),
			chat("a", { "board/column": "Later", "board/pos": "300000" }),
			chat("b", { "board/column": "Later", "board/pos": "100000" }),
		]);

		const plan = detachChat(state, "m", "Later", "b");

		expect(plan?.undo).toBe('Removed "Chat m" from "Chat p"');
		expect(written(plan)).toEqual({
			m: { "board/column": "Later", "board/pos": "200000" },
		});
	});

	it("detachChat of the primary hands the card to the next member without renaming it", () => {
		const state = stateOf([
			chat("p", {
				"board/color": "sky",
				"board/pos": "400000",
				"board/column": "Doing",
				...addCommentLabels({}, "note", 1),
			}),
			chat("a", { "board/group": "p", "board/column": "Doing" }),
			chat("b", { "board/group": "p", "board/column": "Doing" }),
			chat("q", { "board/column": "Doing", "board/pos": "300000" }),
		]);

		expect(written(detachChat(state, "p", "Doing", null))).toEqual({
			a: {
				"board/column": "Doing",
				"board/color": "sky",
				"board/pos": "400000",
				"board/comment.0.timestamp": "1",
				"board/comment.0.0": "note",
				"board/title": "Chat p",
			},
			b: { "board/group": "a", "board/column": "Doing" },
			p: { "board/column": "Doing", "board/pos": "240000" },
		});
	});

	it("removeFromGroup lands the chat right under its card", () => {
		const state = stateOf([
			chat("p", { "board/column": "Doing", "board/pos": "500" }),
			chat("m", { "board/group": "p", "board/column": "Doing" }),
		]);

		expect(written(removeFromGroup(state, "m"))).toEqual({
			m: { "board/column": "Doing", "board/pos": "499" },
		});
		expect(removeFromGroup(state, "nope")).toBeNull();
	});

	it("moveNote reorders within a card and moves across cards, renumbering both", () => {
		const state = stateOf([
			chat("a", {
				"board/pos": "200",
				...addCommentLabels(addCommentLabels({}, "one", 1), "two", 2),
			}),
			chat("b", { "board/pos": "100", ...addCommentLabels({}, "other", 3) }),
		]);

		const within = moveNote(state, "a", 1, "a", { index: 0, side: "before" });
		expect(within?.undo).toBe("Moved note");
		expect(written(within)).toEqual({
			a: {
				"board/pos": "200",
				"board/comment.0.timestamp": "2",
				"board/comment.0.0": "two",
				"board/comment.1.timestamp": "1",
				"board/comment.1.0": "one",
			},
		});

		const across = moveNote(state, "a", 0, "b", null);
		expect(across?.undo).toBe('Moved note to "Chat b"');
		expect(written(across)).toEqual({
			b: {
				"board/pos": "100",
				"board/comment.0.timestamp": "3",
				"board/comment.0.0": "other",
				"board/comment.1.timestamp": "1",
				"board/comment.1.0": "one",
			},
			a: {
				"board/pos": "200",
				"board/comment.0.timestamp": "2",
				"board/comment.0.0": "two",
			},
		});
		expect(moveNote(state, "a", 9, "b", null)).toBeNull();
	});

	it("addNote appends after the existing notes", () => {
		vi.spyOn(Date, "now").mockReturnValue(5000);
		const state = stateOf([chat("p", addCommentLabels({}, "first", 1))]);

		expect(written(addNote(state, "p", "second")).p).toEqual({
			"board/comment.0.timestamp": "1",
			"board/comment.0.0": "first",
			"board/comment.1.timestamp": "5000",
			"board/comment.1.0": "second",
		});
	});

	it("renameCard renames the chat of a single card and labels a group", () => {
		const state = stateOf([
			chat("s"),
			chat("g", { "board/title": "Group" }),
			chat("gm", { "board/group": "g" }),
		]);

		const single = renameCard(state, "s", "Solo");
		expect(single?.writes).toEqual([]);
		expect(single?.titles).toEqual([
			{ chat: state.cards[0]?.primary, title: "Solo" },
		]);

		expect(written(renameCard(state, "g", "Bigger"))).toEqual({
			g: { "board/title": "Bigger" },
		});
		expect(renameCard(state, "nope", "x")).toBeNull();
	});

	it("renameChat and setCardColor touch one chat", () => {
		const state = stateOf([chat("p", { "board/color": "red" })]);

		expect(renameChat(state, "p", "New")?.titles).toEqual([
			{ chat: state.cards[0]?.primary, title: "New" },
		]);
		expect(written(setCardColor(state, "p", "sky"))).toEqual({
			p: { "board/color": "sky" },
		});
		expect(written(setCardColor(state, "p", undefined))).toEqual({ p: {} });
	});

	it("renameColumn relabels every card in the column from the full model and renames storage entries", () => {
		const state = stateOf(
			[
				chat("a", { "board/column": "Doing" }),
				chat("am", { "board/group": "a", "board/column": "Doing" }),
				chat("b", { "board/column": "Doing" }),
				chat("c", { "board/column": "Done" }),
			],
			{ columnOrder: ["Inbox", "Doing", "Done"], emptyColumns: ["Doing"] },
		);

		const plan = renameColumn(state, "Doing", "Active");

		expect(written(plan)).toEqual({
			a: { "board/column": "Active" },
			am: { "board/group": "a", "board/column": "Active" },
			b: { "board/column": "Active" },
		});
		expect(plan?.storage).toEqual({
			columnOrder: ["Inbox", "Active", "Done"],
			emptyColumns: ["Active"],
		});
	});

	it("renameColumn refuses Inbox, duplicates and unknown columns", () => {
		const state = stateOf([
			chat("a", { "board/column": "Doing" }),
			chat("c", { "board/column": "Done" }),
		]);

		expect(renameColumn(state, "Inbox", "Todo")).toBeNull();
		expect(renameColumn(state, "Doing", "Done")).toBeNull();
		expect(renameColumn(state, "Doing", "Inbox")).toBeNull();
		expect(renameColumn(state, "Nope", "Todo")).toBeNull();
	});

	it("deleteColumn sends every card in the column to Inbox and forgets the column", () => {
		const state = stateOf(
			[
				chat("a", { "board/column": "Doing" }),
				chat("b", { "board/column": "Doing" }),
			],
			{ columnOrder: ["Inbox", "Doing"], emptyColumns: ["Doing"] },
		);

		const plan = deleteColumn(state, "Doing");

		expect(written(plan)).toEqual({ a: {}, b: {} });
		expect(plan?.storage).toEqual({ columnOrder: ["Inbox"], emptyColumns: [] });
		expect(deleteColumn(state, "Inbox")).toBeNull();
		expect(deleteColumn(state, "Nope")).toBeNull();
	});

	it("addColumn appends to storage and refuses duplicates", () => {
		const state = stateOf([chat("a", { "board/column": "Doing" })]);

		expect(addColumn(state, "Review")).toEqual({
			writes: [],
			storage: {
				emptyColumns: ["Review"],
				columnOrder: ["Inbox", "Doing", "Review"],
			},
		});
		expect(addColumn(state, "Doing")).toBeNull();
	});

	it("moveColumn reorders storage around the target", () => {
		const state = stateOf([], {
			columnOrder: ["Inbox", "A", "B", "C"],
			emptyColumns: ["A", "B", "C"],
		});

		expect(
			moveColumn(state, "C", { name: "A", side: "before" })?.storage,
		).toEqual({
			columnOrder: ["Inbox", "C", "A", "B"],
		});
		expect(
			moveColumn(state, "A", { name: "C", side: "after" })?.storage,
		).toEqual({
			columnOrder: ["Inbox", "B", "C", "A"],
		});
		expect(moveColumn(state, "A", { name: "A", side: "after" })).toBeNull();
	});

	it("newChatLabels for a column places the chat above the first card", () => {
		const state = stateOf([
			chat("a", { "board/column": "Doing", "board/pos": "300000" }),
			chat("b", { "board/column": "Doing", "board/pos": "100000" }),
			chat("i", { "board/pos": "500000" }),
		]);

		expect(newChatLabels(state, { column: "Doing" })).toEqual({
			"board/column": "Doing",
			"board/pos": "360000",
		});
		// Absent column label means Inbox.
		expect(newChatLabels(state, { column: "Inbox" })).toEqual({
			"board/pos": "560000",
		});
	});

	it("newChatLabels for a card joins the group without a position", () => {
		const state = stateOf([
			chat("p", { "board/column": "Doing", "board/pos": "300000" }),
			chat("i", { "board/pos": "500000" }),
		]);

		expect(newChatLabels(state, { cardId: "p" })).toEqual({
			"board/group": "p",
			"board/column": "Doing",
		});
		expect(newChatLabels(state, { cardId: "i" })).toEqual({
			"board/group": "i",
		});
		expect(newChatLabels(state, { cardId: "nope" })).toBeNull();
	});

	it("cardContext lists title, notes in order and every chat", () => {
		const [card] = buildCards([
			{
				...chat("p", {
					"board/title": "Epic",
					...addCommentLabels(addCommentLabels({}, "first", 1), "second", 2),
				}),
				status: "running",
				last_turn_summary: "Fixed the build",
			},
			{
				...chat("m", { "board/group": "p" }),
				status: "waiting",
				last_turn_summary: null,
			},
		]);
		if (!card) throw new Error("card missing");

		expect(cardContext(card)).toBe(
			[
				"Card context",
				"Title: Epic",
				"Notes:",
				"- first",
				"- second",
				"Chats:",
				"- Chat p (p) status: running; last turn: Fixed the build",
				"- Chat m (m) status: waiting; last turn: none",
			].join("\n"),
		);
		const [bare] = buildCards([chat("s")]);
		if (!bare) throw new Error("card missing");
		expect(cardContext(bare)).toContain("Notes: none\n");
	});

	it("setCardEfforts writes the renumbered list on the primary", () => {
		const state = stateOf([
			chat("p", { "board/effort.3": "old", "board/title": "T" }),
		]);

		expect(written(setCardEfforts(state, "p", ["Q3", "This week"]))).toEqual({
			p: {
				"board/title": "T",
				"board/effort.0": "Q3",
				"board/effort.1": "This week",
			},
		});
		expect(setCardEfforts(state, "nope", [])).toBeNull();
	});

	it("renameEffort relabels every card carrying it, dedupes into an existing name and follows the filter", () => {
		const state = stateOf(
			[
				chat("a", { "board/effort.0": "Q3", "board/title": "A" }),
				chat("b", { "board/effort.0": "Q3", "board/effort.1": "Launch" }),
				chat("c", { "board/effort.0": "Launch" }),
				chat("d"),
			],
			{ effortFilter: "Q3" },
		);

		const plan = renameEffort(state, "Q3", " Q4 ");
		expect(written(plan)).toEqual({
			a: { "board/title": "A", "board/effort.0": "Q4" },
			b: { "board/effort.0": "Q4", "board/effort.1": "Launch" },
		});
		expect(plan?.storage).toEqual({ effortFilter: "Q4" });

		// Renaming onto an existing effort merges: b keeps one Launch.
		expect(written(renameEffort(state, "Q3", "Launch"))).toEqual({
			a: { "board/title": "A", "board/effort.0": "Launch" },
			b: { "board/effort.0": "Launch" },
		});

		// The filter only follows when it pointed at the renamed effort.
		expect(renameEffort(state, "Launch", "Ship")?.storage).toBeUndefined();
	});

	it("renameEffort refuses blank, unchanged and unknown names", () => {
		const state = stateOf([chat("a", { "board/effort.0": "Q3" })]);

		expect(renameEffort(state, "Q3", "  ")).toBeNull();
		expect(renameEffort(state, "Q3", "Q3")).toBeNull();
		expect(renameEffort(state, "Q3", " Q3 ")).toBeNull();
		expect(renameEffort(state, "Nope", "Q4")).toBeNull();
	});

	it("mergeCards unions efforts, kept primary first, without repeats", () => {
		const state = stateOf([
			chat("t", { "board/pos": "200", "board/effort.0": "Q3" }),
			chat("s", {
				"board/pos": "100",
				"board/effort.0": "This week",
				"board/effort.1": "Q3",
			}),
		]);

		expect(written(mergeCards(state, "s", "t")).t).toMatchObject({
			"board/effort.0": "Q3",
			"board/effort.1": "This week",
		});
		expect(written(mergeCards(state, "s", "t")).s).not.toHaveProperty(
			"board/effort.0",
		);
	});

	it("a primary handing off its card passes the efforts on", () => {
		const state = stateOf([
			chat("p", { "board/effort.0": "Q3", "board/pos": "400000" }),
			chat("a", { "board/group": "p" }),
		]);

		const plan = written(detachChat(state, "p", "Inbox", null));
		expect(plan.a).toMatchObject({ "board/effort.0": "Q3" });
		expect(plan.p).not.toHaveProperty("board/effort.0");
	});

	it("effortsOf counts cards per effort in order of first appearance", () => {
		const cards = buildCards([
			chat("a", { "board/pos": "300", "board/effort.0": "Q3" }),
			chat("b", {
				"board/pos": "200",
				"board/effort.0": "This week",
				"board/effort.1": "Q3",
			}),
			chat("c", { "board/pos": "100" }),
		]);

		expect(effortsOf(cards)).toEqual([
			{ name: "Q3", count: 2 },
			{ name: "This week", count: 1 },
		]);
		expect(effortsOf([])).toEqual([]);
	});
});
