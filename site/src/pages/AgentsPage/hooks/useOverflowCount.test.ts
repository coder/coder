import { describe, expect, it } from "vitest";
import { computeOverflowCount, countThatFit } from "./useOverflowCount";

describe("countThatFit", () => {
	it("returns every item when the row fits exactly", () => {
		// 3 items of 100 with 2 gaps of 4 = 308.
		expect(countThatFit([100, 100, 100], 4, 308)).toBe(3);
	});

	it("tolerates subpixel rounding", () => {
		expect(countThatFit([100, 100, 100], 4, 307.5)).toBe(3);
	});

	it("counts only the leading items that fit", () => {
		// First item 100; the second needs 204 total, above the 202
		// budget even with tolerance.
		expect(countThatFit([100, 100, 100], 4, 202)).toBe(1);
	});

	it("charges no gap before the first item", () => {
		expect(countThatFit([100], 4, 100)).toBe(1);
	});

	it("returns zero when nothing fits", () => {
		expect(countThatFit([100, 50], 4, 90)).toBe(0);
	});

	it("treats unknown (zero) widths as free", () => {
		// Items never measured while visible cannot block the row.
		expect(countThatFit([0, 0, 100], 4, 110)).toBe(3);
	});
});

describe("computeOverflowCount", () => {
	it("reports zero when everything fits without the pill", () => {
		expect(
			computeOverflowCount({
				widths: [100, 100],
				gap: 4,
				available: 204,
				pillWidth: 30,
			}),
		).toBe(0);
	});

	it("reserves the pill width once something overflows", () => {
		// All three need 308; only 250 available. With the pill (30 + 4
		// gap) reserved, the budget is 216, fitting two items (204).
		expect(
			computeOverflowCount({
				widths: [100, 100, 100],
				gap: 4,
				available: 250,
				pillWidth: 30,
			}),
		).toBe(1);
	});

	it("overflows everything when even one item cannot fit", () => {
		expect(
			computeOverflowCount({
				widths: [200, 210],
				gap: 4,
				available: 160,
				pillWidth: 30,
			}),
		).toBe(2);
	});

	it("reports at least one overflow when the first pass fails", () => {
		// Borderline case: items fit without the pill's reservation but
		// not with it; the pill still needs one occupant.
		expect(
			computeOverflowCount({
				widths: [100, 100],
				gap: 4,
				available: 200,
				pillWidth: 30,
			}),
		).toBe(1);
	});
});
