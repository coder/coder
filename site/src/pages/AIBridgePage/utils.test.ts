import { describe, expect, it } from "vitest";
import {
	formatSessionCost,
	getProviderDisplayName,
	roundTokenDisplay,
	truncatePromptPreview,
} from "./utils";

describe("truncatePromptPreview", () => {
	it("keeps prompts up to 500 characters unchanged", () => {
		const prompt = "a".repeat(500);
		expect(truncatePromptPreview(prompt)).toBe(prompt);
	});

	it("cuts longer prompts to 500 characters and adds an ellipsis", () => {
		expect(truncatePromptPreview("a".repeat(501))).toBe(`${"a".repeat(500)}…`);
	});

	it("does not split a character made of two UTF-16 code units", () => {
		const prompt = `${"a".repeat(499)}😀😀`;
		expect(truncatePromptPreview(prompt)).toBe(`${"a".repeat(499)}😀…`);
	});
});

describe("roundTokenDisplay", () => {
	it.each([
		[0, "0"],
		[950, "950"],
		[111_700, "111.7K"],
		[999_950, "1M"],
		[1_249_808, "1.2M"],
		[1_978_100, "2M"],
		[22_526_137, "22.5M"],
		[3_400_000_000, "3.4B"],
	])("formats %d as %s", (input, expected) => {
		expect(roundTokenDisplay(input)).toBe(expected);
	});
});

describe("getProviderDisplayName", () => {
	it.each([
		["anthropic", "Anthropic"],
		["openai", "OpenAI"],
		["google", "Google"],
		["azure", "Azure OpenAI"],
		["bedrock", "AWS Bedrock"],
		["copilot", "GitHub Copilot"],
		["openai-compat", "OpenAI-compatible"],
		["openrouter", "OpenRouter"],
		["vercel", "Vercel"],
	])("maps known provider %s to %s", (input, expected) => {
		expect(getProviderDisplayName(input)).toBe(expected);
	});

	it("capitalizes unknown provider names", () => {
		expect(getProviderDisplayName("custom")).toBe("Custom");
		expect(getProviderDisplayName("some-provider")).toBe("Some-provider");
	});

	it("returns Unknown for empty string", () => {
		expect(getProviderDisplayName("")).toBe("Unknown");
	});
});

describe("formatSessionCost", () => {
	it.each([
		[0, "$0.00"],
		[3_900, "<$0.01"],
		[9_999, "<$0.01"],
		[10_000, "$0.01"],
		[1_234_567, "$1.23"],
	])("formats %i micros as %s", (micros, expected) => {
		expect(formatSessionCost(micros)).toBe(expected);
	});
});
