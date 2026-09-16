import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	addCommentLabels,
	buildCards,
	buildColumns,
	chunkByBytes,
	INBOX_COLUMN,
	parseComments,
	removeCommentLabels,
	setColumnLabel,
	setGroupLabel,
} from "./boardLabels";

const chat = (id: string, labels: Record<string, string> = {}): Chat => ({
	...MockChat,
	id,
	title: `Chat ${id}`,
	labels,
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

	it("orders columns: Inbox, stored order, then discovered", () => {
		const cards = buildCards([
			chat("a", { "board/column": "Zeta" }),
			chat("b", { "board/column": "Alpha" }),
		]);
		const names = buildColumns(cards, ["Alpha"], ["Empty"]).map((c) => c.name);
		expect(names).toEqual([INBOX_COLUMN, "Alpha", "Empty", "Zeta"]);
	});
});

describe("label setters", () => {
	it("omits the label for defaults so the map stays minimal", () => {
		expect(setColumnLabel({ "board/column": "X" }, INBOX_COLUMN)).toEqual({});
		expect(setGroupLabel({ "board/group": "p" }, "self", "self")).toEqual({});
	});
});
