import { describe, expect, it } from "vitest";
import type {
	ChatModelConfig,
	ChatModelsResponse,
	ChatProviderConfig,
} from "#/api/typesGenerated";
import {
	MockChatModelConfig,
	MockChatProviderConfig,
} from "#/testHelpers/chatModels";
import {
	countConfiguredProviderConfigs,
	filterConfigsWithEnabledProvider,
	formatProviderLabel,
	getModelOptionsFromConfigs,
	getModelSelectorPlaceholder,
	getUnsupportedProviderNames,
	hasConfiguredProviderConfigs,
	hasUserFixableProviders,
	providerInfoByIDFromConfigs,
	providerInfoByIDFromUserConfigs,
	providerTypeByIDFromConfigs,
	providerTypeByIDFromUserConfigs,
	resolveModelOptionId,
	resolveModelSelector,
} from "./modelOptions";

const createConfig = (
	overrides: Partial<ChatModelConfig> &
		Pick<ChatModelConfig, "id" | "ai_provider_id" | "model">,
): ChatModelConfig => ({
	...MockChatModelConfig,
	context_limit: 0,
	compression_threshold: 0,
	created_at: "",
	updated_at: "",
	...overrides,
});

const providerInfoByID = new Map([
	["prov-openai", { provider: "openai", displayName: "OpenAI", icon: "" }],
	[
		"prov-anthropic",
		{ provider: "anthropic", displayName: "Anthropic", icon: "" },
	],
	[
		"prov-openrouter",
		{ provider: "openrouter", displayName: "OpenRouter", icon: "" },
	],
]);

const createCatalog = (
	providers: ChatModelsResponse["providers"],
	unsupportedProviders: ChatModelsResponse["unsupported_providers"] = [],
): ChatModelsResponse => ({
	providers,
	unsupported_providers: unsupportedProviders,
});

const createProviderConfig = (
	overrides: Pick<ChatProviderConfig, "provider" | "source"> &
		Partial<ChatProviderConfig>,
): ChatProviderConfig => ({
	...MockChatProviderConfig,
	id: "provider-config-1",
	display_name: overrides.provider,
	has_api_key: false,
	central_api_key_enabled: true,
	allow_user_api_key: false,
	allow_central_api_key_fallback: false,
	...overrides,
});

describe("hasUserFixableProviders", () => {
	it("returns true when a provider needs a user API key", () => {
		const catalog = createCatalog([
			{
				provider: "openai",
				available: false,
				unavailable_reason: "user_api_key_required",
				models: [],
			},
		]);

		expect(hasUserFixableProviders(catalog)).toBe(true);
	});

	it("returns false when providers are only admin-fixable", () => {
		const catalog = createCatalog([
			{
				provider: "openai",
				available: false,
				unavailable_reason: "missing_api_key",
				models: [],
			},
		]);

		expect(hasUserFixableProviders(catalog)).toBe(false);
	});
});

describe("hasConfiguredProviderConfigs", () => {
	it("ignores supported provider placeholders", () => {
		const catalog = createCatalog([
			{ provider: "openai", available: true, models: [] },
		]);

		expect(
			hasConfiguredProviderConfigs(
				[createProviderConfig({ provider: "openai", source: "supported" })],
				catalog,
			),
		).toBe(false);
	});

	it("returns true for database and env preset provider configs", () => {
		const catalog = createCatalog([
			{ provider: "openai", available: true, models: [] },
		]);

		expect(
			hasConfiguredProviderConfigs(
				[createProviderConfig({ provider: "openai", source: "database" })],
				catalog,
			),
		).toBe(true);
		expect(
			hasConfiguredProviderConfigs(
				[createProviderConfig({ provider: "openai", source: "env_preset" })],
				catalog,
			),
		).toBe(true);
	});

	it("excludes disabled and unavailable provider configs", () => {
		const catalog = createCatalog([
			{ provider: "openai", available: true, models: [] },
			{
				provider: "anthropic",
				available: false,
				unavailable_reason: "missing_api_key",
				models: [],
			},
		]);

		expect(
			hasConfiguredProviderConfigs(
				[
					createProviderConfig({
						provider: "openai",
						source: "database",
						enabled: false,
					}),
					createProviderConfig({
						provider: "anthropic",
						source: "database",
					}),
				],
				catalog,
			),
		).toBe(false);
	});
});

