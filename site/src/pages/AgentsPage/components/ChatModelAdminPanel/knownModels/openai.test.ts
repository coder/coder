import { describe, expect, it } from "vitest";
import { getKnownModelsForProvider } from "./index";
import { openAIKnownModels } from "./openai";

describe("openAIKnownModels", () => {
	it("returns OpenAI canonical IDs in declared order", () => {
		expect(
			getKnownModelsForProvider("openai").map(
				(knownModel) => knownModel.modelIdentifier,
			),
		).toEqual([
			"gpt-5.5",
			"gpt-5.5-pro",
			"gpt-5.4",
			"gpt-5.4-mini",
			"gpt-5.4-nano",
			"gpt-5.3-codex",
		]);
	});

	it("has source metadata, provider equality, and declared order", () => {
		expect(
			openAIKnownModels.map((knownModel) => knownModel.modelIdentifier),
		).toEqual([
			"gpt-5.5",
			"gpt-5.5-pro",
			"gpt-5.4",
			"gpt-5.4-mini",
			"gpt-5.4-nano",
			"gpt-5.3-codex",
		]);

		for (const knownModel of openAIKnownModels) {
			expect(knownModel.provider).toBe("openai");
			expect(knownModel.sourceMetadata.sourceName).toBe("models.dev");
			expect(knownModel.sourceMetadata.sourceRetrievedAt).toBe("2026-04-30");
			expect(knownModel.sourceMetadata.lastUpdated).not.toBe("");
		}
	});
});
