import { describe, expect, it } from "vitest";
import { formatUsedAgentHours } from "./agentHours";

describe("formatUsedAgentHours", () => {
	it.each([
		[0, "0.0"],
		[359_999, "0.0"],
		[360_000, "0.1"],
		[4_499_999_999, "1,249.9"],
		[Number.NaN, "0.0"],
	])("formats %d ms as %s hours", (ms, text) => {
		expect(formatUsedAgentHours(ms)).toBe(text);
	});
});