describe("countConfiguredProviderConfigs", () => {
	it("counts only enabled provider configs available in the catalog", () => {
		const catalog = createCatalog([
			{ provider: "openai", available: true, models: [] },
			{ provider: "anthropic", available: true, models: [] },
			{ provider: "google", available: true, models: [] },
			{ provider: "azure", available: true, models: [] },
			{
				provider: "bedrock",
				available: false,
				unavailable_reason: "missing_api_key",
				models: [],
			},
		]);

		expect(
			countConfiguredProviderConfigs(
				[
					createProviderConfig({ provider: "openai", source: "database" }),
					createProviderConfig({ provider: "anthropic", source: "env_preset" }),
					createProviderConfig({ provider: "google", source: "supported" }),
					createProviderConfig({
						provider: "azure",
						source: "database",
						enabled: false,
					}),
					createProviderConfig({ provider: "bedrock", source: "database" }),
				],
				catalog,
			),
		).toBe(2);
	});

	it("returns zero while provider availability is unknown", () => {
		expect(
			countConfiguredProviderConfigs(
				[createProviderConfig({ provider: "openai", source: "database" })],
				undefined,
			),
		).toBe(0);
	});
});

describe("formatProviderLabel", () => {
	it("formats OpenAI compatible providers", () => {
		expect(formatProviderLabel("openai-compat")).toBe("OpenAI-compatible");
		expect(formatProviderLabel("openai-compatible")).toBe("OpenAI-compatible");
	});
});

describe("getModelSelectorPlaceholder", () => {
	it("returns user guidance when only user-configured keys are missing", () => {
		const catalog = createCatalog([
			{
				provider: "openai",
				available: false,
				unavailable_reason: "user_api_key_required",
				models: [],
			},
		]);

		expect(getModelSelectorPlaceholder([], false, true, catalog)).toBe(
			"Configure API Keys",
		);
	});

	it("keeps the generic unavailable placeholder for admin fixes", () => {
		const catalog = createCatalog([
			{
				provider: "openai",
				available: false,
				unavailable_reason: "missing_api_key",
				models: [],
			},
		]);

		expect(getModelSelectorPlaceholder([], false, true, catalog)).toBe(
			"No Models Available",
		);
	});
});

