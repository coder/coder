import { describe, expect, it } from "vitest";
import type { ChatModelConfig, ChatModelsResponse } from "#/api/typesGenerated";
import {
	getModelOptionsFromConfigs,
	getNormalizedModelRef,
	resolveModelOptionId,
} from "./modelOptions";

const createConfig = (
	overrides: Partial<ChatModelConfig> &
		Pick<ChatModelConfig, "id" | "provider" | "model">,
): ChatModelConfig => {
	const {
		id,
		provider,
		model,
		display_name,
		enabled = true,
		is_default = false,
		context_limit = 0,
		compression_threshold = 0,
		model_config,
		created_at = "",
		updated_at = "",
	} = overrides;

	return {
		id,
		provider,
		model,
		display_name: display_name ?? model,
		enabled,
		is_default,
		context_limit,
		compression_threshold,
		model_config,
		created_at,
		updated_at,
	};
};

const createCatalog = (
	providers: ChatModelsResponse["providers"],
): ChatModelsResponse => ({
	providers,
});

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

describe("resolveModelOptionId", () => {
	const modelOptions = [
		{
			id: "config-1",
			provider: "openai",
			model: "gpt-4o",
			displayName: "GPT-4o",
		},
		{
			id: "config-2",
			provider: "anthropic",
			model: "claude-sonnet-4-20250514",
			displayName: "Claude Sonnet",
		},
	] as const;

	it("returns an empty string for nullish and blank input", () => {
		expect(resolveModelOptionId(undefined, modelOptions)).toBe("");
		expect(resolveModelOptionId(null, modelOptions)).toBe("");
		expect(resolveModelOptionId("   ", modelOptions)).toBe("");
	});

	it("returns the config ID for a direct match", () => {
		expect(resolveModelOptionId("config-2", modelOptions)).toBe("config-2");
	});

	it("returns the config ID for a legacy provider:model match", () => {
		expect(resolveModelOptionId("openai:gpt-4o", modelOptions)).toBe(
			"config-1",
		);
	});

	it("returns an empty string when no option matches", () => {
		expect(resolveModelOptionId("openai:gpt-5", modelOptions)).toBe("");
	});

	it("returns the first duplicate legacy match deterministically", () => {
		const duplicateModelOptions = [
			...modelOptions,
			{
				id: "config-3",
				provider: "openai",
				model: "gpt-4o",
				displayName: "GPT-4o duplicate",
			},
		] as const;

		expect(resolveModelOptionId("openai:gpt-4o", duplicateModelOptions)).toBe(
			"config-1",
		);
	});
});

