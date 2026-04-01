import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { shortRelativeTime } from "./time";

describe("shortRelativeTime", () => {
	// Pin "now" so tests are deterministic.
	const NOW = new Date("2025-06-15T12:00:00Z");

	beforeEach(() => {
		vi.useFakeTimers();
		vi.setSystemTime(NOW);
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it('returns "now" for 0 seconds ago', () => {
		expect(shortRelativeTime(NOW)).toBe("now");
	});

	it('returns "now" for 30 seconds ago', () => {
		const date = new Date(NOW.getTime() - 30 * 1000);
		expect(shortRelativeTime(date)).toBe("now");
	});

	it('returns "now" for 59 seconds ago', () => {
		const date = new Date(NOW.getTime() - 59 * 1000);
		expect(shortRelativeTime(date)).toBe("now");
	});

	it('returns "1m" for 60 seconds ago', () => {
		const date = new Date(NOW.getTime() - 60 * 1000);
		expect(shortRelativeTime(date)).toBe("1m");
	});

	it('returns "5m" for 5 minutes ago', () => {
		const date = new Date(NOW.getTime() - 5 * 60 * 1000);
		expect(shortRelativeTime(date)).toBe("5m");
	});

	it('returns "59m" for 59 minutes ago', () => {
		const date = new Date(NOW.getTime() - 59 * 60 * 1000);
		expect(shortRelativeTime(date)).toBe("59m");
	});

	it('returns "1h" for 60 minutes ago', () => {
		const date = new Date(NOW.getTime() - 60 * 60 * 1000);
		expect(shortRelativeTime(date)).toBe("1h");
	});

	it('returns "23h" for 23 hours ago', () => {
		const date = new Date(NOW.getTime() - 23 * 60 * 60 * 1000);
		expect(shortRelativeTime(date)).toBe("23h");
	});

	it('returns "1d" for 24 hours ago', () => {
		const date = new Date(NOW.getTime() - 24 * 60 * 60 * 1000);
		expect(shortRelativeTime(date)).toBe("1d");
	});

	it('returns "6d" for 6 days ago', () => {
		const date = new Date(NOW.getTime() - 6 * 24 * 60 * 60 * 1000);
		expect(shortRelativeTime(date)).toBe("6d");
	});

	it('returns "1w" for 7 days ago', () => {
		const date = new Date(NOW.getTime() - 7 * 24 * 60 * 60 * 1000);
		expect(shortRelativeTime(date)).toBe("1w");
	});

	it('returns "4w" for 30 days ago', () => {
		const date = new Date(NOW.getTime() - 30 * 24 * 60 * 60 * 1000);
		expect(shortRelativeTime(date)).toBe("4w");
	});

	it("returns months for dates 2-11 months ago", () => {
		// ~3 months ago
		const date = new Date(NOW.getTime() - 90 * 24 * 60 * 60 * 1000);
		const result = shortRelativeTime(date);
		expect(result).toMatch(/^\d+mo$/);
	});

	it('returns "1y" for a date 1 year ago', () => {
		const date = new Date(NOW.getTime() - 365 * 24 * 60 * 60 * 1000);
		expect(shortRelativeTime(date)).toBe("1y");
	});

	it('returns "now" for a future date (graceful handling)', () => {
		// A date 5 minutes in the future results in a negative diff,
		// which dayjs reports as 0 seconds.
		const date = new Date(NOW.getTime() + 5 * 60 * 1000);
		expect(shortRelativeTime(date)).toBe("now");
	});

	it("accepts ISO string input", () => {
		const isoStr = new Date(NOW.getTime() - 2 * 60 * 60 * 1000).toISOString();
		expect(shortRelativeTime(isoStr)).toBe("2h");
	});

	it("accepts numeric timestamp input", () => {
		const timestamp = NOW.getTime() - 10 * 60 * 1000;
		expect(shortRelativeTime(timestamp)).toBe("10m");
	});
});
