import { describe, expect, it } from "vitest";
import type {
	ChatModel,
	ChatModelProviderDescriptor,
	ChatProviderConfig,
	OrganizationChatModelsResponse,
} from "#/api/typesGenerated";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
	MockChatProviderConfig,
} from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import {
	countConfiguredProviderConfigs,
	filterModelsWithEnabledProvider,
	formatProviderLabel,
	getModelOptionsFromModels,
	getModelSelectorPlaceholder,
	getUnsupportedProviderNames,
	getUsableDefaultModelIDForOrganization,
	hasConfiguredProviderConfigs,
	hasUserFixableProviders,
	isUnavailableHistoricalModelID,
	isUnsetModelRef,
	NIL_UUID,
	organizationsWithEnabledChatModels,
	providerInfoByIDFromUserConfigs,
	providerTypeByIDFromUserConfigs,
	resolveModelOptionId,
	resolveModelSelector,
} from "./modelOptions";

const createConfig = (
	overrides: Partial<ChatModel> &
		Pick<ChatModel, "id" | "ai_provider_id" | "model">,
): ChatModel => ({
	...MockChatModel,
	context_limit: 0,
	compression_threshold: 0,
	created_at: "",
	updated_at: "",
	...overrides,
});

const testOrganizationID = MockChatModel.organization_id;

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

type TestProvider = Pick<
	ChatModelProviderDescriptor,
	"available" | "unavailable_reason"
> & {
	enabled?: boolean;
	id?: string;
	provider: string;
};

const createCatalog = (
	providers: readonly TestProvider[],
	unsupportedProviders: OrganizationChatModelsResponse["unsupported_providers"] = [],
	models: readonly ChatModel[] = [],
): OrganizationChatModelsResponse => ({
	models,
	providers: providers.map(({ id, provider, ...status }) => ({
		...MockChatModelProviderDescriptor,
		id: id ?? `prov-${provider}`,
		type: provider,
		display_name:
			provider === MockChatModelProviderDescriptor.type
				? MockChatModelProviderDescriptor.display_name
				: provider,
		...status,
	})),
	unsupported_providers: unsupportedProviders,
});

const createProviderConfig = (
	overrides: Pick<ChatProviderConfig, "provider" | "source"> &
		Partial<ChatProviderConfig>,
): ChatProviderConfig => ({
	...MockChatProviderConfig,
	id: `prov-${overrides.provider}`,
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
			},
		]);

		expect(hasUserFixableProviders(catalog)).toBe(false);
	});
});

