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

describe("deriveProviderStates", () => {
	it("orders provider configs first, then catalog-only, then model-only providers", () => {
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
		};
		const modelConfigs = [
			{ ...MockChatModelConfig, id: "m-vercel", provider: "vercel" },
		];

		const states = deriveProviderStates(modelConfigs, providerConfigs, catalog);

		expect(states.map((s) => s.provider)).toEqual([
			"anthropic",
			"google",
			"vercel",
		]);
		// The config-backed provider keys on its config id, others on the name.
		expect(states[0].key).toBe("prov-anthropic");
		expect(states[1].key).toBe("google");
		expect(states[2].key).toBe("vercel");
	});

	it("matches model configs to provider configs by ai_provider_id", () => {
		const providerConfigs = [
			{ ...MockChatProviderConfig, id: "prov-openai", provider: "openai" },
		];
		const modelConfigs = [
			{
				...MockChatModelConfig,
				id: "m1",
				provider: "openai",
				ai_provider_id: "prov-openai",
			},
			// A model without ai_provider_id falls back to the single matching
			// provider config key.
			{ ...MockChatModelConfig, id: "m2", provider: "openai" },
		];

		const states = deriveProviderStates(modelConfigs, providerConfigs, null);

		expect(states).toHaveLength(1);
		expect(states[0].key).toBe("prov-openai");
		expect(states[0].modelConfigs.map((m) => m.id)).toEqual(["m1", "m2"]);
	});

	it("detects env-preset providers from the catalog when no config exists", () => {
		const catalog: TypesGen.ChatModelsResponse = {
			providers: [
				{ ...MockChatModelProvider, provider: "openai", available: true },
			],
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
	});
});

describe("canManageProviderModels", () => {
	const baseState: ProviderState = {
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
