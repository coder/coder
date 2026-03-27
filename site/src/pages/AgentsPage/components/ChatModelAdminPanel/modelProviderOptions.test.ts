import { describe, expect, it } from "vitest";
import type { ChatProviderConfig } from "#/api/typesGenerated";
import type { ProviderState } from "./ChatModelAdminPanel";
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

describe("buildModelProviderOptions", () => {
	it("treats undefined source as a database config", () => {
		const options = buildModelProviderOptions([
			providerState({
				hasEffectiveAPIKey: true,
				providerConfigs: [
					providerConfig({
						id: "provider-config-undefined-source",
						display_name: "OpenAI Legacy",
						source: undefined,
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
			},
		]);
	});

	it("includes enabled database configs without a stored API key", () => {
		const options = buildModelProviderOptions([
			providerState({
				hasEffectiveAPIKey: true,
				providerConfigs: [
					providerConfig({
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
