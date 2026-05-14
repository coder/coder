import { describe, expect, it } from "vitest";
import { anthropicKnownModels } from "./anthropic";
import { getKnownModelsForProvider } from "./index";
import type { KnownModel } from "./types";

const anthropicKnownModelList: readonly KnownModel[] = anthropicKnownModels;

const requireAnthropicKnownModel = (modelIdentifier: string): KnownModel => {
	const knownModel = anthropicKnownModelList.find(
		(knownModel) => knownModel.modelIdentifier === modelIdentifier,
	);
	if (knownModel === undefined) {
		throw new Error(`missing Anthropic Known Model: ${modelIdentifier}`);
	}
	return knownModel;
};

describe("anthropicKnownModels", () => {
	it("returns Anthropic canonical IDs in declared order", () => {
		expect(
			getKnownModelsForProvider("anthropic").map(
				(knownModel) => knownModel.modelIdentifier,
			),
		).toEqual([
			"claude-opus-4-7",
			"claude-opus-4-6",
			"claude-sonnet-4-6",
			"claude-haiku-4-5",
			"claude-sonnet-4-5",
		]);
	});

	it("declares Anthropic reasoning defaults by API support", () => {
		for (const modelIdentifier of ["claude-opus-4-7", "claude-opus-4-6"]) {
			const knownModel = requireAnthropicKnownModel(modelIdentifier);

			expect(knownModel.reasoningEffort).toBe("high");
			expect(knownModel.thinkingBudgetTokens).toBeUndefined();
		}

		const sonnet46 = requireAnthropicKnownModel("claude-sonnet-4-6");
		expect(sonnet46.reasoningEffort).toBe("medium");
		expect(sonnet46.thinkingBudgetTokens).toBeUndefined();

		for (const modelIdentifier of ["claude-haiku-4-5", "claude-sonnet-4-5"]) {
			const knownModel = requireAnthropicKnownModel(modelIdentifier);

			expect(knownModel.reasoningEffort).toBeUndefined();
			expect(knownModel.thinkingBudgetTokens).toBe(8192);
		}
	});

	it("has source metadata, provider equality, and declared order", () => {
		expect(
			anthropicKnownModels.map((knownModel) => knownModel.modelIdentifier),
		).toEqual([
			"claude-opus-4-7",
			"claude-opus-4-6",
			"claude-sonnet-4-6",
			"claude-haiku-4-5",
			"claude-sonnet-4-5",
		]);

		for (const knownModel of anthropicKnownModels) {
			expect(knownModel.provider).toBe("anthropic");
			expect(knownModel.sourceMetadata.sourceName).toBe("models.dev");
			expect(knownModel.sourceMetadata.sourceRetrievedAt).toBe("2026-04-30");
			expect(knownModel.sourceMetadata.lastUpdated).not.toBe("");
		}
	});
});
