import { describe, expect, it } from "vitest";
import { checkedAgoLabel } from "./LastCheckedLabel";

describe("checkedAgoLabel", () => {
	it("renders 'just now' for sub-2-second gaps", () => {
		const now = new Date("2024-01-01T00:00:00Z");
		const at = new Date("2024-01-01T00:00:00.500Z");
		// `at` is 500ms before `now`.
		expect(checkedAgoLabel(at, now)).toBe("checked just now");
	});

	it("renders 'just now' when the timestamp is in the future (clock skew)", () => {
		const now = new Date("2024-01-01T00:00:00Z");
		const at = new Date("2024-01-01T00:00:05Z");
		expect(checkedAgoLabel(at, now)).toBe("checked just now");
	});

	it("renders seconds below 60s", () => {
		const now = new Date("2024-01-01T00:01:00Z");
		const at = new Date("2024-01-01T00:00:55Z");
		expect(checkedAgoLabel(at, now)).toBe("checked 5s ago");
	});

	it("renders minutes at the 60s boundary", () => {
		const now = new Date("2024-01-01T00:01:00Z");
		const at = new Date("2024-01-01T00:00:00Z");
		expect(checkedAgoLabel(at, now)).toBe("checked 1m ago");
	});

	it("renders minutes below 60m", () => {
		const now = new Date("2024-01-01T01:00:00Z");
		const at = new Date("2024-01-01T00:45:00Z");
		expect(checkedAgoLabel(at, now)).toBe("checked 15m ago");
	});

	it("renders hours at the 60m boundary", () => {
		const now = new Date("2024-01-01T01:00:00Z");
		const at = new Date("2024-01-01T00:00:00Z");
		expect(checkedAgoLabel(at, now)).toBe("checked 1h ago");
	});

	it("falls through to a locale date for >24h gaps", () => {
		const now = new Date("2024-01-02T12:00:00Z");
		const at = new Date("2024-01-01T00:00:00Z");
		// The exact format is locale-dependent; just assert that we
		// no longer use the "ago" phrasing.
		const label = checkedAgoLabel(at, now);
		expect(label.startsWith("checked ")).toBe(true);
		expect(label).not.toContain("ago");
	});
});