describe("resolveModelOptionId", () => {
	const modelOptions = [
		{
			id: "config-1",
			provider: "openai",
			providerId: "prov-openai",
			providerLabel: "OpenAI",
			providerIcon: "",
			model: "gpt-4o",
			displayName: "GPT-4o",
		},
		{
			id: "config-2",
			provider: "anthropic",
			providerId: "prov-anthropic",
			providerLabel: "Anthropic",
			providerIcon: "",
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

	it("returns an empty string when no option matches", () => {
		expect(resolveModelOptionId("openai:gpt-5", modelOptions)).toBe("");
	});
});

describe("getModelOptionsFromConfigs", () => {
	it("returns distinct options for configs with the same provider and model", () => {
		const configs = [
			createConfig({
				id: "config-1",
				ai_provider_id: "prov-openai",
				model: "gpt-4o",
				display_name: "GPT-4o (Fast)",
				context_limit: 128_000,
			}),
			createConfig({
				id: "config-2",
				ai_provider_id: "prov-openai",
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

		expect(
			getModelOptionsFromConfigs(configs, catalog, providerInfoByID),
		).toEqual([
			{
				id: "config-1",
				provider: "openai",
				providerId: "prov-openai",
				providerLabel: "OpenAI",
				providerIcon: "",
				model: "gpt-4o",
				displayName: "GPT-4o (Fast)",
				contextLimit: 128_000,
			},
			expect.objectContaining({ id: "config-2" }),
		]);
	});

	it("populates reasoning effort bounds from the model config", () => {
		const configs = [
			createConfig({
				id: "config-effort",
				ai_provider_id: "prov-openai",
				model: "gpt-5",
				display_name: "GPT-5",
				model_config: {
					reasoning_effort: { default: "medium", max: "xhigh" },
				},
				reasoning_efforts: ["minimal", "low", "medium", "high", "xhigh"],
			}),
			createConfig({
				id: "config-no-effort",
				ai_provider_id: "prov-openai",
				model: "gpt-4o",
				display_name: "GPT-4o",
				model_config: {},
			}),
		];
		const catalog = createCatalog([
			{ provider: "openai", available: true, models: [] },
		]);

		expect(
			getModelOptionsFromConfigs(configs, catalog, providerInfoByID),
		).toEqual([
			{
				id: "config-no-effort",
				provider: "openai",
				providerId: "prov-openai",
				providerLabel: "OpenAI",
				providerIcon: "",
				model: "gpt-4o",
				displayName: "GPT-4o",
				contextLimit: 0,
			},
			{
				id: "config-effort",
				provider: "openai",
				providerId: "prov-openai",
				providerLabel: "OpenAI",
				providerIcon: "",
				model: "gpt-5",
				displayName: "GPT-5",
				contextLimit: 0,
				reasoningEffortDefault: "medium",
				reasoningEfforts: ["minimal", "low", "medium", "high", "xhigh"],
			},
		]);
	});

	it("excludes configs whose providers are unavailable", () => {
		const configs = [
			createConfig({
				id: "config-1",
				ai_provider_id: "prov-anthropic",
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

		expect(
			getModelOptionsFromConfigs(configs, catalog, providerInfoByID),
		).toEqual([]);
	});

	it("excludes disabled configs", () => {
		const configs = [
			createConfig({
				id: "config-1",
				ai_provider_id: "prov-openai",
				model: "gpt-4o",
				display_name: "GPT-4o",
				enabled: false,
				context_limit: 128_000,
			}),
			createConfig({
				id: "config-2",
				ai_provider_id: "prov-openai",
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

		expect(
			getModelOptionsFromConfigs(configs, catalog, providerInfoByID).map(
				(option) => option.id,
			),
		).toEqual(["config-2"]);
	});

	it("falls back to the model name when display_name is blank", () => {
		const configs = [
			createConfig({
				id: "config-1",
				ai_provider_id: "prov-openai",
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

		expect(
			getModelOptionsFromConfigs(configs, catalog, providerInfoByID),
		).toEqual([
			expect.objectContaining({
				id: "config-1",
				model: "gpt-4o",
				displayName: "gpt-4o",
			}),
		]);
	});

	it("returns an empty array for null and undefined inputs", () => {
		expect(getModelOptionsFromConfigs(null, null, providerInfoByID)).toEqual(
			[],
		);
		expect(
			getModelOptionsFromConfigs(undefined, undefined, providerInfoByID),
		).toEqual([]);
	});

	it("sorts options by provider and display name", () => {
		const configs = [
			createConfig({
				id: "config-openai-zeta",
				ai_provider_id: "prov-openai",
				model: "gpt-z",
				display_name: "Zeta",
				context_limit: 32_000,
			}),
			createConfig({
				id: "config-anthropic",
				ai_provider_id: "prov-anthropic",
				model: "claude-sonnet-4-20250514",
				display_name: "Claude Sonnet",
				context_limit: 200_000,
			}),
			createConfig({
				id: "config-openai-alpha",
				ai_provider_id: "prov-openai",
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
			getModelOptionsFromConfigs(configs, catalog, providerInfoByID).map(
				(option) => option.id,
			),
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
				ai_provider_id: "prov-openrouter",
				model: "openai/gpt-4o",
				display_name: "GPT-4o via OpenRouter",
				context_limit: 128_000,
			}),
			createConfig({
				id: "config-2",
				ai_provider_id: "prov-openrouter",
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

		expect(
			getModelOptionsFromConfigs(configs, catalog, providerInfoByID).map(
				(option) => option.id,
			),
		).toEqual(["config-2", "config-1"]);
	});

	it("drops configs whose ai_provider_id is absent from the provider map", () => {
		const configs = [
			createConfig({
				id: "config-1",
				ai_provider_id: "prov-openai",
				model: "gpt-4o",
				display_name: "GPT-4o",
				context_limit: 128_000,
			}),
		];
		const catalog = createCatalog([
			{ provider: "openai", available: true, models: [] },
		]);

		expect(getModelOptionsFromConfigs(configs, catalog, new Map())).toEqual([]);
	});

	it("keeps only configs whose ai_provider_id resolves in the provider map", () => {
		const configs = [
			createConfig({
				id: "config-openai",
				ai_provider_id: "prov-openai",
				model: "gpt-4o",
				display_name: "GPT-4o",
				context_limit: 128_000,
			}),
			createConfig({
				id: "config-orphan",
				ai_provider_id: "prov-missing",
				model: "claude-sonnet-4-20250514",
				display_name: "Claude Sonnet",
				context_limit: 200_000,
			}),
		];
		const catalog = createCatalog([
			{ provider: "openai", available: true, models: [] },
			{ provider: "anthropic", available: true, models: [] },
		]);
		const partialMap = new Map([
			["prov-openai", { provider: "openai", displayName: "OpenAI", icon: "" }],
		]);

		expect(
			getModelOptionsFromConfigs(configs, catalog, partialMap).map(
				(option) => option.id,
			),
		).toEqual(["config-openai"]);
	});

	it("preserves provider instance metadata for same-type providers", () => {
		const configs = [
			createConfig({
				id: "config-primary",
				ai_provider_id: "prov-anthropic-primary",
				model: "claude-sonnet-4-20250514",
			}),
			createConfig({
				id: "config-hyper",
				ai_provider_id: "prov-anthropic-hyper",
				model: "claude-opus-4-20250514",
			}),
		];
		const catalog = createCatalog([
			{ provider: "anthropic", available: true, models: [] },
		]);
		const sameTypeProviders = new Map([
			[
				"prov-anthropic-primary",
				{ provider: "anthropic", displayName: "Anthropic", icon: "" },
			],
			[
				"prov-anthropic-hyper",
				{
					provider: "anthropic",
					displayName: "Hyper",
					icon: "/icon/coder.svg",
				},
			],
		]);

		expect(
			getModelOptionsFromConfigs(configs, catalog, sameTypeProviders),
		).toEqual([
			expect.objectContaining({
				id: "config-primary",
				providerId: "prov-anthropic-primary",
				providerLabel: "Anthropic",
			}),
			expect.objectContaining({
				id: "config-hyper",
				providerId: "prov-anthropic-hyper",
				providerLabel: "Hyper",
				providerIcon: "/icon/coder.svg",
			}),
		]);
	});

	it("drops configs whose provider row is disabled", () => {
		const configs = [
			createConfig({
				id: "config-enabled",
				ai_provider_id: "prov-enabled",
				model: "gpt-4o",
			}),
			createConfig({
				id: "config-disabled",
				ai_provider_id: "prov-disabled",
				model: "gpt-4o-mini",
			}),
		];
		const catalog = createCatalog([
			{ provider: "openai", available: true, models: [] },
		]);
		const providers = new Map([
			[
				"prov-enabled",
				{ provider: "openai", displayName: "OpenAI", icon: "", enabled: true },
			],
			[
				"prov-disabled",
				{
					provider: "openai",
					displayName: "OpenAI Disabled",
					icon: "",
					enabled: false,
				},
			],
		]);

		expect(
			getModelOptionsFromConfigs(configs, catalog, providers).map(
				(option) => option.id,
			),
		).toEqual(["config-enabled"]);
	});

	it("keeps configs when the provider enabled flag is undefined", () => {
		const configs = [
			createConfig({
				id: "config-openai",
				ai_provider_id: "prov-openai",
				model: "gpt-4o",
			}),
		];
		const catalog = createCatalog([
			{ provider: "openai", available: true, models: [] },
		]);

		expect(
			getModelOptionsFromConfigs(configs, catalog, providerInfoByID).map(
				(option) => option.id,
			),
		).toEqual(["config-openai"]);
	});

	it("excludes only the disabled instance for same-type providers", () => {
		// The catalog marks the type as available because of the enabled
		// instance, so only the per-row flag can exclude the disabled one.
		const configs = [
			createConfig({
				id: "config-primary",
				ai_provider_id: "prov-anthropic-primary",
				model: "claude-sonnet-4-20250514",
			}),
			createConfig({
				id: "config-secondary",
				ai_provider_id: "prov-anthropic-secondary",
				model: "claude-opus-4-20250514",
			}),
		];
		const catalog = createCatalog([
			{ provider: "anthropic", available: true, models: [] },
		]);
		const sameTypeProviders = new Map([
			[
				"prov-anthropic-primary",
				{
					provider: "anthropic",
					displayName: "Anthropic",
					icon: "",
					enabled: true,
				},
			],
			[
				"prov-anthropic-secondary",
				{
					provider: "anthropic",
					displayName: "Anthropic Secondary",
					icon: "",
					enabled: false,
				},
			],
		]);

		expect(
			getModelOptionsFromConfigs(configs, catalog, sameTypeProviders).map(
				(option) => option.id,
			),
		).toEqual(["config-primary"]);
	});
});

describe("filterConfigsWithEnabledProvider", () => {
	const configs = [
		createConfig({
			id: "config-enabled",
			ai_provider_id: "prov-enabled",
			model: "gpt-4o",
		}),
		createConfig({
			id: "config-disabled",
			ai_provider_id: "prov-disabled",
			model: "gpt-4o-mini",
		}),
		createConfig({
			id: "config-unknown",
			ai_provider_id: "prov-unknown",
			model: "claude-sonnet-4-20250514",
		}),
	];
	const providers = new Map([
		[
			"prov-enabled",
			{ provider: "openai", displayName: "OpenAI", icon: "", enabled: true },
		],
		[
			"prov-disabled",
			{
				provider: "openai",
				displayName: "OpenAI Disabled",
				icon: "",
				enabled: false,
			},
		],
	]);

	it("drops configs of disabled or unknown providers", () => {
		expect(
			filterConfigsWithEnabledProvider(configs, providers).map(
				(config) => config.id,
			),
		).toEqual(["config-enabled"]);
	});

	it("keeps configs whose provider rows lack enabled flags", () => {
		const flaglessProviders = new Map(
			["prov-enabled", "prov-disabled", "prov-unknown"].map((id) => [
				id,
				{ provider: "openai", displayName: "OpenAI", icon: "" },
			]),
		);
		expect(
			filterConfigsWithEnabledProvider(configs, flaglessProviders),
		).toEqual(configs);
	});
});

describe("providerInfoByIDFromConfigs", () => {
	it("maps ChatProviderConfig.id to provider metadata", () => {
		const map = providerInfoByIDFromConfigs([
			{
				...MockChatProviderConfig,
				id: "prov-openai",
				provider: "openai",
				display_name: "Primary OpenAI",
				icon: "/icon/openai.svg",
			},
		]);

		expect(map.get("prov-openai")).toEqual({
			provider: "openai",
			displayName: "Primary OpenAI",
			icon: "/icon/openai.svg",
			enabled: true,
		});
		expect(map.size).toBe(1);
	});
});

describe("providerInfoByIDFromUserConfigs", () => {
	it("maps UserChatProviderConfig.provider_id to provider metadata", () => {
		const map = providerInfoByIDFromUserConfigs([
			{
				provider_id: "prov-openai",
				provider: "openai",
				display_name: "Primary OpenAI",
				icon: "/icon/openai.svg",
				enabled: true,
				has_user_api_key: false,
				has_central_api_key_fallback: true,
				byok_enabled: true,
			},
		]);

		expect(map.get("prov-openai")).toEqual({
			provider: "openai",
			displayName: "Primary OpenAI",
			icon: "/icon/openai.svg",
			enabled: true,
		});
		expect(map.size).toBe(1);
	});
});

describe("providerTypeByIDFromConfigs", () => {
	it("maps ChatProviderConfig.id to its provider type", () => {
		const map = providerTypeByIDFromConfigs([
			{ ...MockChatProviderConfig, id: "prov-openai", provider: "openai" },
			{
				...MockChatProviderConfig,
				id: "prov-anthropic",
				provider: "anthropic",
			},
		]);

		expect(map.get("prov-openai")).toBe("openai");
		expect(map.get("prov-anthropic")).toBe("anthropic");
		expect(map.size).toBe(2);
	});

	it("returns an empty map for nullish input", () => {
		expect(providerTypeByIDFromConfigs(undefined).size).toBe(0);
		expect(providerTypeByIDFromConfigs(null).size).toBe(0);
	});
});

describe("providerTypeByIDFromUserConfigs", () => {
	it("maps UserChatProviderConfig.provider_id to its provider type", () => {
		const map = providerTypeByIDFromUserConfigs([
			{
				provider_id: "prov-openai",
				provider: "openai",
				display_name: "OpenAI",
				icon: "",
				enabled: true,
				has_user_api_key: false,
				has_central_api_key_fallback: true,
				byok_enabled: true,
			},
		]);

		expect(map.get("prov-openai")).toBe("openai");
		expect(map.size).toBe(1);
	});

	it("returns an empty map for nullish input", () => {
		expect(providerTypeByIDFromUserConfigs(undefined).size).toBe(0);
		expect(providerTypeByIDFromUserConfigs(null).size).toBe(0);
	});
});

describe("getUnsupportedProviderNames", () => {
	const unsupportedCopilot: ChatModelsResponse["unsupported_providers"] = [
		{
			provider: "copilot",
			display_name: "GitHub Copilot",
		},
	];

	it("returns names when no supported provider is configured", () => {
		const catalog = createCatalog([], unsupportedCopilot);
		expect(getUnsupportedProviderNames(catalog)).toEqual(["GitHub Copilot"]);
	});

	it("returns empty when a supported provider is also configured", () => {
		const catalog = createCatalog(
			[{ provider: "anthropic", available: false, models: [] }],
			unsupportedCopilot,
		);
		expect(getUnsupportedProviderNames(catalog)).toEqual([]);
	});

	it("returns empty when there are no unsupported providers", () => {
		expect(getUnsupportedProviderNames(createCatalog([]))).toEqual([]);
	});

	it("falls back to the provider type when display_name is empty", () => {
		const catalog = createCatalog(
			[],
			[
				{
					provider: "copilot",
					display_name: "",
				},
			],
		);
		expect(getUnsupportedProviderNames(catalog)).toEqual(["copilot"]);
	});

	it("tolerates a missing catalog", () => {
		expect(getUnsupportedProviderNames(undefined)).toEqual([]);
		expect(getUnsupportedProviderNames(null)).toEqual([]);
	});
});

describe("resolveModelSelector", () => {
	const config = createConfig({
		id: "config-openai",
		ai_provider_id: "prov-openai",
		model: "gpt-4o",
		display_name: "GPT-4o",
		context_limit: 128_000,
	});
	const catalog = createCatalog([
		{ provider: "openai", available: true, models: [] },
	]);
	const userProviderConfigs = [
		{
			provider_id: "prov-openai",
			provider: "openai",
			display_name: "OpenAI",
			icon: "",
			enabled: true,
			has_user_api_key: false,
			has_central_api_key_fallback: true,
			byok_enabled: true,
		},
	];

	it("stays loading and drops options while the provider query is pending", () => {
		// Catalog + configs have resolved, but provider identity has not, so
		// the provider map is empty. Options must be dropped and the flag must
		// stay loading rather than flashing "No Models".
		const state = resolveModelSelector(
			{ data: [config], isLoading: false },
			{ data: catalog, isLoading: false },
			{ data: undefined, isLoading: true },
		);

		expect(state.isModelCatalogLoading).toBe(true);
		expect(state.options).toEqual([]);
	});

	it("resolves options once every query settles", () => {
		const state = resolveModelSelector(
			{ data: [config], isLoading: false },
			{ data: catalog, isLoading: false },
			{ data: userProviderConfigs, isLoading: false },
		);

		expect(state.isModelCatalogLoading).toBe(false);
		expect(state.modelCatalog).toBe(catalog);
		expect(state.options).toEqual([
			{
				id: "config-openai",
				provider: "openai",
				providerId: "prov-openai",
				providerLabel: "OpenAI",
				providerIcon: "",
				model: "gpt-4o",
				displayName: "GPT-4o",
				contextLimit: 128_000,
			},
		]);
	});
});
