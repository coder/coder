import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatModelConfig,
	MockChatModelProvider,
	MockChatProviderConfig,
} from "#/testHelpers/chatModels";
import {
	canManageProviderModels,
	deriveProviderStates,
	type ProviderState,
} from "./providerStates";

const baseProviderState: ProviderState = {
	key: "prov-openai",
	provider: "openai",
	label: "OpenAI",
	providerConfig: MockChatProviderConfig,
	modelConfigs: [],
	catalogModelCount: 0,
	hasManagedAPIKey: true,
	hasCatalogAPIKey: false,
	hasEffectiveAPIKey: true,
	allowUserAPIKey: false,
	isEnvPreset: false,
	baseURL: "",
};

describe("deriveProviderStates", () => {
	it("orders provider configs first, then catalog-only providers", () => {
		const providerConfigs = [
			{
				...MockChatProviderConfig,
				id: "prov-anthropic",
				provider: "anthropic",
				display_name: "Anthropic",
			},
		];
		const catalog: TypesGen.ChatModelsResponse = {
			providers: [
				{ ...MockChatModelProvider, provider: "anthropic" },
				{ ...MockChatModelProvider, provider: "google" },
			],
			unsupported_providers: [],
		};

		const states = deriveProviderStates([], providerConfigs, catalog);

		expect(states.map((s) => s.provider)).toEqual(["anthropic", "google"]);
		expect(states[0].key).toBe("prov-anthropic");
		expect(states[1].key).toBe("google");
		expect(states[0].hasEffectiveAPIKey).toBe(true);
		expect(states[1].hasEffectiveAPIKey).toBe(true);
	});

	it("matches model configs to provider configs by ai_provider_id", () => {
		const providerConfigs = [
			{ ...MockChatProviderConfig, id: "prov-openai", provider: "openai" },
		];
		const modelConfigs = [
			{
				...MockChatModelConfig,
				id: "m1",
				ai_provider_id: "prov-openai",
			},
			{ ...MockChatModelConfig, id: "m2", ai_provider_id: "prov-openai" },
		];

		const states = deriveProviderStates(modelConfigs, providerConfigs, null);

		expect(states).toHaveLength(1);
		expect(states[0].key).toBe("prov-openai");
		expect(states[0].modelConfigs.map((m) => m.id)).toEqual(["m1", "m2"]);
	});

	it("treats bedrock with central_api_key_enabled as having an effective key", () => {
		const providerConfigs = [
			{
				...MockChatProviderConfig,
				id: "prov-bedrock",
				provider: "bedrock",
				has_api_key: false,
				central_api_key_enabled: true,
			},
		];

		const states = deriveProviderStates([], providerConfigs, null);

		expect(states[0].hasEffectiveAPIKey).toBe(true);
	});

	it("drops models without ai_provider_id", () => {
		const providerConfigs = [
			{ ...MockChatProviderConfig, id: "prov-a", provider: "openai" },
			{ ...MockChatProviderConfig, id: "prov-b", provider: "openai" },
		];
		const modelConfigs = [
			{ ...MockChatModelConfig, id: "m1", ai_provider_id: "" },
		];

		const states = deriveProviderStates(modelConfigs, providerConfigs, null);

		expect(states.flatMap((s) => s.modelConfigs)).toHaveLength(0);
	});

	it("detects env-preset providers from the catalog when no config exists", () => {
		const catalog: TypesGen.ChatModelsResponse = {
			providers: [
				{ ...MockChatModelProvider, provider: "openai", available: true },
			],
			unsupported_providers: [],
		};

		const states = deriveProviderStates([], null, catalog);

		expect(states).toHaveLength(1);
		expect(states[0].provider).toBe("openai");
		expect(states[0].isEnvPreset).toBe(true);
		expect(states[0].hasCatalogAPIKey).toBe(true);
	});

	it("flags env-preset providers via the provider config source", () => {
		const providerConfigs = [
			{
				...MockChatProviderConfig,
				id: "prov-openai",
				provider: "openai",
				source: "env_preset" as const,
			},
		];

		const states = deriveProviderStates([], providerConfigs, null);

		expect(states[0].isEnvPreset).toBe(true);
		expect(states[0].providerConfig).toBeUndefined();
	});

	it("treats an unavailable catalog provider as having a key unless the api key is missing", () => {
		const catalog: TypesGen.ChatModelsResponse = {
			providers: [
				{
					...MockChatModelProvider,
					provider: "openai",
					available: false,
					unavailable_reason: "fetch_failed",
				},
			],
			unsupported_providers: [],
		};

		const states = deriveProviderStates([], null, catalog);

		expect(states[0].hasCatalogAPIKey).toBe(true);
	});

	it("derives label, catalogModelCount, and baseURL from the inputs", () => {
		const providerConfigs = [
			{
				...MockChatProviderConfig,
				id: "prov-openai",
				provider: "openai",
				display_name: "Custom OpenAI",
				base_url: "https://custom.example.com/v1",
			},
		];
		const catalog: TypesGen.ChatModelsResponse = {
			providers: [
				{
					...MockChatModelProvider,
					provider: "openai",
					models: [
						{
							id: "gpt-x",
							provider: "openai",
							model: "gpt-x",
							display_name: "GPT-X",
						},
					],
				},
			],
			unsupported_providers: [],
		};

		const states = deriveProviderStates([], providerConfigs, catalog);

		expect(states[0].label).toBe("Custom OpenAI");
		expect(states[0].catalogModelCount).toBe(1);
		expect(states[0].baseURL).toBe("https://custom.example.com/v1");
	});
});

describe("canManageProviderModels", () => {
	const baseState = baseProviderState;

	it("returns false without a managed provider config", () => {
		expect(
			canManageProviderModels({ ...baseState, providerConfig: undefined }),
		).toBe(false);
	});

	it("returns true when the provider has an effective API key", () => {
		expect(canManageProviderModels(baseState)).toBe(true);
	});

	it("returns true when user-supplied API keys are allowed", () => {
		expect(
			canManageProviderModels({
				...baseState,
				hasEffectiveAPIKey: false,
				providerConfig: { ...MockChatProviderConfig, allow_user_api_key: true },
			}),
		).toBe(true);
	});

	it("returns false with no key and user keys disallowed", () => {
		expect(
			canManageProviderModels({
				...baseState,
				hasEffectiveAPIKey: false,
				providerConfig: {
					...MockChatProviderConfig,
					allow_user_api_key: false,
				},
			}),
		).toBe(false);
	});

	it("returns false for undefined provider state", () => {
		expect(canManageProviderModels(undefined)).toBe(false);
	});
});
