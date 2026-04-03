import { describe, expect, it } from "vitest";
import type { ChatProviderConfig } from "#/api/typesGenerated";
import type { ProviderState } from "./ChatModelAdminPanel";
import { isDatabaseProviderConfig, NIL_UUID } from "./helpers";
import {
	buildModelProviderOptions,
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

const providerConfig = (
	overrides: Partial<ChatProviderConfig>,
): ChatProviderConfig => ({
	id: "provider-config-id",
	provider: "openai",
	display_name: "OpenAI Primary",
	enabled: true,
	has_api_key: true,
	base_url: "",
	source: "database",
	created_at: "2025-01-01T00:00:00Z",
	updated_at: "2025-01-01T00:00:00Z",
	...overrides,
	has_effective_api_key:
		overrides.has_effective_api_key ?? overrides.has_api_key ?? true,
	central_api_key_enabled: overrides.central_api_key_enabled ?? true,
	allow_user_api_key: overrides.allow_user_api_key ?? false,
	allow_central_api_key_fallback:
		overrides.allow_central_api_key_fallback ?? false,
});

const providerState = (overrides: Partial<ProviderState>): ProviderState => ({
	provider: "openai",
	label: "OpenAI",
	providerConfigs: [],
	modelConfigs: [],
	catalogModelCount: 0,
	hasManagedAPIKey: false,
	hasCatalogAPIKey: false,
	hasEffectiveAPIKey: false,
	isEnvPreset: false,
	baseURL: "",
	...overrides,
});

const providerConfigWithoutSource = (
	overrides: Partial<ChatProviderConfig> = {},
): ChatProviderConfig => {
	const config = providerConfig(overrides);
	Reflect.deleteProperty(config, "source");
	return config;
};

describe("isDatabaseProviderConfig", () => {
	it("returns true only for database-backed configs", () => {
		expect(isDatabaseProviderConfig(providerConfigWithoutSource())).toBe(true);
		expect(
			isDatabaseProviderConfig(providerConfig({ source: "env_preset" })),
		).toBe(false);
		expect(
			isDatabaseProviderConfig(providerConfigWithoutSource({ id: NIL_UUID })),
		).toBe(false);
	});
});

describe("buildModelProviderOptions", () => {
	it("treats undefined source as a database config", () => {
		const options = buildModelProviderOptions([
			providerState({
				hasEffectiveAPIKey: true,
				providerConfigs: [
					providerConfigWithoutSource({
						id: "provider-config-undefined-source",
						display_name: "OpenAI Legacy",
					}),
				],
			}),
		]);

		expect(options).toEqual([
			{
				key: "provider-config-undefined-source",
				provider: "openai",
				label: "OpenAI Legacy",
				iconProvider: "openai",
				configId: "provider-config-undefined-source",
			},
		]);
	});

	it("includes enabled database configs without a stored API key", () => {
		const options = buildModelProviderOptions([
			providerState({
				hasEffectiveAPIKey: true,
				providerConfigs: [
					providerConfigWithoutSource({
						id: "provider-config-env-api-key",
						display_name: "OpenAI Env Key",
						has_api_key: false,
					}),
				],
			}),
		]);

		expect(options).toEqual([
			{
				key: "provider-config-env-api-key",
				provider: "openai",
				label: "OpenAI Env Key",
				iconProvider: "openai",
				configId: "provider-config-env-api-key",
			},
		]);
	});

	it("falls back to an env preset option when only disabled database configs exist", () => {
		const options = buildModelProviderOptions([
			providerState({
				provider: "openai",
				label: "OpenAI",
				hasEffectiveAPIKey: true,
				isEnvPreset: true,
				providerConfigs: [
					providerConfigWithoutSource({
						id: "openai-disabled-config",
						display_name: "OpenAI Disabled",
						enabled: false,
						has_api_key: false,
						has_effective_api_key: false,
					}),
				],
			}),
		]);

		expect(options).toEqual([
			{
				key: "env:openai",
				provider: "openai",
				label: "OpenAI",
				iconProvider: "openai",
			},
		]);
	});

	it("does not attach configId to env preset options", () => {
		const options = buildModelProviderOptions([
			providerState({
				provider: "anthropic",
				label: "Anthropic",
				hasEffectiveAPIKey: true,
				isEnvPreset: true,
			}),
		]);

		expect(options).toEqual([
			{
				key: "env:anthropic",
				provider: "anthropic",
				label: "Anthropic",
				iconProvider: "anthropic",
			},
		]);
		expect(options[0]?.configId).toBeUndefined();
	});

	it("excludes disabled database configs from add-model options", () => {
		const options = buildModelProviderOptions([
			providerState({
				hasEffectiveAPIKey: true,
				providerConfigs: [
					providerConfigWithoutSource({
						id: "openai-enabled",
						display_name: "OpenAI Enabled",
						enabled: true,
					}),
					providerConfigWithoutSource({
						id: "openai-disabled",
						display_name: "OpenAI Disabled",
						enabled: false,
						has_api_key: true,
						has_effective_api_key: true,
					}),
				],
			}),
		]);

		expect(options).toEqual([
			{
				key: "openai-enabled",
				provider: "openai",
				label: "OpenAI Enabled",
				iconProvider: "openai",
				configId: "openai-enabled",
			},
		]);
	});

	it("keeps separate options for multiple enabled configs in one family", () => {
		const options = buildModelProviderOptions([
			providerState({
				hasEffectiveAPIKey: true,
				providerConfigs: [
					providerConfigWithoutSource({
						id: "openai-first",
						display_name: "",
					}),
					providerConfigWithoutSource({
						id: "openai-second",
						display_name: "",
					}),
				],
			}),
		]);

		expect(options).toEqual([
			{
				key: "openai-first",
				provider: "openai",
				label: "OpenAI 1",
				iconProvider: "openai",
				configId: "openai-first",
			},
			{
				key: "openai-second",
				provider: "openai",
				label: "OpenAI 2",
				iconProvider: "openai",
				configId: "openai-second",
			},
		]);
	});
});

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
