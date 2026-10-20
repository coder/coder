import { describe, expect, it } from "vitest";
import {
	chevronColumnWidth,
	countFittingColumns,
	defaultSessionColumnSettings,
	getActiveView,
	moveShownColumn,
	parseSessionColumnSettings,
	type SessionColumnSettings,
	serializeSessionColumnSettings,
	sessionColumnIds,
	sessionColumns,
	sessionColumnViews,
	toggleColumn,
} from "./sessionColumns";

const settings: SessionColumnSettings = {
	shown: ["last_prompt", "user", "client"],
	notShown: [
		"provider",
		"model",
		"tokens",
		"cost",
		"network",
		"threads",
		"last_active",
	],
};

describe("sessionColumnViews", () => {
	it("lists every column exactly once in each view", () => {
		for (const view of sessionColumnViews) {
			const ids = [...view.settings.shown, ...view.settings.notShown];
			expect([...ids].sort()).toEqual([...sessionColumnIds].sort());
		}
	});

	it("never shows the model column by default", () => {
		for (const view of sessionColumnViews) {
			expect(view.settings.shown).not.toContain("model");
		}
	});
});

describe("getActiveView", () => {
	it("matches the view with the same shown columns in the same order", () => {
		const security = sessionColumnViews[1];
		expect(getActiveView(security.settings)?.id).toBe("security");
	});

	it("returns undefined once the columns are customized", () => {
		const reordered = moveShownColumn(defaultSessionColumnSettings, "user", 0);
		expect(getActiveView(reordered)).toBeUndefined();
	});
});

describe("toggleColumn", () => {
	it("adds a shown column to the end of the shown list", () => {
		expect(toggleColumn(settings, "cost").shown).toEqual([
			"last_prompt",
			"user",
			"client",
			"cost",
		]);
	});

	it("moves a hidden column to the top of the not shown list", () => {
		const next = toggleColumn(settings, "user");
		expect(next.shown).toEqual(["last_prompt", "client"]);
		expect(next.notShown[0]).toBe("user");
	});

	it("keeps the last shown column", () => {
		const single: SessionColumnSettings = {
			shown: ["user"],
			notShown: settings.notShown,
		};
		expect(toggleColumn(single, "user")).toBe(single);
	});
});

describe("moveShownColumn", () => {
	it("moves a column to the target index", () => {
		expect(moveShownColumn(settings, "client", 0).shown).toEqual([
			"client",
			"last_prompt",
			"user",
		]);
	});
});

describe("parseSessionColumnSettings", () => {
	it("returns the default without saved settings", () => {
		expect(parseSessionColumnSettings(null)).toBe(defaultSessionColumnSettings);
	});

	it("restores a saved view from its current definition", () => {
		expect(parseSessionColumnSettings(JSON.stringify({ view: "cost" }))).toBe(
			sessionColumnViews[2].settings,
		);
	});

	it("round-trips a selected view and customized columns", () => {
		for (const value of [sessionColumnViews[1].settings, settings]) {
			expect(
				parseSessionColumnSettings(serializeSessionColumnSettings(value)),
			).toEqual(value);
		}
		expect(
			JSON.parse(
				serializeSessionColumnSettings(sessionColumnViews[1].settings),
			),
		).toEqual({ view: "security" });
	});

	it("returns the default for invalid JSON", () => {
		expect(parseSessionColumnSettings("{")).toBe(defaultSessionColumnSettings);
	});

	it("returns the default when no saved column is shown", () => {
		expect(
			parseSessionColumnSettings(JSON.stringify({ shown: ["removed"] })),
		).toBe(defaultSessionColumnSettings);
	});

	it("drops unknown columns and hides columns missing from the saved value", () => {
		const parsed = parseSessionColumnSettings(
			JSON.stringify({
				shown: ["user", "removed", "user", "cost"],
				notShown: ["threads"],
			}),
		);
		expect(parsed.shown).toEqual(["user", "cost"]);
		expect(parsed.notShown[0]).toBe("threads");
		expect([...parsed.shown, ...parsed.notShown].sort()).toEqual(
			[...sessionColumnIds].sort(),
		);
	});
});

describe("countFittingColumns", () => {
	const shown = ["last_prompt", "user", "client"] as const;
	const fullWidth =
		chevronColumnWidth +
		sessionColumns.last_prompt.width +
		sessionColumns.user.width +
		sessionColumns.client.width;

	it("fits every column when there is room", () => {
		expect(countFittingColumns(shown, fullWidth)).toBe(3);
	});

	it("hides columns from the end first", () => {
		expect(countFittingColumns(shown, fullWidth - 1)).toBe(2);
	});

	it("always keeps the first column", () => {
		expect(countFittingColumns(shown, 10)).toBe(1);
	});
});