describe("hasConfiguredProviderConfigs", () => {
	it("ignores supported provider placeholders", () => {
		const catalog = createCatalog([{ provider: "openai", available: true }]);

		expect(
			hasConfiguredProviderConfigs(
				[createProviderConfig({ provider: "openai", source: "supported" })],
				catalog,
			),
		).toBe(false);
	});

	it("returns true for database and env preset provider models", () => {
		const catalog = createCatalog([{ provider: "openai", available: true }]);

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

	it("excludes disabled and unavailable provider models", () => {
		const catalog = createCatalog([
			{ provider: "openai", available: true },
			{
				provider: "anthropic",
				available: false,
				unavailable_reason: "missing_api_key",
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
	it("counts only enabled provider models available in the catalog", () => {
		const catalog = createCatalog([
			{ provider: "openai", available: true },
			{ provider: "anthropic", available: true },
			{ provider: "google", available: true },
			{ provider: "azure", available: true },
			{
				provider: "bedrock",
				available: false,
				unavailable_reason: "missing_api_key",
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
			},
		]);

		expect(getModelSelectorPlaceholder([], false, true, catalog)).toBe(
			"No Models Available",
		);
	});
});

describe("isUnsetModelRef", () => {
	it.each([
		[undefined, true],
		[null, true],
		["", true],
		["   ", true],
		[NIL_UUID, true],
		[`  ${NIL_UUID}  `, true],
		["config-1", false],
	])("reports whether %j is unset", (modelRef, expected) => {
		expect(isUnsetModelRef(modelRef)).toBe(expected);
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

	it("treats a nil UUID as unset", () => {
		expect(
			resolveModelOptionId(
				"00000000-0000-0000-0000-000000000000",
				modelOptions,
			),
		).toBe("");
	});

	it("returns an empty string when no option matches", () => {
		expect(resolveModelOptionId("openai:gpt-5", modelOptions)).toBe("");
	});
	it("detects only non-empty unavailable historical IDs", () => {
		expect(isUnavailableHistoricalModelID("config-1", modelOptions)).toBe(
			false,
		);
		expect(isUnavailableHistoricalModelID("foreign-config", modelOptions)).toBe(
			true,
		);
		expect(isUnavailableHistoricalModelID("", modelOptions)).toBe(false);
		expect(
			isUnavailableHistoricalModelID(
				"00000000-0000-0000-0000-000000000000",
				modelOptions,
			),
		).toBe(false);
	});
});

describe("getUsableDefaultModelIDForOrganization", () => {
	const modelOptions = [
		{
			id: "local-default",
			provider: "openai",
			model: "gpt-4o",
			displayName: "GPT-4o",
		},
	] as const;

	it("selects only a usable default from the requested organization", () => {
		const configs = [
			createConfig({
				id: "foreign-default",
				organization_id: "foreign-org",
				ai_provider_id: "prov-openai",
				model: "gpt-4.1",
				is_default: true,
			}),
			createConfig({
				id: "local-default",
				organization_id: testOrganizationID,
				ai_provider_id: "prov-openai",
				model: "gpt-4o",
				is_default: true,
			}),
		];

		expect(
			getUsableDefaultModelIDForOrganization(
				configs,
				modelOptions,
				testOrganizationID,
			),
		).toBe("local-default");
	});

	it("does not return a foreign or unusable default", () => {
		const configs = [
			createConfig({
				id: "foreign-default",
				organization_id: "foreign-org",
				ai_provider_id: "prov-openai",
				model: "gpt-4.1",
				is_default: true,
			}),
		];

		expect(
			getUsableDefaultModelIDForOrganization(
				configs,
				modelOptions,
				testOrganizationID,
			),
		).toBe("");
	});
});

describe("organizationsWithEnabledChatModels", () => {
	const organizations = [MockDefaultOrganization, MockOrganization2];

	it("keeps only organizations with at least one enabled model", () => {
		const models: ChatModel[] = [
			{
				...MockChatModel,
				id: "model-disabled",
				organization_id: MockDefaultOrganization.id,
				enabled: false,
			},
			{
				...MockChatModel,
				id: "model-enabled",
				organization_id: MockOrganization2.id,
				enabled: true,
			},
		];

		expect(organizationsWithEnabledChatModels(organizations, models)).toEqual([
			MockOrganization2,
		]);
	});

	it("preserves organization order", () => {
		const models: ChatModel[] = [
			{
				...MockChatModel,
				id: "model-2",
				organization_id: MockOrganization2.id,
			},
			{
				...MockChatModel,
				id: "model-1",
				organization_id: MockDefaultOrganization.id,
			},
		];

		expect(organizationsWithEnabledChatModels(organizations, models)).toEqual(
			organizations,
		);
	});

	it("returns no organizations when no models exist", () => {
		expect(organizationsWithEnabledChatModels(organizations, [])).toEqual([]);
	});
});

describe("getModelOptionsFromModels", () => {
	it("excludes models from other organizations", () => {
		const models = [
			createConfig({
				id: "foreign-config",
				organization_id: "foreign-org",
				ai_provider_id: "prov-openai",
				model: "gpt-4.1",
			}),
			createConfig({
				id: "local-config",
				organization_id: testOrganizationID,
				ai_provider_id: "prov-openai",
				model: "gpt-4o",
			}),
		];
		const catalog = createCatalog([{ provider: "openai", available: true }]);

		expect(
			getModelOptionsFromModels(
				models,
				catalog,
				providerInfoByID,
				testOrganizationID,
			).map((option) => option.id),
		).toEqual(["local-config"]);
	});

	it("returns distinct options for models with the same provider and model", () => {
		const models = [
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
			},
		]);

		expect(
			getModelOptionsFromModels(
				models,
				catalog,
				providerInfoByID,
				testOrganizationID,
			),
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
		const models = [
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
		const catalog = createCatalog([{ provider: "openai", available: true }]);

		expect(
			getModelOptionsFromModels(
				models,
				catalog,
				providerInfoByID,
				testOrganizationID,
			),
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

	it("excludes models whose providers are unavailable", () => {
		const models = [
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
			},
		]);

		expect(
			getModelOptionsFromModels(
				models,
				catalog,
				providerInfoByID,
				testOrganizationID,
			),
		).toEqual([]);
	});

	it("excludes disabled models", () => {
		const models = [
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
			},
		]);

		expect(
			getModelOptionsFromModels(
				models,
				catalog,
				providerInfoByID,
				testOrganizationID,
			).map((option) => option.id),
		).toEqual(["config-2"]);
	});

	it("falls back to the model name when display_name is blank", () => {
		const models = [
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
			},
		]);

		expect(
			getModelOptionsFromModels(
				models,
				catalog,
				providerInfoByID,
				testOrganizationID,
			),
		).toEqual([
			expect.objectContaining({
				id: "config-1",
				model: "gpt-4o",
				displayName: "gpt-4o",
			}),
		]);
	});

	it("returns an empty array for null and undefined inputs", () => {
		expect(
			getModelOptionsFromModels(
				null,
				null,
				providerInfoByID,
				testOrganizationID,
			),
		).toEqual([]);
		expect(
			getModelOptionsFromModels(
				undefined,
				undefined,
				providerInfoByID,
				testOrganizationID,
			),
		).toEqual([]);
	});

	it("sorts options by provider and display name", () => {
		const models = [
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
			},
			{
				provider: "anthropic",
				available: true,
			},
		]);

		expect(
			getModelOptionsFromModels(
				models,
				catalog,
				providerInfoByID,
				testOrganizationID,
			).map((option) => option.id),
		).toEqual([
			"config-anthropic",
			"config-openai-alpha",
			"config-openai-zeta",
		]);
	});

	it("keeps canonical wrapper-provider model strings distinct", () => {
		const models = [
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
			},
		]);

		expect(
			getModelOptionsFromModels(
				models,
				catalog,
				providerInfoByID,
				testOrganizationID,
			).map((option) => option.id),
		).toEqual(["config-2", "config-1"]);
	});

	it("drops models whose ai_provider_id is absent from the provider map", () => {
		const models = [
			createConfig({
				id: "config-1",
				ai_provider_id: "prov-openai",
				model: "gpt-4o",
				display_name: "GPT-4o",
				context_limit: 128_000,
			}),
		];
		const catalog = createCatalog([{ provider: "openai", available: true }]);

		expect(
			getModelOptionsFromModels(models, catalog, new Map(), testOrganizationID),
		).toEqual([]);
	});

	it("keeps only models whose ai_provider_id resolves in the provider map", () => {
		const models = [
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
			{ provider: "openai", available: true },
			{ provider: "anthropic", available: true },
		]);
		const partialMap = new Map([
			["prov-openai", { provider: "openai", displayName: "OpenAI", icon: "" }],
		]);

		expect(
			getModelOptionsFromModels(
				models,
				catalog,
				partialMap,
				testOrganizationID,
			).map((option) => option.id),
		).toEqual(["config-openai"]);
	});

	it("preserves provider instance metadata for same-type providers", () => {
		const models = [
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
			{
				id: "prov-anthropic-primary",
				provider: "anthropic",
				available: true,
			},
			{
				id: "prov-anthropic-hyper",
				provider: "anthropic",
				available: true,
			},
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
			getModelOptionsFromModels(
				models,
				catalog,
				sameTypeProviders,
				testOrganizationID,
			),
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

	it("drops models whose provider row is disabled", () => {
		const models = [
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
			{ id: "prov-enabled", provider: "openai", available: true },
			{ id: "prov-disabled", provider: "openai", available: true },
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
			getModelOptionsFromModels(
				models,
				catalog,
				providers,
				testOrganizationID,
			).map((option) => option.id),
		).toEqual(["config-enabled"]);
	});

	it("keeps models when the provider enabled flag is undefined", () => {
		const models = [
			createConfig({
				id: "config-openai",
				ai_provider_id: "prov-openai",
				model: "gpt-4o",
			}),
		];
		const catalog = createCatalog([{ provider: "openai", available: true }]);

		expect(
			getModelOptionsFromModels(
				models,
				catalog,
				providerInfoByID,
				testOrganizationID,
			).map((option) => option.id),
		).toEqual(["config-openai"]);
	});

	it("uses exact UUID availability for same-type providers", () => {
		const models = [
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
			{
				id: "prov-anthropic-primary",
				provider: "anthropic",
				available: true,
			},
			{
				id: "prov-anthropic-secondary",
				provider: "anthropic",
				available: false,
				unavailable_reason: "missing_api_key",
			},
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
					enabled: true,
				},
			],
		]);

		expect(
			getModelOptionsFromModels(
				models,
				catalog,
				sameTypeProviders,
				testOrganizationID,
			).map((option) => option.id),
		).toEqual(["config-primary"]);
	});
});

describe("filterModelsWithEnabledProvider", () => {
	const models = [
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

	it("drops models of disabled or unknown providers", () => {
		expect(
			filterModelsWithEnabledProvider(models, providers).map(
				(config) => config.id,
			),
		).toEqual(["config-enabled"]);
	});

	it("keeps models whose provider rows lack enabled flags", () => {
		const flaglessProviders = new Map(
			["prov-enabled", "prov-disabled", "prov-unknown"].map((id) => [
				id,
				{ provider: "openai", displayName: "OpenAI", icon: "" },
			]),
		);
		expect(filterModelsWithEnabledProvider(models, flaglessProviders)).toEqual(
			models,
		);
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
	const unsupportedCopilot: OrganizationChatModelsResponse["unsupported_providers"] =
		[
			{
				provider: "copilot",
				display_name: "GitHub Copilot",
			},
		];

	it("returns names when no supported provider is configured", () => {
		const catalog = createCatalog(
			[{ provider: "copilot", available: false }],
			unsupportedCopilot,
		);
		expect(getUnsupportedProviderNames(catalog)).toEqual(["GitHub Copilot"]);
	});

	it("normalizes provider types when identifying unsupported descriptors", () => {
		const catalog = createCatalog(
			[{ provider: " COPILOT ", available: false }],
			unsupportedCopilot,
		);
		expect(getUnsupportedProviderNames(catalog)).toEqual(["GitHub Copilot"]);
	});

	it("returns empty when a supported provider is also configured", () => {
		const catalog = createCatalog(
			[
				{ provider: "copilot", available: false },
				{ provider: "anthropic", available: false },
			],
			unsupportedCopilot,
		);
		expect(getUnsupportedProviderNames(catalog)).toEqual([]);
	});

	it("returns names when the only supported provider is disabled", () => {
		const catalog = createCatalog(
			[
				{ provider: "copilot", available: false },
				{ provider: "anthropic", available: false, enabled: false },
			],
			unsupportedCopilot,
		);
		expect(getUnsupportedProviderNames(catalog)).toEqual(["GitHub Copilot"]);
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
	const catalog = createCatalog([{ provider: "openai", available: true }]);

	it("stays loading and drops options while the collection query is pending", () => {
		const state = resolveModelSelector(testOrganizationID, {
			data: undefined,
			isLoading: true,
		});

		expect(state.isModelCatalogLoading).toBe(true);
		expect(state.options).toEqual([]);
	});

	it("resolves options once every query settles", () => {
		const runtimeCatalog = { ...catalog, models: [config] };
		const state = resolveModelSelector(testOrganizationID, {
			data: runtimeCatalog,
			isLoading: false,
		});

		expect(state.isModelCatalogLoading).toBe(false);
		expect(state.modelCatalog).toBe(runtimeCatalog);
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
