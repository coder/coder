import { describe, expect, it } from "vitest";
import { getProviderDisplayName } from "./utils";

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
