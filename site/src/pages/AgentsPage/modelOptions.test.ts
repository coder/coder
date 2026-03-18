import type * as TypesGen from "api/typesGenerated";
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
					],
				},
			],
		} as unknown as TypesGen.ChatModelsResponse;

		const configs = [
			{
				id: "config-bad",
				provider: undefined,
				model: " gpt-4o ",
				display_name: "",
				enabled: true,
				is_default: false,
				context_limit: 123,
				compression_threshold: 0,
				created_at: "",
				updated_at: "",
			},
			{
				id: "config-good",
				provider: " openai ",
				model: " gpt-4o ",
				display_name: "",
				enabled: true,
				is_default: false,
				context_limit: 456,
				compression_threshold: 0,
				created_at: "",
				updated_at: "",
			},
		] as unknown as readonly TypesGen.ChatModelConfig[];

		expect(() => getModelOptionsFromCatalog(catalog, configs)).not.toThrow();
		expect(getModelOptionsFromCatalog(catalog, configs)).toEqual([
			{
				id: "valid-model",
				provider: "openai",
				model: "gpt-4o",
				displayName: "GPT‑4o",
				contextLimit: 456,
			},
		]);
	});
});
