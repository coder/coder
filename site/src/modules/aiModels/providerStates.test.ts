import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	canManageProviderModels,
	deriveProviderStates,
	type ProviderState,
} from "./providerStates";

const now = "2026-02-18T12:00:00.000Z";

const makeProviderConfig = (
	overrides: Partial<TypesGen.ChatProviderConfig> = {},
): TypesGen.ChatProviderConfig => ({
	id: "11111111-1111-1111-1111-111111111111",
	provider: "openai",
	display_name: "OpenAI",
	enabled: true,
	has_api_key: true,
	central_api_key_enabled: true,
	allow_user_api_key: false,
	allow_central_api_key_fallback: true,
	base_url: "",
	source: "database",
	created_at: now,
	updated_at: now,
	...overrides,
});

const makeModelConfig = (
	overrides: Partial<TypesGen.ChatModelConfig> = {},
): TypesGen.ChatModelConfig => ({
	id: "model-1",
	provider: "openai",
	model: "gpt-5",
	display_name: "GPT-5",
	enabled: true,
	is_default: false,
	context_limit: 200000,
	compression_threshold: 70,
	created_at: now,
	updated_at: now,
	...overrides,
});

const makeCatalogProvider = (
	overrides: Partial<TypesGen.ChatModelProvider> = {},
): TypesGen.ChatModelProvider => ({
	provider: "openai",
	available: true,
	models: [],
	...overrides,
});

describe("deriveProviderStates", () => {
	it("orders provider configs first, then catalog-only, then model-only providers", () => {
		const providerConfigs = [
			makeProviderConfig({
				id: "prov-anthropic",
				provider: "anthropic",
				display_name: "Anthropic",
			}),
		];
		const catalog: TypesGen.ChatModelsResponse = {
			providers: [
				makeCatalogProvider({ provider: "anthropic" }),
				makeCatalogProvider({ provider: "google" }),
			],
		};
		const modelConfigs = [
			makeModelConfig({ id: "m-vercel", provider: "vercel" }),
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
			makeProviderConfig({ id: "prov-openai", provider: "openai" }),
		];
		const modelConfigs = [
			makeModelConfig({
				id: "m1",
				provider: "openai",
				ai_provider_id: "prov-openai",
			}),
			// A model without ai_provider_id falls back to the single matching
			// provider config key.
			makeModelConfig({ id: "m2", provider: "openai" }),
		];

		const states = deriveProviderStates(modelConfigs, providerConfigs, null);

		expect(states).toHaveLength(1);
		expect(states[0].key).toBe("prov-openai");
		expect(states[0].modelConfigs.map((m) => m.id)).toEqual(["m1", "m2"]);
	});

	it("detects env-preset providers from the catalog when no config exists", () => {
		const catalog: TypesGen.ChatModelsResponse = {
			providers: [makeCatalogProvider({ provider: "openai", available: true })],
		};

		const states = deriveProviderStates([], null, catalog);

		expect(states).toHaveLength(1);
		expect(states[0].provider).toBe("openai");
		expect(states[0].isEnvPreset).toBe(true);
		expect(states[0].hasCatalogAPIKey).toBe(true);
	});

	it("flags env-preset providers via the provider config source", () => {
		const providerConfigs = [
			makeProviderConfig({
				id: "prov-openai",
				provider: "openai",
				source: "env_preset",
			}),
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
		providerConfig: makeProviderConfig(),
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
				providerConfig: makeProviderConfig({ allow_user_api_key: true }),
			}),
		).toBe(true);
	});

	it("returns false with no key and user keys disallowed", () => {
		expect(
			canManageProviderModels({
				...baseState,
				hasEffectiveAPIKey: false,
				providerConfig: makeProviderConfig({ allow_user_api_key: false }),
			}),
		).toBe(false);
	});

	it("returns false for undefined provider state", () => {
		expect(canManageProviderModels(undefined)).toBe(false);
	});
});
