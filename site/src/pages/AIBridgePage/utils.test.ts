import { describe, expect, it } from "vitest";
import type { AIBridgeProvider } from "#/api/typesGenerated";
import { MockAIBridgeProvider } from "#/testHelpers/entities";
import {
	getProviderDisplayName,
	getProviderNameLabel,
	getProvidersByName,
	roundTokenDisplay,
} from "./utils";

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

describe("getProviderNameLabel", () => {
	const mockUnnamedProvider: AIBridgeProvider = {
		...MockAIBridgeProvider,
		name: "internal",
		display_name: "",
	};
	const mockProvidersByName = getProvidersByName([
		MockAIBridgeProvider,
		mockUnnamedProvider,
	]);

	it.each([
		["claude-platform", "Claude Platform"],
		["internal", "internal"],
		["anthropic", "Anthropic"],
	])("labels %s as %s", (input, expected) => {
		expect(getProviderNameLabel(input, mockProvidersByName)).toBe(expected);
	});

	it("labels names as types without the provider list", () => {
		expect(getProviderNameLabel("openai", undefined)).toBe("OpenAI");
	});
});
