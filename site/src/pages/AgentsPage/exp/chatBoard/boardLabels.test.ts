import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	addCommentLabels,
	buildCards,
	buildColumns,
	chunkByBytes,
	INBOX_COLUMN,
	keyBetween,
	parseComments,
	placementKey,
	removeCommentLabels,
	setColumnLabel,
	setGroupLabel,
	stripCardLabels,
	updateCommentLabels,
} from "./boardLabels";

const chat = (id: string, labels: Record<string, string> = {}): Chat => ({
	...MockChat,
	id,
	title: `Chat ${id}`,
	labels,
	created_at: `2026-01-0${id.length}T00:00:00Z`,
	updated_at: `2026-01-0${id.length}T00:00:00Z`,
});

describe("comments", () => {
	it("round-trips a comment through labels in order", () => {
		let labels = addCommentLabels({}, "first", 1000);
		labels = addCommentLabels(labels, "second", 2000);
		expect(parseComments(labels)).toEqual([
			{ index: 0, timestamp: 1000, text: "first" },
			{ index: 1, timestamp: 2000, text: "second" },
		]);
	});

	it("keeps later indices after a deletion", () => {
		let labels = addCommentLabels({}, "a", 1);
		labels = addCommentLabels(labels, "b", 2);
		labels = removeCommentLabels(labels, 0);
		labels = addCommentLabels(labels, "c", 3);
		expect(parseComments(labels).map((c) => c.text)).toEqual(["b", "c"]);
	});

	it("chunks on byte length without splitting characters", () => {
		const text = "ä".repeat(300);
		const chunks = chunkByBytes(text, 256);
		expect(chunks.join("")).toBe(text);
		for (const chunk of chunks) {
			expect(new TextEncoder().encode(chunk).length).toBeLessThanOrEqual(256);
		}
		expect(parseComments(addCommentLabels({}, text, 1))[0]?.text).toBe(text);
	});

	it("round-trips four-byte characters across chunk boundaries", () => {
		const text = "a".repeat(255) + "😀".repeat(70);
		const chunks = chunkByBytes(text, 256);
		expect(chunks.length).toBeGreaterThan(1);
		expect(chunks.join("")).toBe(text);
		for (const chunk of chunks) {
			expect(new TextEncoder().encode(chunk).length).toBeLessThanOrEqual(256);
			expect(chunk.includes("\uFFFD")).toBe(false);
		}
		expect(parseComments(addCommentLabels({}, text, 1))[0]?.text).toBe(text);
	});

	it("edits a comment in place, keeping index and timestamp", () => {
		let labels = addCommentLabels({}, "a", 1);
		labels = addCommentLabels(labels, "b".repeat(600), 2);
		labels = updateCommentLabels(labels, 1, "short");
		expect(parseComments(labels)).toEqual([
			{ index: 0, timestamp: 1, text: "a" },
			{ index: 1, timestamp: 2, text: "short" },
		]);
		expect(
			Object.keys(labels).filter((k) => k.startsWith("board/comment.1.")),
		).toEqual(["board/comment.1.timestamp", "board/comment.1.0"]);
	});
});

describe("placement", () => {
	it("prefers a valid board/pos over creation time", () => {
		const created = new Date("2026-01-01T00:00:00Z").getTime();
		expect(placementKey(chat("a"))).toBe(created);
		expect(placementKey(chat("a", { "board/pos": "12345" }))).toBe(12345);
		expect(placementKey(chat("a", { "board/pos": "nope" }))).toBe(created);
		expect(placementKey(chat("a", { "board/pos": "0" }))).toBe(created);
	});

	it("keys between neighbours and past the column edges", () => {
		expect(keyBetween(100, 50)).toBe(75);
		expect(keyBetween(100, undefined)).toBeLessThan(100);
		expect(keyBetween(undefined, 50)).toBeGreaterThan(50);
		const now = Date.now();
		expect(keyBetween(undefined, undefined)).toBeGreaterThanOrEqual(now);
	});
});

describe("stripCardLabels", () => {
	it("removes card data but keeps the column and foreign labels", () => {
		const labels = {
			"board/title": "t",
			"board/group": "p",
			"board/color": "sky",
			"board/pos": "1",
			"board/column": "Doing",
			...addCommentLabels({}, "note", 1),
			"other/tool": "keep",
		};
		expect(stripCardLabels(labels)).toEqual({
			"board/column": "Doing",
			"other/tool": "keep",
		});
	});
});

describe("cards", () => {
	it("groups members under their primary and falls back to Inbox", () => {
		const primary = chat("p", { "board/column": "Done" });
		const member = chat("m", { "board/group": "p", "board/column": "Done" });
		const loner = chat("l");
		const cards = buildCards([member, loner, primary]);
		const byId = new Map(cards.map((c) => [c.id, c]));
		expect(byId.get("p")?.members.map((c) => c.id)).toEqual(["p", "m"]);
		expect(byId.get("p")?.column).toBe("Done");
		expect(byId.get("l")?.column).toBe(INBOX_COLUMN);
		expect(byId.has("m")).toBe(false);
	});

	it("treats a member whose primary is not loaded as its own card", () => {
		const orphan = chat("o", { "board/group": "missing" });
		expect(buildCards([orphan]).map((c) => c.id)).toEqual(["o"]);
	});

	it("skips assistant chats and orders cards by placement, newest first", () => {
		const cards = buildCards([
			chat("old", { "board/pos": "100" }),
			chat("new", { "board/pos": "200" }),
			chat("helper", { "board/assistant": "old" }),
		]);
		expect(cards.map((c) => c.id)).toEqual(["new", "old"]);
	});

	it("reads card title, color and comments from the primary", () => {
		const [card] = buildCards([
			chat("p", {
				"board/title": "Epic",
				"board/color": "sky",
				...addCommentLabels({}, "note", 5),
			}),
		]);
		expect(card?.title).toBe("Epic");
		expect(card?.color).toBe("sky");
		expect(card?.comments).toEqual([{ index: 0, timestamp: 5, text: "note" }]);
		expect(
			buildCards([chat("q", { "board/color": "plaid" })])[0]?.color,
		).toBeUndefined();
	});

	it("orders columns: Inbox, stored order, then discovered", () => {
		const cards = buildCards([
			chat("a", { "board/column": "Zeta" }),
			chat("b", { "board/column": "Alpha" }),
		]);
		const names = buildColumns(cards, ["Alpha"], ["Empty"]).map((c) => c.name);
		expect(names).toEqual([INBOX_COLUMN, "Alpha", "Empty", "Zeta"]);
	});

	it("lets a stored order that includes Inbox place it anywhere", () => {
		const cards = buildCards([chat("a", { "board/column": "Done" })]);
		const columns = buildColumns(cards, ["Done", INBOX_COLUMN], []);
		expect(columns.map((c) => c.name)).toEqual(["Done", INBOX_COLUMN]);
		expect(columns[0]?.cards.map((c) => c.id)).toEqual(["a"]);
		expect(columns[1]?.cards).toEqual([]);
	});
});

describe("label setters", () => {
	it("omits the label for defaults so the map stays minimal", () => {
		expect(setColumnLabel({ "board/column": "X" }, INBOX_COLUMN)).toEqual({});
		expect(setGroupLabel({ "board/group": "p" }, "self", "self")).toEqual({});
	});
});
