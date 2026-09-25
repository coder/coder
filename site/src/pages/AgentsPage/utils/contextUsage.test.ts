import { describe, expect, it } from "vitest";
import {
	compactionThresholdLabel,
	contextUsageLabel,
	formatContextTokenCount,
} from "./contextUsage";

describe("contextUsageLabel", () => {
	it.each([0, 70, 100, undefined])(
		"uses the full window with a %s threshold",
		(compressionThreshold) => {
			expect(
				contextUsageLabel({
					usedTokens: 211_400,
					contextLimitTokens: 1_000_000,
					compressionThreshold,
				}),
			).toBe("21% - 211.4K / 1M context used");
		},
	);
	it("does not invent missing numbers", () => {
		expect(contextUsageLabel(null)).toBe("Context usage unavailable");
		expect(contextUsageLabel({ usedTokens: 26_000 })).toBe(
			"26K tokens used; context window size unavailable",
		);
		expect(contextUsageLabel({ usedTokens: Number.NaN })).toBe(
			"Context usage unavailable",
		);
		expect(
			contextUsageLabel({ usedTokens: 26_000, contextLimitTokens: 0 }),
		).toBe("26K tokens used; context window size unavailable");
	});
	it("preserves measured zero and qualifies estimates", () => {
		expect(
			contextUsageLabel({ usedTokens: 0, contextLimitTokens: 1_000 }),
		).toBe("0% - 0 / 1K context used");
		expect(
			contextUsageLabel({
				usedTokens: 350,
				contextLimitTokens: 1_000,
				estimated: true,
			}),
		).toBe(
			"35% - 350 / 1K context used (estimated from compaction summary only)",
		);
	});
});
describe("compactionThresholdLabel", () => {
	it.each([
		[70, "Compacts at 70%"],
		[0, "Compacts at 0%"],
		[100, "Auto-compaction disabled"],
		[-1, "Compacts at 70%"],
		[101, "Compacts at 70%"],
		[undefined, "Auto-compaction threshold unavailable"],
		[Number.NaN, "Auto-compaction threshold unavailable"],
		[Number.POSITIVE_INFINITY, "Auto-compaction threshold unavailable"],
	])("describes the %s threshold", (threshold, expected) => {
		expect(compactionThresholdLabel(threshold)).toBe(expected);
	});
});
describe("formatContextTokenCount", () => {
	it("uses deterministic counts and compact units", () => {
		expect(formatContextTokenCount(1_100_000)).toBe("1,100,000");
		expect(formatContextTokenCount(1_100_000, true)).toBe("1.1M");
		expect(formatContextTokenCount(26_000, true)).toBe("26K");
		expect(formatContextTokenCount(-1)).toBe("Unknown");
		expect(formatContextTokenCount(Number.POSITIVE_INFINITY)).toBe("Unknown");
	});
});
