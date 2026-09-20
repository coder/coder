import { describe, expect, it } from "vitest";
import { type TreeKeyboardRow, treeKeyboardReducer } from "./treeKeyboard";

// root
//   alpha (expanded)
//     alpha-child
//   beta (collapsed, has children)
//   gamma (leaf)
const rows: TreeKeyboardRow[] = [
	{ id: "root", level: 1, hasChildren: true, isExpanded: true, label: "Root" },
	{
		id: "alpha",
		level: 2,
		parentId: "root",
		hasChildren: true,
		isExpanded: true,
		label: "Alpha",
	},
	{
		id: "alpha-child",
		level: 3,
		parentId: "alpha",
		hasChildren: false,
		isExpanded: false,
		label: "Nested",
	},
	{
		id: "beta",
		level: 2,
		parentId: "root",
		hasChildren: true,
		isExpanded: false,
		label: "Beta",
	},
	{
		id: "gamma",
		level: 2,
		parentId: "root",
		hasChildren: false,
		isExpanded: false,
		label: "Gamma",
	},
];

const press = (key: string, focusedId?: string, buffer = "") =>
	treeKeyboardReducer(rows, focusedId, key, buffer);

describe(treeKeyboardReducer.name, () => {
	it("moves down and up through visible rows and stops at the ends", () => {
		expect(press("ArrowDown", "alpha").focusId).toBe("alpha-child");
		expect(press("ArrowUp", "alpha-child").focusId).toBe("alpha");
		expect(press("ArrowDown", "gamma").focusId).toBe("gamma");
		expect(press("ArrowUp", "root").focusId).toBe("root");
	});

	it("focuses the first row when nothing is focused yet", () => {
		expect(press("ArrowDown").focusId).toBe("root");
		expect(press("ArrowUp").focusId).toBe("root");
	});

	it("jumps to the first and last rows with Home and End", () => {
		expect(press("Home", "beta").focusId).toBe("root");
		expect(press("End", "root").focusId).toBe("gamma");
	});

	it("expands a collapsed node on Right and moves into an expanded one", () => {
		expect(press("ArrowRight", "beta").toggle).toEqual({
			id: "beta",
			expanded: true,
		});
		expect(press("ArrowRight", "alpha").focusId).toBe("alpha-child");
		const leaf = press("ArrowRight", "gamma");
		expect(leaf.handled).toBe(true);
		expect(leaf.focusId).toBeUndefined();
		expect(leaf.toggle).toBeUndefined();
	});

	it("collapses an expanded node on Left, otherwise moves to the parent", () => {
		expect(press("ArrowLeft", "alpha").toggle).toEqual({
			id: "alpha",
			expanded: false,
		});
		expect(press("ArrowLeft", "alpha-child").focusId).toBe("alpha");
		expect(press("ArrowLeft", "beta").focusId).toBe("root");
	});

	it("moves to the parent instead of collapsing a force-expanded node", () => {
		const forced = rows.map((row) =>
			row.id === "alpha" ? { ...row, isForceExpanded: true } : row,
		);
		const result = treeKeyboardReducer(forced, "alpha", "ArrowLeft", "");
		expect(result.toggle).toBeUndefined();
		expect(result.focusId).toBe("root");
	});

	it("opens the focused row on Enter", () => {
		expect(press("Enter", "gamma").open).toBe("gamma");
	});

	it("expands collapsed siblings with children on *", () => {
		expect(press("*", "gamma").expandIds).toEqual(["beta"]);
	});

	it("type-ahead moves to the next row starting with the character", () => {
		expect(press("g", "root")).toMatchObject({
			focusId: "gamma",
			typeahead: "g",
		});
		expect(press("a", "alpha")).toMatchObject({
			focusId: "alpha",
			typeahead: "a",
		});
	});

	it("type-ahead refines with a growing buffer and cycles on a repeated character", () => {
		expect(press("e", "beta", "b")).toMatchObject({
			focusId: "beta",
			typeahead: "be",
		});
		expect(press("n", "alpha", "").focusId).toBe("alpha-child");
		expect(press("a", "alpha", "a")).toMatchObject({
			focusId: "alpha",
			typeahead: "a",
		});
	});

	it("leaves unrelated keys unhandled and clears the buffer", () => {
		expect(press("Tab", "root")).toEqual({ handled: false, typeahead: "" });
		expect(press(" ", "root").handled).toBe(false);
	});
});
