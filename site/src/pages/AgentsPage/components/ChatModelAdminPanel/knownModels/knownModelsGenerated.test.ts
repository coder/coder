import { describe, expect, it } from "vitest";
import knownModelsGenerated from "./knownModelsGenerated.json";

// knownModelsGenerated.json crosses a typed boundary via a cast in index.ts,
// so this suite validates the shape and enum values of every generated entry.
const providers = Object.entries(knownModelsGenerated);

describe("knownModelsGenerated", () => {
	it("pins the Anthropic thinking-mode split", () => {
		// Anthropic has two mutually exclusive thinking APIs. Models on the
		// legacy `thinking.budget_tokens` API must not be sent
		// reasoningEffort: setting effort on a legacy-thinking model returns
		// HTTP 400 from Anthropic. This map pins which side of the split each
		// curated model is on; update it deliberately when curating models.
		const thinkingMode = Object.fromEntries(
			knownModelsGenerated.anthropic.map((model) => {
				let mode = "none";
				if ("thinkingBudgetTokens" in model) {
					mode = "thinkingBudgetTokens";
				} else if ("reasoningEffort" in model) {
					mode = "reasoningEffort";
				}
				return [model.modelIdentifier, mode];
			}),
		);
		expect(thinkingMode).toEqual({
			"claude-fable-5": "reasoningEffort",
			"claude-mythos-5": "reasoningEffort",
			"claude-opus-4-8": "reasoningEffort",
			"claude-opus-4-7": "reasoningEffort",
			"claude-opus-4-6": "reasoningEffort",
			"claude-sonnet-5": "reasoningEffort",
			"claude-sonnet-4-6": "reasoningEffort",
			"claude-haiku-4-5": "thinkingBudgetTokens",
			"claude-sonnet-4-5": "thinkingBudgetTokens",
		});
	});

	it("pins the claude-sonnet-4-5 context limit override", () => {
		// Pinned to the flat-priced 200k tier by
		// scripts/aibridgepricesgen/overrides.jq; guards the override at the
		// generated-artifact layer.
		const sonnet45 = knownModelsGenerated.anthropic.find(
			(model) => model.modelIdentifier === "claude-sonnet-4-5",
		);
		expect(sonnet45?.contextLimit).toBe(200000);
	});

	it.each(providers)("validates every %s entry", (provider, models) => {
		expect(models.length).toBeGreaterThan(0);
		for (const model of models) {
			expect(model.provider).toBe(provider);
			expect(model.modelIdentifier).not.toBe("");
			expect(model.displayName).not.toBe("");
			expect(Array.isArray(model.aliases)).toBe(true);

			const record = model as Record<string, unknown>;
			if (record.reasoningEffort !== undefined) {
				expect(["low", "medium", "high"]).toContain(record.reasoningEffort);
			}
			expect(
				record.reasoningEffort !== undefined &&
					record.thinkingBudgetTokens !== undefined,
			).toBe(false);

			// Token limits and budgets must be strictly positive; costs may
			// legitimately be zero upstream (e.g. gpt-3.5-turbo cache_read).
			for (const field of [
				"contextLimit",
				"maxOutputTokens",
				"thinkingBudgetTokens",
			]) {
				const value = record[field];
				if (value !== undefined) {
					expect(typeof value, field).toBe("number");
					expect(value, field).toBeGreaterThan(0);
				}
			}
			for (const field of [
				"inputCost",
				"outputCost",
				"cacheReadCost",
				"cacheWriteCost",
			]) {
				const value = record[field];
				if (value !== undefined) {
					expect(typeof value, field).toBe("number");
					expect(value, field).toBeGreaterThanOrEqual(0);
				}
			}
		}
	});
});
