import { describe, expect, it } from "vitest";
import { contextUsageLabel, formatContextTokenCount } from "./contextUsage";

describe("contextUsageLabel", () => {
	it("uses the trigger budget instead of the full window", () => {
		expect(
			contextUsageLabel({
				usedTokens: 26_000,
				contextLimitTokens: 1_100_000,
				compressionThreshold: 70,
			}),
		).toBe("26K of 770K before auto-compaction");
	});
	it("does not invent missing numbers", () => {
		expect(contextUsageLabel(null)).toBe("Context usage unavailable");
		expect(contextUsageLabel({ usedTokens: 26_000 })).toBe(
			"26K tokens used; auto-compaction budget unavailable",
		);
		expect(contextUsageLabel({ usedTokens: Number.NaN })).toBe(
			"Context usage unavailable",
		);
	});
	it("qualifies disabled compaction, zero threshold, and estimates", () => {
		expect(contextUsageLabel({ compressionThreshold: 100 })).toBe(
			"Context usage unavailable; automatic compaction is disabled",
		);
		expect(
			contextUsageLabel({
				usedTokens: 0,
				contextLimitTokens: 100,
				compressionThreshold: 0,
			}),
		).toBe("0 of 1 before auto-compaction");
		expect(
			contextUsageLabel({
				usedTokens: 350,
				contextLimitTokens: 1000,
				compressionThreshold: 70,
				estimated: true,
			}),
		).toBe(
			"350 of 700 before auto-compaction (estimated from compaction summary only)",
		);
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
