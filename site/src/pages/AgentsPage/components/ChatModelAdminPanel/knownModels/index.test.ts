import { describe, expect, it } from "vitest";
import {
	findKnownModelByCanonicalId,
	findKnownModelByExactAlias,
	formatContextBadge,
	formatPricePerMillionTokens,
	getKnownModelsForProvider,
	searchKnownModels,
} from "./index";

const modelIds = (provider: string): readonly string[] =>
	getKnownModelsForProvider(provider).map(
		(knownModel) => knownModel.modelIdentifier,
	);

describe("formatContextBadge", () => {
	it("formats 200K context", () => {
		expect(formatContextBadge(200_000)).toBe("200K context");
	});

	it("formats 400K context", () => {
		expect(formatContextBadge(400_000)).toBe("400K context");
	});

	it("formats 1M context without trailing decimals", () => {
		expect(formatContextBadge(1_000_000)).toBe("1M context");
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
		expect(getKnownModelsForProvider("openai-compat")).toEqual([]);
	});

	it("returns empty provider as an empty list", () => {
		expect(getKnownModelsForProvider("")).toEqual([]);
	});
});

describe("searchKnownModels", () => {
	it("returns provider list in display order for empty search query", () => {
		expect(
			searchKnownModels("openai", "").map(
				(knownModel) => knownModel.modelIdentifier,
			),
		).toEqual(modelIds("openai"));
	});

	it("matches canonical Model Identifier", () => {
		expect(
			searchKnownModels("openai", "gpt-5.4-mini").map(
				(knownModel) => knownModel.modelIdentifier,
			),
		).toEqual(["gpt-5.4-mini"]);
	});

	it("matches display name", () => {
		expect(
			searchKnownModels("openai", "codex").map(
				(knownModel) => knownModel.modelIdentifier,
			),
		).toEqual(["gpt-5.3-codex"]);
	});

	it("matches alias with hyphen, underscore, dot, and whitespace normalization", () => {
		expect(
			searchKnownModels("anthropic", "haiku 4_5.20251001").map(
				(knownModel) => knownModel.modelIdentifier,
			),
		).toEqual(["claude-haiku-4-5"]);
	});
});

describe("findKnownModelByExactAlias", () => {
	it("returns verbatim alias lookup case-insensitively", () => {
		expect(
			findKnownModelByExactAlias("anthropic", "CLAUDE-HAIKU-4-5-20251001")
				?.modelIdentifier,
		).toBe("claude-haiku-4-5");
	});

	it("does not normalize punctuation differences", () => {
		expect(
			findKnownModelByExactAlias("anthropic", "claude.haiku.4.5.20251001"),
		).toBeUndefined();
	});

	it("does not match alias substrings", () => {
		expect(findKnownModelByExactAlias("anthropic", "haiku")).toBeUndefined();
	});

	it("does not match unknown strings", () => {
		expect(
			findKnownModelByExactAlias("anthropic", "unknown-model"),
		).toBeUndefined();
	});

	it("does not match canonical Model Identifiers", () => {
		expect(
			findKnownModelByExactAlias("anthropic", "claude-haiku-4-5"),
		).toBeUndefined();
	});
});

describe("formatPricePerMillionTokens", () => {
	it("formats whole-dollar prices", () => {
		expect(formatPricePerMillionTokens(10)).toBe("$10");
	});

	it("formats fractional prices without dropping precision", () => {
		expect(formatPricePerMillionTokens(1.25)).toBe("$1.25");
		expect(formatPricePerMillionTokens(0.1)).toBe("$0.10");
		expect(formatPricePerMillionTokens(0.3)).toBe("$0.30");
	});

	it("keeps sub-cent prices visible", () => {
		expect(formatPricePerMillionTokens(0.075)).toBe("$0.075");
		expect(formatPricePerMillionTokens(0.003625)).toBe("$0.0036");
		expect(formatPricePerMillionTokens(0.125)).toBe("$0.125");
	});

	it("shows a threshold for positive prices below four decimals", () => {
		expect(formatPricePerMillionTokens(0.000001)).toBe("<$0.0001");
		expect(formatPricePerMillionTokens(0.000049)).toBe("<$0.0001");
		expect(formatPricePerMillionTokens(0.0001)).toBe("$0.0001");
	});

	it("formats zero", () => {
		expect(formatPricePerMillionTokens(0)).toBe("$0");
	});

	it("rejects non-finite values", () => {
		for (const invalidValue of [Number.NaN, Number.POSITIVE_INFINITY]) {
			expect(() => formatPricePerMillionTokens(invalidValue)).toThrow(
				"price must be a finite number",
			);
		}
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
