import { describe, expect, it } from "vitest";
import type { ResolvedCompactionThreshold } from "../compactionTriggers";
import {
	compactionThresholdLabel,
	compactionThresholdTokens,
	contextUsageLabel,
	formatContextTokenCount,
} from "./contextUsage";

const modelThreshold = (
	percent: number,
	organizationOverrideNotLoaded = false,
): ResolvedCompactionThreshold => ({
	percent,
	source: "model",
	organizationOverrideNotLoaded,
});
const organizationThreshold = (
	pointTokens: number,
): ResolvedCompactionThreshold => ({ source: "organization", pointTokens });

describe("contextUsageLabel", () => {
	it.each([0, 70, 100, undefined])(
		"uses the full window with a %s threshold",
		(percent) => {
			expect(
				contextUsageLabel({
					usedTokens: 211_400,
					contextLimitTokens: 1_000_000,
					compactionThreshold:
						percent === undefined ? undefined : modelThreshold(percent),
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
		[Number.NaN, "Auto-compaction threshold unavailable"],
		[Number.POSITIVE_INFINITY, "Auto-compaction threshold unavailable"],
	])("describes the %s chat threshold", (percent, expected) => {
		expect(compactionThresholdLabel(modelThreshold(percent), 1_000)).toBe(
			expected,
		);
	});
	it("does not invent a threshold before settings load", () => {
		expect(compactionThresholdLabel(undefined, 1_000)).toBe(
			"Auto-compaction threshold unavailable",
		);
	});
	it("flags chat thresholds when the organization override did not load", () => {
		expect(compactionThresholdLabel(modelThreshold(70, true), 1_000)).toBe(
			"Compacts at 70% (organization override not loaded)",
		);
		expect(compactionThresholdLabel(modelThreshold(100, true), 1_000)).toBe(
			"Compaction off (organization override not loaded)",
		);
	});
	it("measures a binding organization override against the window", () => {
		expect(
			compactionThresholdLabel(organizationThreshold(32_000), 200_000),
		).toBe("Compacts at 16% (organization override)");
		expect(
			compactionThresholdLabel(organizationThreshold(16_000), 10_000),
		).toBe("Auto-compaction disabled");
		expect(
			compactionThresholdLabel(organizationThreshold(32_000), undefined),
		).toBe("Auto-compaction threshold unavailable");
	});
});
describe("compactionThresholdTokens", () => {
	it("resolves the chat trigger like the backend", () => {
		expect(compactionThresholdTokens(modelThreshold(70), 1_000)).toBe(700);
		expect(compactionThresholdTokens(modelThreshold(100), 1_000)).toBe(
			undefined,
		);
		expect(compactionThresholdTokens(undefined, 1_000)).toBe(undefined);
	});
	it("rounds an organization point up to the first triggering token count", () => {
		expect(
			compactionThresholdTokens(organizationThreshold(6_400.4), 20_000),
		).toBe(6_401);
	});
	it("omits organization points that are unknown or beyond the window", () => {
		expect(
			compactionThresholdTokens(organizationThreshold(16_000), 10_000),
		).toBe(undefined);
		expect(
			compactionThresholdTokens(organizationThreshold(16_000), undefined),
		).toBe(undefined);
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
