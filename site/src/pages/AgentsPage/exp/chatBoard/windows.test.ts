import { afterEach, describe, expect, it, vi } from "vitest";
import { type ChatWindow, windowKey } from "./boardStorage";
import {
	changeWindow,
	closeWindow,
	dismissTop,
	draftCreated,
	draftWindow,
	dropPreview,
	raise,
	toFront,
	windowBeside,
	windowCentered,
} from "./windows";

const viewport = (width: number, height: number) => {
	vi.spyOn(window, "innerWidth", "get").mockReturnValue(width);
	vi.spyOn(window, "innerHeight", "get").mockReturnValue(height);
};

const rect = (left: number, top: number, width: number, height: number) =>
	new DOMRect(left, top, width, height);

const win = (chatId: string, pinned = true): ChatWindow => ({
	kind: "chat",
	chatId,
	x: 0,
	y: 0,
	width: 100,
	height: 100,
	pinned,
});

describe("window geometry", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("opens to the right of the anchor when there is room", () => {
		viewport(1400, 900);
		const win = windowBeside("c", rect(100, 200, 300, 40), true);
		expect(win).toMatchObject({
			kind: "chat",
			chatId: "c",
			x: 100 + 300 + 8,
			y: 200,
			width: 520,
			height: 640,
			pinned: true,
		});
	});

	it("flips to the left when the right side does not fit", () => {
		viewport(1400, 900);
		const win = windowBeside("c", rect(1000, 200, 300, 40), false);
		expect(win.x).toBe(1000 - 8 - 520);
		expect(win.pinned).toBe(false);
	});

	it("clamps into the viewport margin", () => {
		viewport(1400, 900);
		const win = windowBeside("c", rect(0, 850, 300, 40), true);
		expect(win.y).toBe(900 - 640 - 12);
		const left = windowBeside("c", rect(20, 0, 300, 40), true);
		// Neither side fits fully; the flip goes negative and is clamped.
		viewport(600, 900);
		const narrow = windowBeside("c", rect(400, 0, 150, 40), true);
		expect(narrow.x).toBe(12);
		expect(left.y).toBe(12);
	});

	it("shrinks to the viewport on small screens", () => {
		viewport(400, 300);
		const win = windowBeside("c", rect(0, 0, 50, 20), true);
		expect(win.width).toBe(400 - 24);
		expect(win.height).toBe(300 - 24);
		expect(win.x).toBe(12);
		expect(win.y).toBe(12);
	});

	it("centres a pinned window", () => {
		viewport(1400, 900);
		expect(windowCentered("c")).toEqual({
			kind: "chat",
			chatId: "c",
			x: (1400 - 520) / 2,
			y: (900 - 640) / 2,
			width: 520,
			height: 640,
			pinned: true,
		});
	});

	it("centres a draft window, pinned, with context off", () => {
		viewport(1400, 900);
		expect(draftWindow({ column: "Doing" })).toEqual({
			kind: "draft",
			target: { column: "Doing" },
			withContext: false,
			x: (1400 - 520) / 2,
			y: (900 - 640) / 2,
			width: 520,
			height: 640,
			pinned: true,
		});
	});
});

describe("window list", () => {
	it("brings a window to the front pinned, replacing its old entry", () => {
		const list = [win("a"), win("b"), win("p", false)];
		expect(toFront(list, win("a", false)).map(windowKey)).toEqual([
			"b",
			"p",
			"a",
		]);
		expect(toFront(list, win("a", false)).at(-1)?.pinned).toBe(true);
		expect(raise(list, "p").at(-1)).toEqual(win("p"));
		expect(raise(list, "nope")).toBe(list);
	});

	it("keeps pinning when geometry changes and drops the preview on escape", () => {
		const list = [win("a"), win("p", false)];
		const moved = changeWindow(list, { ...win("p"), x: 50 });
		expect(moved[1]).toEqual({ ...win("p", false), x: 50 });
		expect(dropPreview(list)).toEqual([win("a")]);
		expect(dismissTop(list)).toEqual([win("a")]);
		expect(dismissTop([win("a"), win("b")])).toEqual([win("a")]);
	});

	it("keeps one draft, keyed apart from chats, and toggles its option in place", () => {
		viewport(1400, 900);
		const draft = draftWindow({ column: "Doing" });
		const list = toFront([win("a"), draft], draftWindow({ cardId: "p" }));
		expect(list.map((w) => w.kind)).toEqual(["chat", "draft"]);
		expect(list[1]).toMatchObject({ target: { cardId: "p" } });
		const toggled = changeWindow(list, {
			...draftWindow({ cardId: "p" }),
			withContext: true,
		});
		expect(toggled[1]).toMatchObject({ kind: "draft", withContext: true });
		expect(closeWindow(toggled, "draft")).toEqual([win("a")]);
		expect(raise(toggled, "draft").at(-1)?.kind).toBe("draft");
	});

	it("hands the draft's frame to the chat it created, else opens the chat centred", () => {
		viewport(1400, 900);
		const draft = { ...draftWindow({ cardId: "p" }), x: 30, y: 40 };
		const list = [draft, win("a")];
		expect(draftCreated(list, { cardId: "p" }, "n")).toEqual([
			{ ...win("n"), x: 30, y: 40, width: 520, height: 640 },
			win("a"),
		]);
		expect(draftCreated(list, { column: "Doing" }, "n")).toEqual([
			draft,
			win("a"),
			windowCentered("n"),
		]);
		expect(draftCreated([win("a")], { cardId: "p" }, "n")).toEqual([
			win("a"),
			windowCentered("n"),
		]);
	});
});