describe("getModelOptionsFromConfigs", () => {
	it("returns distinct options for configs with the same provider and model", () => {
		const configs = [
			createConfig({
				id: "config-1",
				provider: "openai",
				model: "gpt-4o",
				display_name: "GPT-4o (Fast)",
				context_limit: 128_000,
			}),
			createConfig({
				id: "config-2",
				provider: "openai",
				model: "gpt-4o",
				display_name: "GPT-4o (Quality)",
				context_limit: 128_000,
			}),
		];
		const catalog = createCatalog([
			{
				provider: "openai",
				available: true,
				models: [],
			},
		]);

		expect(getModelOptionsFromConfigs(configs, catalog)).toEqual([
			{
				id: "config-1",
				provider: "openai",
				model: "gpt-4o",
				displayName: "GPT-4o (Fast)",
				contextLimit: 128_000,
			},
			{
				id: "config-2",
				provider: "openai",
				model: "gpt-4o",
				displayName: "GPT-4o (Quality)",
				contextLimit: 128_000,
			},
		]);
	});

	it("excludes configs whose providers are unavailable", () => {
		const configs = [
			createConfig({
				id: "config-1",
				provider: "anthropic",
				model: "claude-sonnet-4-20250514",
				display_name: "Claude Sonnet",
				context_limit: 200_000,
			}),
		];
		const catalog = createCatalog([
			{
				provider: "anthropic",
				available: false,
				models: [],
			},
		]);

		expect(getModelOptionsFromConfigs(configs, catalog)).toEqual([]);
	});

	it("excludes disabled configs", () => {
		const configs = [
			createConfig({
				id: "config-1",
				provider: "openai",
				model: "gpt-4o",
				display_name: "GPT-4o",
				enabled: false,
				context_limit: 128_000,
			}),
			createConfig({
				id: "config-2",
				provider: "openai",
				model: "gpt-4.1",
				display_name: "GPT-4.1",
				context_limit: 128_000,
			}),
		];
		const catalog = createCatalog([
			{
				provider: "openai",
				available: true,
				models: [],
			},
		]);

		expect(getModelOptionsFromConfigs(configs, catalog)).toEqual([
			{
				id: "config-2",
				provider: "openai",
				model: "gpt-4.1",
				displayName: "GPT-4.1",
				contextLimit: 128_000,
			},
		]);
	});

	it("falls back to the model name when display_name is blank", () => {
		const configs = [
			createConfig({
				id: "config-1",
				provider: " openai ",
				model: " gpt-4o ",
				display_name: " ",
				context_limit: 0,
			}),
		];
		const catalog = createCatalog([
			{
				provider: "openai",
				available: true,
				models: [],
			},
		]);

		expect(getModelOptionsFromConfigs(configs, catalog)).toEqual([
			{
				id: "config-1",
				provider: "openai",
				model: "gpt-4o",
				displayName: "gpt-4o",
				contextLimit: 0,
			},
		]);
	});

	it("returns an empty array for null and undefined inputs", () => {
		expect(getModelOptionsFromConfigs(null, null)).toEqual([]);
		expect(getModelOptionsFromConfigs(undefined, undefined)).toEqual([]);
	});

	it("sorts options by provider and display name", () => {
		const configs = [
			createConfig({
				id: "config-openai-zeta",
				provider: "openai",
				model: "gpt-z",
				display_name: "Zeta",
				context_limit: 32_000,
			}),
			createConfig({
				id: "config-anthropic",
				provider: "anthropic",
				model: "claude-sonnet-4-20250514",
				display_name: "Claude Sonnet",
				context_limit: 200_000,
			}),
			createConfig({
				id: "config-openai-alpha",
				provider: "openai",
				model: "gpt-a",
				display_name: "Alpha",
				context_limit: 32_000,
			}),
		];
		const catalog = createCatalog([
			{
				provider: "openai",
				available: true,
				models: [],
			},
			{
				provider: "anthropic",
				available: true,
				models: [],
			},
		]);

		expect(
			getModelOptionsFromConfigs(configs, catalog).map((option) => option.id),
		).toEqual([
			"config-anthropic",
			"config-openai-alpha",
			"config-openai-zeta",
		]);
	});

	it("keeps canonical wrapper-provider model strings distinct", () => {
		const configs = [
			createConfig({
				id: "config-1",
				provider: "openrouter",
				model: "openai/gpt-4o",
				display_name: "GPT-4o via OpenRouter",
				context_limit: 128_000,
			}),
			createConfig({
				id: "config-2",
				provider: "openrouter",
				model: "anthropic/claude-sonnet-4-20250514",
				display_name: "Claude via OpenRouter",
				context_limit: 200_000,
			}),
		];
		const catalog = createCatalog([
			{
				provider: "openrouter",
				available: true,
				models: [],
			},
		]);

		expect(getModelOptionsFromConfigs(configs, catalog)).toEqual([
			{
				id: "config-2",
				provider: "openrouter",
				model: "anthropic/claude-sonnet-4-20250514",
				displayName: "Claude via OpenRouter",
				contextLimit: 200_000,
			},
			{
				id: "config-1",
				provider: "openrouter",
				model: "openai/gpt-4o",
				displayName: "GPT-4o via OpenRouter",
				contextLimit: 128_000,
			},
		]);
	});
});
