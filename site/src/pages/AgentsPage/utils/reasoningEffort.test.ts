import { describe, expect, it } from "vitest";
import {
	clampReasoningEffort,
	formatReasoningEffort,
	getSelectableReasoningEfforts,
	getSupportedReasoningEfforts,
	reasoningEffortRank,
} from "./reasoningEffort";

describe("reasoningEffortRank", () => {
	it("orders efforts on the global scale", () => {
		expect(reasoningEffortRank("none")).toBeLessThan(
			reasoningEffortRank("minimal"),
		);
		expect(reasoningEffortRank("minimal")).toBeLessThan(
			reasoningEffortRank("low"),
		);
		expect(reasoningEffortRank("low")).toBeLessThan(
			reasoningEffortRank("medium"),
		);
		expect(reasoningEffortRank("medium")).toBeLessThan(
			reasoningEffortRank("high"),
		);
		expect(reasoningEffortRank("high")).toBeLessThan(
			reasoningEffortRank("xhigh"),
		);
		expect(reasoningEffortRank("xhigh")).toBeLessThan(
			reasoningEffortRank("max"),
		);
	});

	it("ranks none at the bottom of the scale", () => {
		expect(reasoningEffortRank("none")).toBe(0);
	});

	it("returns -1 for unknown values", () => {
		expect(reasoningEffortRank("extreme")).toBe(-1);
		expect(reasoningEffortRank("")).toBe(-1);
	});

	it("normalizes case and whitespace", () => {
		expect(reasoningEffortRank(" High ")).toBe(reasoningEffortRank("high"));
	});
});

describe("formatReasoningEffort", () => {
	it.each([
		["none", "None"],
		["minimal", "Minimal"],
		["low", "Low"],
		["medium", "Medium"],
		["high", "High"],
		["xhigh", "Xhigh"],
		["max", "Max"],
	])("formats %s as %s", (value, expected) => {
		expect(formatReasoningEffort(value)).toBe(expected);
	});
});

describe("getSupportedReasoningEfforts", () => {
	it("returns provider runtime sets", () => {
		expect(getSupportedReasoningEfforts("openai")).toEqual([
			"minimal",
			"low",
			"medium",
			"high",
			"xhigh",
		]);
		expect(getSupportedReasoningEfforts("anthropic")).toEqual([
			"low",
			"medium",
			"high",
			"xhigh",
			"max",
		]);
		expect(getSupportedReasoningEfforts("openrouter")).toEqual([
			"low",
			"medium",
			"high",
		]);
		expect(getSupportedReasoningEfforts("vercel")).toEqual([
			"none",
			"minimal",
			"low",
			"medium",
			"high",
			"xhigh",
		]);
	});

	it("shares OpenAI's set with Azure and Anthropic's with Bedrock", () => {
		expect(getSupportedReasoningEfforts("azure")).toEqual(
			getSupportedReasoningEfforts("openai"),
		);
		expect(getSupportedReasoningEfforts("bedrock")).toEqual(
			getSupportedReasoningEfforts("anthropic"),
		);
	});

	it("returns an empty set for unsupported providers", () => {
		expect(getSupportedReasoningEfforts("google")).toEqual([]);
		expect(getSupportedReasoningEfforts("")).toEqual([]);
	});
});

describe("getSelectableReasoningEfforts", () => {
	it("returns provider values up to the configured max", () => {
		expect(
			getSelectableReasoningEfforts({
				provider: "openai",
				reasoningEffortDefault: "medium",
				reasoningEffortMax: "high",
			}),
		).toEqual(["minimal", "low", "medium", "high"]);
	});

	it("returns the full supported set when max is the top value", () => {
		expect(
			getSelectableReasoningEfforts({
				provider: "anthropic",
				reasoningEffortMax: "max",
			}),
		).toEqual(["low", "medium", "high", "xhigh", "max"]);
	});

	it("returns empty when max is not configured", () => {
		expect(
			getSelectableReasoningEfforts({
				provider: "openai",
				reasoningEffortDefault: "medium",
			}),
		).toEqual([]);
	});

	it("returns empty for providers without effort support", () => {
		expect(
			getSelectableReasoningEfforts({
				provider: "google",
				reasoningEffortMax: "high",
			}),
		).toEqual([]);
	});

	it("keeps the provider minimum when max is below it", () => {
		expect(
			getSelectableReasoningEfforts({
				provider: "anthropic",
				reasoningEffortMax: "minimal",
			}),
		).toEqual(["low"]);
	});

	it("ignores invalid max values", () => {
		expect(
			getSelectableReasoningEfforts({
				provider: "openai",
				reasoningEffortMax: "extreme",
			}),
		).toEqual([]);
	});

	it("includes none for Vercel from the bottom of its range", () => {
		expect(
			getSelectableReasoningEfforts({
				provider: "vercel",
				reasoningEffortDefault: "medium",
				reasoningEffortMax: "xhigh",
			}),
		).toEqual(["none", "minimal", "low", "medium", "high", "xhigh"]);
		// "none" itself is a selectable value.
		expect(
			clampReasoningEffort("none", {
				provider: "vercel",
				reasoningEffortDefault: "medium",
				reasoningEffortMax: "xhigh",
			}),
		).toBe("none");
	});
});

describe("clampReasoningEffort", () => {
	const openaiModel = {
		provider: "openai",
		reasoningEffortDefault: "medium",
		reasoningEffortMax: "high",
	};

	it("keeps a value the model supports under its max", () => {
		expect(clampReasoningEffort("low", openaiModel)).toBe("low");
		expect(clampReasoningEffort("high", openaiModel)).toBe("high");
	});

	it("falls back to the default when the value exceeds max", () => {
		expect(clampReasoningEffort("xhigh", openaiModel)).toBe("medium");
	});

	it("falls back to the default when the value is unsupported", () => {
		// "max" is on the global scale but not in OpenAI's set.
		expect(clampReasoningEffort("max", openaiModel)).toBe("medium");
		expect(clampReasoningEffort("bogus", openaiModel)).toBe("medium");
	});

	it("falls back to the default when no value is given", () => {
		expect(clampReasoningEffort(undefined, openaiModel)).toBe("medium");
		expect(clampReasoningEffort("", openaiModel)).toBe("medium");
	});

	it("normalizes case and whitespace", () => {
		expect(clampReasoningEffort(" High ", openaiModel)).toBe("high");
	});

	it("snaps a default above max down to max", () => {
		expect(
			clampReasoningEffort(undefined, {
				provider: "openai",
				reasoningEffortDefault: "xhigh",
				reasoningEffortMax: "medium",
			}),
		).toBe("medium");
	});

	it("returns undefined when the model has no effort configured", () => {
		expect(
			clampReasoningEffort("high", {
				provider: "openai",
				reasoningEffortDefault: "medium",
			}),
		).toBeUndefined();
	});

	it("returns undefined when no default is configured and value invalid", () => {
		expect(
			clampReasoningEffort("bogus", {
				provider: "openai",
				reasoningEffortMax: "high",
			}),
		).toBeUndefined();
	});
});
