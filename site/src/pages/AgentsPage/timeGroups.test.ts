import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getTimeGroup, TIME_GROUPS } from "./timeGroups";

describe("getTimeGroup", () => {
	beforeEach(() => {
		// Pin "now" to 2025-07-15T14:30:00.000Z (a Tuesday) so all
		// assertions are deterministic.
		vi.useFakeTimers();
		vi.setSystemTime(new Date("2025-07-15T14:30:00.000Z"));
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("exports the expected group labels in order", () => {
		expect(TIME_GROUPS).toEqual(["Today", "Yesterday", "This Week", "Older"]);
	});

	it('returns "Today" for a date later today', () => {
		expect(getTimeGroup("2025-07-15T18:00:00.000Z")).toBe("Today");
	});

	it('returns "Today" for a date at start of today (midnight)', () => {
		expect(getTimeGroup("2025-07-15T00:00:00.000Z")).toBe("Today");
	});

	it('returns "Today" for a date earlier today', () => {
		expect(getTimeGroup("2025-07-15T08:00:00.000Z")).toBe("Today");
	});

	it('returns "Yesterday" for a date in the previous calendar day', () => {
		expect(getTimeGroup("2025-07-14T10:00:00.000Z")).toBe("Yesterday");
	});

	it('returns "Yesterday" for exactly midnight of yesterday', () => {
		expect(getTimeGroup("2025-07-14T00:00:00.000Z")).toBe("Yesterday");
	});

	it('returns "This Week" for a date 2 days ago', () => {
		expect(getTimeGroup("2025-07-13T12:00:00.000Z")).toBe("This Week");
	});

	it('returns "This Week" for a date 6 days ago', () => {
		expect(getTimeGroup("2025-07-09T12:00:00.000Z")).toBe("This Week");
	});

	it('returns "This Week" for exactly 7 days ago at midnight', () => {
		// weekAgo = today - 7 days = 2025-07-08T00:00:00 local.
		// A date at exactly that boundary should be "This Week"
		// because the comparison is date >= weekAgo.
		expect(getTimeGroup("2025-07-08T00:00:00.000Z")).toBe("This Week");
	});

	it('returns "Older" for a date 8 days ago', () => {
		expect(getTimeGroup("2025-07-07T23:59:59.000Z")).toBe("Older");
	});

	it('returns "Older" for a date far in the past', () => {
		expect(getTimeGroup("2024-01-01T00:00:00.000Z")).toBe("Older");
	});

	it('returns "Today" for a date in the future', () => {
		expect(getTimeGroup("2025-07-20T00:00:00.000Z")).toBe("Today");
	});

	describe("midnight boundary edge cases", () => {
		it('classifies 1 second before midnight as "Yesterday"', () => {
			// One second before today's midnight in UTC.
			expect(getTimeGroup("2025-07-14T23:59:59.000Z")).toBe("Yesterday");
		});

		it('classifies exactly midnight as "Today"', () => {
			expect(getTimeGroup("2025-07-15T00:00:00.000Z")).toBe("Today");
		});

		it('classifies 1 second after midnight as "Today"', () => {
			expect(getTimeGroup("2025-07-15T00:00:01.000Z")).toBe("Today");
		});
	});
});
