import { describe, expect, it } from "vitest";
import {
	getModelOptionsFromCatalog,
	getNormalizedModelRef,
} from "./modelOptions";

describe("getNormalizedModelRef", () => {
	it("returns empty strings for malformed values", () => {
		expect(getNormalizedModelRef({ provider: undefined, model: null })).toEqual(
			{ provider: "", model: "" },
		);
	});

	it("trims and normalizes provider values", () => {
		expect(
			getNormalizedModelRef({ provider: " OpenAI ", model: " gpt-4o " }),
		).toEqual({ provider: "openai", model: "gpt-4o" });
	});
});

describe("getModelOptionsFromCatalog", () => {
	it("skips malformed configs and catalog models without crashing", () => {
		const catalog = {
			providers: [
				{
					provider: "openai",
					available: true,
					models: [
						{
							id: " valid-model ",
							provider: " OpenAI ",
							model: " gpt-4o ",
							display_name: " GPT‑4o ",
						},
						{
							id: "broken-model",
							provider: undefined,
							model: " gpt-4.1 ",
							display_name: "Broken",
						},
						{
							id: " fallback-model ",
							provider: " OpenAI ",
							model: " zz-model ",
							display_name: undefined,
						},
					],
				},
			],
		} satisfies NonNullable<Parameters<typeof getModelOptionsFromCatalog>[0]>;

		const configs = [
			{
				provider: undefined,
				model: " gpt-4o ",
				context_limit: 123,
			},
			{
				provider: " openai ",
				model: " gpt-4o ",
				context_limit: 456,
			},
			{
				provider: " openai ",
				model: " zz-model ",
				context_limit: 789,
			},
		] satisfies NonNullable<Parameters<typeof getModelOptionsFromCatalog>[1]>;

		expect(() => getModelOptionsFromCatalog(catalog, configs)).not.toThrow();
		expect(getModelOptionsFromCatalog(catalog, configs)).toEqual([
			{
				id: "valid-model",
				provider: "openai",
				model: "gpt-4o",
				displayName: "GPT‑4o",
				contextLimit: 456,
			},
			{
				id: "fallback-model",
				provider: "openai",
				model: "zz-model",
				displayName: "zz-model",
				contextLimit: 789,
			},
		]);
	});
});
