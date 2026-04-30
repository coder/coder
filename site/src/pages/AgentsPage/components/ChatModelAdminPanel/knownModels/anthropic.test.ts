import { describe, expect, it } from "vitest";
import { anthropicKnownModels } from "./anthropic";
import { getKnownModelsForProvider } from "./index";

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
