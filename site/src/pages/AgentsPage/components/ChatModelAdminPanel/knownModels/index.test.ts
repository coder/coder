import { describe, expect, it } from "vitest";
import {
	findKnownModelByCanonicalId,
	formatContextBadge,
	getKnownModelsForProvider,
	searchKnownModels,
} from "./index";

const modelIds = (provider: string): readonly string[] =>
	getKnownModelsForProvider(provider).map((knownModel) => knownModel.model);

describe("formatContextBadge", () => {
	it("formats 200K context", () => {
		expect(formatContextBadge(200_000)).toBe("200K context");
	});

	it("formats 400K context", () => {
		expect(formatContextBadge(400_000)).toBe("400K context");
	});

	it("formats 1.05M context", () => {
		expect(formatContextBadge(1_050_000)).toBe("1.05M context");
	});

	it("formats values below 1K", () => {
		expect(formatContextBadge(999)).toBe("999 context");
	});

	it("rejects invalid values", () => {
		for (const invalidValue of [
			0,
			-1,
			1.5,
			Number.NaN,
			Number.POSITIVE_INFINITY,
		]) {
			expect(() => formatContextBadge(invalidValue)).toThrow(
				"contextLimit must be a positive finite integer",
			);
		}
	});
});

describe("getKnownModelsForProvider", () => {
	it("returns unsupported provider as an empty list", () => {
		expect(getKnownModelsForProvider("azure")).toEqual([]);
	});

	it("returns empty provider as an empty list", () => {
		expect(getKnownModelsForProvider("")).toEqual([]);
	});
});

describe("searchKnownModels", () => {
	it("returns provider list in display order for empty search query", () => {
		expect(
			searchKnownModels("openai", "").map((knownModel) => knownModel.model),
		).toEqual(modelIds("openai"));
	});

	it("matches canonical Model Identifier", () => {
		expect(
			searchKnownModels("openai", "gpt-5.4-mini").map(
				(knownModel) => knownModel.model,
			),
		).toEqual(["gpt-5.4-mini"]);
	});

	it("matches display name", () => {
		expect(
			searchKnownModels("openai", "codex").map(
				(knownModel) => knownModel.model,
			),
		).toEqual(["gpt-5.3-codex"]);
	});

	it("matches alias with hyphen, underscore, dot, and whitespace normalization", () => {
		expect(
			searchKnownModels("anthropic", "haiku 4_5.20251001").map(
				(knownModel) => knownModel.model,
			),
		).toEqual(["claude-haiku-4-5"]);
	});
});

describe("findKnownModelByCanonicalId", () => {
	it("returns exact canonical lookup", () => {
		expect(findKnownModelByCanonicalId("openai", "gpt-5.5")?.displayName).toBe(
			"GPT-5.5",
		);
	});

	it("does not match aliases", () => {
		expect(
			findKnownModelByCanonicalId("anthropic", "claude-haiku-4-5-20251001"),
		).toBeUndefined();
	});
});
