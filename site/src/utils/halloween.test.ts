import { isHalloweenEnabled } from "./halloween";

describe("isHalloweenEnabled", () => {
	it.each<[string, Date, boolean]>([
		["Oct 23", new Date(2026, 9, 23, 23, 59), false],
		["Oct 24", new Date(2026, 9, 24, 0, 0), true],
		["Oct 31, late", new Date(2026, 9, 31, 23, 59), true],
		["Nov 1", new Date(2026, 10, 1, 0, 0), false],
		["Mar 1", new Date(2026, 2, 1), false],
	])("follows the date: %s", (_, date, expected) => {
		expect(isHalloweenEnabled(date, "")).toBe(expected);
	});

	it("can be forced on or off with ?halloween", () => {
		expect(isHalloweenEnabled(new Date(2026, 2, 1), "?halloween=1")).toBe(true);
		expect(isHalloweenEnabled(new Date(2026, 9, 30), "?halloween=0")).toBe(
			false,
		);
	});
});
