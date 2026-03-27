import { describe, expect, it } from "vitest";
import {
	type ModelProviderOption,
	resolveDefaultOption,
} from "./modelProviderOptions";

const options: readonly ModelProviderOption[] = [
	{
		key: "openai-primary",
		provider: "openai",
		label: "OpenAI",
		iconProvider: "openai",
	},
	{
		key: "anthropic-primary",
		provider: "anthropic",
		label: "Anthropic",
		iconProvider: "anthropic",
	},
];

describe("resolveDefaultOption", () => {
	it("returns the matching provider option when present", () => {
		expect(resolveDefaultOption(options, "anthropic")).toEqual(options[1]);
	});

	it("returns undefined when the requested provider has no option", () => {
		expect(resolveDefaultOption(options, "google")).toBeUndefined();
	});

	it("falls back to the first option when no provider is selected", () => {
		expect(resolveDefaultOption(options, null)).toEqual(options[0]);
	});
});
