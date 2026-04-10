import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import {
	buildInitialModelFormValues,
	buildModelConfigFromForm,
	emptyModelConfigFormState,
	extractModelConfigFormState,
	type ModelConfigFormState,
	parsePositiveInteger,
	parseThresholdInteger,
} from "./modelConfigFormLogic";

// ── Helpers ────────────────────────────────────────────────────

/**
 * Return an empty form state with the given overrides applied.
 * Provider sub-objects are deep-merged so callers can pass
 * partial overrides for nested fields.
 */
const formWith = (overrides: Record<string, unknown>): ModelConfigFormState => {
	const base = structuredClone(emptyModelConfigFormState);

	for (const [key, val] of Object.entries(overrides)) {
		if (val && typeof val === "object" && !Array.isArray(val)) {
			// Deep-merge provider sub-objects.
			base[key] = deepMerge(
				(base[key] as Record<string, unknown>) ?? {},
				val as Record<string, unknown>,
			);
		} else {
			(base as Record<string, unknown>)[key] = val;
		}
	}

	return base;
};

/** Simple recursive merge for plain objects. */
function deepMerge(
	target: Record<string, unknown>,
	source: Record<string, unknown>,
): Record<string, unknown> {
	const result = { ...target };
	for (const [key, val] of Object.entries(source)) {
		if (
			val &&
			typeof val === "object" &&
			!Array.isArray(val) &&
			result[key] &&
			typeof result[key] === "object" &&
			!Array.isArray(result[key])
		) {
			result[key] = deepMerge(
				result[key] as Record<string, unknown>,
				val as Record<string, unknown>,
			);
		} else {
			result[key] = val;
		}
	}
	return result;
}

/** Helper to read a nested value from the form state. */
function deepGet(obj: unknown, path: string[]): unknown {
	let current = obj;
	for (const key of path) {
		if (
			current === undefined ||
			current === null ||
			typeof current !== "object"
		) {
			return undefined;
		}
		current = (current as Record<string, unknown>)[key];
	}
	return current;
}

const expectFields = (value: unknown, expected: Record<string, unknown>) => {
	for (const [path, expectedValue] of Object.entries(expected)) {
		expect(deepGet(value, path.split("."))).toEqual(expectedValue);
	}
};

const expectExtractedFields = (
	modelConfig: TypesGen.ChatModelCallConfig,
	expected: Record<string, unknown>,
) => {
	expectFields(
		extractModelConfigFormState({
			...baseChatModelConfig,
			model_config: modelConfig,
		}),
		expected,
	);
};

const expectProviderFields = (
	provider: string,
	providerOptions: Record<string, unknown>,
	expected: Record<string, unknown>,
) => {
	expectFields(
		extractModelConfigFormState({
			...baseChatModelConfig,
			model_config: { provider_options: { [provider]: providerOptions } },
		})[provider],
		expected,
	);
};

const expectBuiltFields = (
	overrides: Record<string, unknown>,
	expected: Record<string, unknown>,
	provider: string | null | undefined = "openai",
) => {
	const result = buildModelConfigFromForm(provider, formWith(overrides));
	expect(result.fieldErrors).toEqual({});
	expectFields(result.modelConfig, expected);
};

/** Minimal ChatModelConfig with no model_config. */
const baseChatModelConfig: TypesGen.ChatModelConfig = {
	id: "test-id",
	provider: "openai",
	model: "gpt-4",
	display_name: "GPT-4",
	enabled: true,
	is_default: false,
	context_limit: 128000,
	compression_threshold: 80,
	created_at: "2025-01-01T00:00:00Z",
	updated_at: "2025-01-01T00:00:00Z",
};

const providerAttachment = (
	overrides: Partial<TypesGen.ChatModelProviderAttachment>,
): TypesGen.ChatModelProviderAttachment => ({
	id:
		overrides.id ?? `attachment-${overrides.provider_config_id ?? "config-a"}`,
	provider_config_id: overrides.provider_config_id ?? "config-a",
	provider: overrides.provider ?? "openai",
	priority: overrides.priority ?? 0,
	display_name: overrides.display_name ?? "Test config",
	enabled: overrides.enabled ?? true,
	has_api_key: overrides.has_api_key ?? true,
});

const attachments = (
	...configs: readonly (readonly [
		provider_config_id: string,
		priority: number,
	])[]
): TypesGen.ChatModelProviderAttachment[] =>
	configs.map(([provider_config_id, priority]) =>
		providerAttachment({ provider_config_id, priority }),
	);

const expectProviderConfigIds = (
	provider_configs: readonly TypesGen.ChatModelProviderAttachment[] | undefined,
	expected: string[],
): void => {
	expect(
		buildInitialModelFormValues({
			...baseChatModelConfig,
			provider_configs,
		}).providerConfigIds,
	).toEqual(expected);
};

// ── buildInitialModelFormValues ────────────────────────────────

describe("buildInitialModelFormValues", () => {
	it("returns create mode defaults including empty providerConfigIds", () => {
		expect(buildInitialModelFormValues()).toEqual({
			model: "",
			displayName: "",
			enabled: true,
			contextLimit: "",
			compressionThreshold: "",
			isDefault: false,
			config: emptyModelConfigFormState,
			providerConfigIds: [],
		});
	});

	it("returns a single providerConfigId when editing a model with one attachment", () => {
		expectProviderConfigIds(attachments(["config-a", 0]), ["config-a"]);
	});

	it.each([
		[
			"sorts providerConfigIds by ascending attachment priority",
			attachments(["config-c", 2], ["config-a", 0], ["config-b", 1]),
			["config-a", "config-b", "config-c"],
		],
		[
			"preserves source order for attachments with equal priorities",
			attachments(["config-b", 1], ["config-a", 1], ["config-c", 2]),
			["config-b", "config-a", "config-c"],
		],
	])("%s", (_description, provider_configs, expected) => {
		expectProviderConfigIds(provider_configs, expected);
	});

	it.each([
		undefined,
		[],
	] as const)("returns empty providerConfigIds when provider_configs is %o", (provider_configs) => {
		expectProviderConfigIds(provider_configs, []);
	});

	it("preserves ordered providerConfigIds alongside other populated fields", () => {
		const result = buildInitialModelFormValues({
			...baseChatModelConfig,
			model: "gpt-4.1",
			display_name: "GPT 4.1",
			enabled: false,
			is_default: true,
			context_limit: 64000,
			compression_threshold: 55,
			model_config: {
				max_output_tokens: 4096,
				temperature: 0.7,
			},
			provider_configs: attachments(
				["config-c", 2],
				["config-a", 0],
				["config-b", 1],
			),
		});

		expect(result).toMatchObject({
			model: "gpt-4.1",
			displayName: "GPT 4.1",
			enabled: false,
			contextLimit: "64000",
			compressionThreshold: "55",
			isDefault: true,
			providerConfigIds: ["config-a", "config-b", "config-c"],
		});
		expect(result.config.maxOutputTokens).toBe("4096");
		expect(result.config.temperature).toBe("0.7");
	});

	it("preserves enabled=true when editing an enabled model", () => {
		expect(buildInitialModelFormValues(baseChatModelConfig).enabled).toBe(true);
	});

	it("preserves enabled=false when editing a disabled model", () => {
		expect(
			buildInitialModelFormValues({
				...baseChatModelConfig,
				enabled: false,
			}).enabled,
		).toBe(false);
	});
});

// ── parsePositiveInteger ───────────────────────────────────────

describe("parsePositiveInteger", () => {
	const sharedCases = [
		["returns null for empty string", "", null],
		["returns null for whitespace-only string", "   ", null],
		["parses a valid positive integer", "42", 42],
		["parses a string with surrounding whitespace", "  42  ", 42],
		["returns null for negative numbers", "-5", null],
		["returns null for non-numeric strings", "abc", null],
	] as const;

	it.each([
		...sharedCases,
		["returns null for zero", "0", null],
		["returns null for Infinity", "Infinity", null],
	] as const)("%s", (_description, input, expected) => {
		expect(parsePositiveInteger(input)).toBe(expected);
	});

	it.each([
		["3.9", 3],
		["1.1", 1],
	] as const)("truncates floating point value %s to %d", (input, expected) => {
		expect(parsePositiveInteger(input)).toBe(expected);
	});
});

// ── parseThresholdInteger ──────────────────────────────────────

describe("parseThresholdInteger", () => {
	const sharedCases = [
		["returns null for empty string", "", null],
		["returns null for whitespace-only string", "   ", null],
		["parses a value in range", "50", 50],
		["trims whitespace before parsing", "  70  ", 70],
		["returns null for negative values", "-1", null],
		["returns null for non-numeric strings", "abc", null],
	] as const;

	it.each([
		...sharedCases,
		["parses 0 (lower bound)", "0", 0],
		["parses 100 (upper bound)", "100", 100],
		["returns null for values above 100", "101", null],
	] as const)("%s", (_description, input, expected) => {
		expect(parseThresholdInteger(input)).toBe(expected);
	});
});

// ── extractModelConfigFormState ────────────────────────────────

describe("extractModelConfigFormState", () => {
	it("returns empty form state when model_config is undefined", () => {
		const result = extractModelConfigFormState(baseChatModelConfig);
		expect(result).toEqual(emptyModelConfigFormState);
	});

	it("returns a copy, not a reference to emptyModelConfigFormState", () => {
		const result = extractModelConfigFormState(baseChatModelConfig);
		expect(result).not.toBe(emptyModelConfigFormState);
	});

	it("extracts top-level numeric fields", () => {
		expectExtractedFields(
			{
				max_output_tokens: 4096,
				temperature: 0.7,
				top_p: 0.9,
				top_k: 40,
				presence_penalty: 0.5,
				frequency_penalty: 0.3,
			},
			{
				maxOutputTokens: "4096",
				temperature: "0.7",
				topP: "0.9",
				topK: "40",
				presencePenalty: "0.5",
				frequencyPenalty: "0.3",
			},
		);
	});

	it("extracts pricing fields", () => {
		expectExtractedFields(
			{
				cost: {
					input_price_per_million_tokens: "0.15",
					output_price_per_million_tokens: "0.6",
					cache_read_price_per_million_tokens: "0.03",
					cache_write_price_per_million_tokens: "0.3",
				},
			},
			{
				"cost.inputPricePerMillionTokens": "0.15",
				"cost.outputPricePerMillionTokens": "0.6",
				"cost.cacheReadPricePerMillionTokens": "0.03",
				"cost.cacheWritePricePerMillionTokens": "0.3",
			},
		);
	});

	const googleSafetySettings = [{ category: "harm", threshold: "block" }];

	it.each([
		{
			description: "extracts OpenAI provider options",
			provider: "openai",
			providerOptions: {
				reasoning_effort: "high",
				parallel_tool_calls: true,
				text_verbosity: "medium",
				service_tier: "auto",
				reasoning_summary: "concise",
				user: "test-user",
				prompt_cache_key: "my-cache-key",
			},
			expected: {
				reasoningEffort: "high",
				parallelToolCalls: "true",
				textVerbosity: "medium",
				serviceTier: "auto",
				reasoningSummary: "concise",
				user: "test-user",
				promptCacheKey: "my-cache-key",
			},
		},
		{
			description: "extracts Anthropic provider options with thinking",
			provider: "anthropic",
			providerOptions: {
				effort: "high",
				thinking: { budget_tokens: 1024 },
				send_reasoning: true,
				disable_parallel_tool_use: false,
			},
			expected: {
				effort: "high",
				"thinking.budgetTokens": "1024",
				sendReasoning: "true",
				disableParallelToolUse: "false",
			},
		},
		{
			description: "extracts Google provider options with safety settings",
			provider: "google",
			providerOptions: {
				thinking_config: {
					thinking_budget: 2048,
					include_thoughts: true,
				},
				cached_content: "cache-123",
				safety_settings: googleSafetySettings,
			},
			expected: {
				"thinkingConfig.thinkingBudget": "2048",
				"thinkingConfig.includeThoughts": "true",
				cachedContent: "cache-123",
				safetySettings: JSON.stringify(googleSafetySettings, null, 2),
			},
		},
		{
			description:
				"returns empty string for google safety settings when absent",
			provider: "google",
			providerOptions: {},
			expected: { safetySettings: "" },
		},
		{
			description: "extracts OpenAI-compatible provider options",
			provider: "openaicompat",
			providerOptions: {
				reasoning_effort: "low",
				user: "compat-user",
			},
			expected: {
				reasoningEffort: "low",
				user: "compat-user",
			},
		},
		{
			description: "extracts OpenRouter provider options",
			provider: "openrouter",
			providerOptions: {
				reasoning: {
					enabled: true,
					effort: "medium",
					max_tokens: 500,
					exclude: false,
				},
				parallel_tool_calls: true,
				include_usage: true,
				user: "router-user",
			},
			expected: {
				"reasoning.enabled": "true",
				"reasoning.effort": "medium",
				"reasoning.maxTokens": "500",
				"reasoning.exclude": "false",
				parallelToolCalls: "true",
				includeUsage: "true",
				user: "router-user",
			},
		},
		{
			description: "extracts Vercel provider options",
			provider: "vercel",
			providerOptions: {
				reasoning: {
					enabled: false,
					effort: "high",
					max_tokens: 1000,
					exclude: true,
				},
				parallel_tool_calls: false,
				user: "vercel-user",
			},
			expected: {
				"reasoning.enabled": "false",
				"reasoning.effort": "high",
				"reasoning.maxTokens": "1000",
				"reasoning.exclude": "true",
				parallelToolCalls: "false",
				user: "vercel-user",
			},
		},
	] as const)("$description", ({ provider, providerOptions, expected }) => {
		expectProviderFields(provider, providerOptions, expected);
	});

	it("handles missing provider_options gracefully", () => {
		const model: TypesGen.ChatModelConfig = {
			...baseChatModelConfig,
			model_config: {
				temperature: 0.5,
			},
		};
		const result = extractModelConfigFormState(model);
		expect(result.temperature).toBe("0.5");
		// All provider-specific fields should be empty.
		const openai = result.openai as Record<string, unknown>;
		expect(openai.reasoningEffort).toBe("");
		const anthropic = result.anthropic as Record<string, unknown>;
		expect(anthropic.effort).toBe("");
		const google = result.google as Record<string, unknown>;
		expect(deepGet(google, ["thinkingConfig", "thinkingBudget"])).toBe("");
	});

	it("returns deep copies of provider sub-objects", () => {
		const result = extractModelConfigFormState(baseChatModelConfig);
		const empty = emptyModelConfigFormState;
		expect(result.openai).not.toBe(empty.openai);
		expect(result.anthropic).not.toBe(empty.anthropic);
		expect(result.google).not.toBe(empty.google);
		expect(result.openaicompat).not.toBe(empty.openaicompat);
		expect(result.openrouter).not.toBe(empty.openrouter);
		expect(result.vercel).not.toBe(empty.vercel);
	});
});

// ── buildModelConfigFromForm ───────────────────────────────────

describe("buildModelConfigFromForm", () => {
	describe("empty form", () => {
		it("returns no modelConfig and no errors for empty form", () => {
			const result = buildModelConfigFromForm(
				"openai",
				emptyModelConfigFormState,
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig).toBeUndefined();
		});
	});

	describe("top-level numeric fields", () => {
		it.each([
			["maxOutputTokens", "4096", "max_output_tokens", 4096],
			["temperature", "0.7", "temperature", 0.7],
			["topP", "0.95", "top_p", 0.95],
			["topK", "40", "top_k", 40],
			["presencePenalty", "0.5", "presence_penalty", 0.5],
			["frequencyPenalty", "0.3", "frequency_penalty", 0.3],
		] as const)("builds config with valid %s", (fieldName, value, path, expected) => {
			expectBuiltFields({ [fieldName]: value }, { [path]: expected });
		});

		it("reports error for non-numeric maxOutputTokens", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ maxOutputTokens: "abc" }),
			);
			expect(result.fieldErrors.maxOutputTokens).toContain(
				"must be a valid integer",
			);
			expect(result.modelConfig).toBeUndefined();
		});

		it("reports error for non-numeric temperature", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ temperature: "hot" }),
			);
			expect(result.fieldErrors.temperature).toContain(
				"must be a valid number",
			);
		});

		it("reports error for non-numeric topP", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ topP: "not-a-number" }),
			);
			expect(result.fieldErrors.topP).toContain("must be a valid number");
		});

		it("reports error for non-numeric topK", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ topK: "xyz" }),
			);
			expect(result.fieldErrors.topK).toContain("must be a valid integer");
		});

		it("skips empty string fields (no undefined values in output)", () => {
			const result = buildModelConfigFromForm(
				null,
				formWith({ temperature: "0.5" }),
			);
			expect(result.modelConfig).toBeDefined();
			expect(result.modelConfig).not.toHaveProperty("max_output_tokens");
			expect(result.modelConfig).not.toHaveProperty("top_p");
			expect(result.modelConfig).not.toHaveProperty("top_k");
			expect(result.modelConfig).not.toHaveProperty("presence_penalty");
			expect(result.modelConfig).not.toHaveProperty("frequency_penalty");
			expect(result.modelConfig).not.toHaveProperty("provider_options");
		});
	});

	describe("pricing fields", () => {
		it.each([
			[
				"inputPricePerMillionTokens",
				"0.15",
				"cost.input_price_per_million_tokens",
				"0.15",
			],
			[
				"outputPricePerMillionTokens",
				"0.6",
				"cost.output_price_per_million_tokens",
				"0.6",
			],
			[
				"cacheReadPricePerMillionTokens",
				"0.03",
				"cost.cache_read_price_per_million_tokens",
				"0.03",
			],
			[
				"cacheWritePricePerMillionTokens",
				"0.3",
				"cost.cache_write_price_per_million_tokens",
				"0.3",
			],
		] as const)("builds config with valid pricing field %s", (fieldName, value, path, expected) => {
			expectBuiltFields({ cost: { [fieldName]: value } }, { [path]: expected });
		});

		it("reports error for negative pricing fields", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ cost: { inputPricePerMillionTokens: "-0.5" } }),
			);
			expect(result.fieldErrors["cost.inputPricePerMillionTokens"]).toContain(
				"must be zero or greater",
			);
			expect(result.modelConfig).toBeUndefined();
		});
	});
	describe("OpenAI / Azure provider", () => {
		it("builds OpenAI provider options with reasoning effort", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ openai: { reasoningEffort: "high" } }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.provider_options?.openai).toEqual({
				reasoning_effort: "high",
			});
		});

		it("builds Azure provider options (same as OpenAI)", () => {
			const result = buildModelConfigFromForm(
				"azure",
				formWith({ openai: { parallelToolCalls: "true" } }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.provider_options?.openai).toEqual({
				parallel_tool_calls: true,
			});
		});

		it("builds OpenAI options with all fields set", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({
					openai: {
						reasoningEffort: "medium",
						parallelToolCalls: "false",
						textVerbosity: "low",
						serviceTier: "auto",
						reasoningSummary: "concise",
						user: "user-123",
						promptCacheKey: "cache-key-1",
					},
				}),
			);
			expect(result.fieldErrors).toEqual({});
			const openai = result.modelConfig?.provider_options?.openai as Record<
				string,
				unknown
			>;
			expect(openai.reasoning_effort).toBe("medium");
			expect(openai.parallel_tool_calls).toBe(false);
			expect(openai.text_verbosity).toBe("low");
			expect(openai.service_tier).toBe("auto");
			expect(openai.reasoning_summary).toBe("concise");
			expect(openai.user).toBe("user-123");
			expect(openai.prompt_cache_key).toBe("cache-key-1");
		});

		it("reports error for invalid reasoning effort option", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ openai: { reasoningEffort: "invalid_value" } }),
			);
			expect(result.fieldErrors["openai.reasoningEffort"]).toContain(
				"invalid value",
			);
		});

		it("reports error for invalid parallel tool calls boolean", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ openai: { parallelToolCalls: "maybe" } }),
			);
			expect(result.fieldErrors["openai.parallelToolCalls"]).toContain(
				"must be true or false",
			);
		});

		it("reports error for invalid text verbosity option", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ openai: { textVerbosity: "invalid" } }),
			);
			expect(result.fieldErrors["openai.textVerbosity"]).toContain(
				"invalid value",
			);
		});

		it("does not set provider_options when all OpenAI fields are empty", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ temperature: "0.5" }),
			);
			expect(result.modelConfig).toBeDefined();
			expect(result.modelConfig?.provider_options).toBeUndefined();
		});
	});

	describe("Anthropic / Bedrock provider", () => {
		it("builds Anthropic provider options with effort", () => {
			const result = buildModelConfigFromForm(
				"anthropic",
				formWith({ anthropic: { effort: "high" } }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.provider_options?.anthropic).toEqual({
				effort: "high",
			});
		});

		it("builds Bedrock provider options (same as Anthropic)", () => {
			const result = buildModelConfigFromForm(
				"bedrock",
				formWith({ anthropic: { sendReasoning: "true" } }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.provider_options?.anthropic).toEqual({
				send_reasoning: true,
			});
		});

		it("builds Anthropic options with thinking budget", () => {
			const result = buildModelConfigFromForm(
				"anthropic",
				formWith({
					anthropic: { thinking: { budgetTokens: "2048" } },
				}),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.provider_options?.anthropic).toEqual({
				thinking: { budget_tokens: 2048 },
			});
		});

		it("builds Anthropic options with all fields", () => {
			const result = buildModelConfigFromForm(
				"anthropic",
				formWith({
					anthropic: {
						effort: "max",
						thinking: { budgetTokens: "1024" },
						sendReasoning: "false",
						disableParallelToolUse: "true",
					},
				}),
			);
			expect(result.fieldErrors).toEqual({});
			const anthropic = result.modelConfig?.provider_options
				?.anthropic as Record<string, unknown>;
			expect(anthropic.effort).toBe("max");
			expect(anthropic.thinking).toEqual({ budget_tokens: 1024 });
			expect(anthropic.send_reasoning).toBe(false);
			expect(anthropic.disable_parallel_tool_use).toBe(true);
		});

		it("reports error for invalid Anthropic effort option", () => {
			const result = buildModelConfigFromForm(
				"anthropic",
				formWith({ anthropic: { effort: "ultra" } }),
			);
			expect(result.fieldErrors["anthropic.effort"]).toContain("invalid value");
		});

		it("reports error for non-numeric thinking budget tokens", () => {
			const result = buildModelConfigFromForm(
				"anthropic",
				formWith({
					anthropic: { thinking: { budgetTokens: "lots" } },
				}),
			);
			expect(result.fieldErrors["anthropic.thinking.budgetTokens"]).toContain(
				"must be a valid integer",
			);
		});
	});

	describe("Google provider", () => {
		it("builds Google provider options with thinking budget", () => {
			const result = buildModelConfigFromForm(
				"google",
				formWith({
					google: { thinkingConfig: { thinkingBudget: "4096" } },
				}),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.provider_options?.google).toEqual({
				thinking_config: { thinking_budget: 4096 },
			});
		});

		it("builds Google options with include_thoughts", () => {
			const result = buildModelConfigFromForm(
				"google",
				formWith({
					google: { thinkingConfig: { includeThoughts: "true" } },
				}),
			);
			expect(result.fieldErrors).toEqual({});
			const google = result.modelConfig?.provider_options?.google as Record<
				string,
				unknown
			>;
			expect(google.thinking_config).toEqual({ include_thoughts: true });
		});

		it("builds Google options with both thinking fields", () => {
			const result = buildModelConfigFromForm(
				"google",
				formWith({
					google: {
						thinkingConfig: {
							thinkingBudget: "2048",
							includeThoughts: "false",
						},
					},
				}),
			);
			expect(result.fieldErrors).toEqual({});
			const google = result.modelConfig?.provider_options?.google as Record<
				string,
				unknown
			>;
			expect(google.thinking_config).toEqual({
				thinking_budget: 2048,
				include_thoughts: false,
			});
		});

		it("builds Google options with cached_content", () => {
			const result = buildModelConfigFromForm(
				"google",
				formWith({ google: { cachedContent: "cache-abc" } }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.provider_options?.google).toEqual({
				cached_content: "cache-abc",
			});
		});

		it("builds Google options with valid safety settings JSON array", () => {
			const settings = [{ category: "harm", threshold: "block" }];
			const result = buildModelConfigFromForm(
				"google",
				formWith({ google: { safetySettings: JSON.stringify(settings) } }),
			);
			expect(result.fieldErrors).toEqual({});
			const google = result.modelConfig?.provider_options?.google as Record<
				string,
				unknown
			>;
			expect(google.safety_settings).toEqual(settings);
		});

		it("reports error for invalid JSON in safety settings", () => {
			const result = buildModelConfigFromForm(
				"google",
				formWith({ google: { safetySettings: "not-json" } }),
			);
			expect(result.fieldErrors["google.safetySettings"]).toContain(
				"must be valid JSON",
			);
		});

		it("reports error when safety settings JSON is an object (not array)", () => {
			const result = buildModelConfigFromForm(
				"google",
				formWith({ google: { safetySettings: '{"key":"value"}' } }),
			);
			expect(result.fieldErrors["google.safetySettings"]).toContain(
				"must be a JSON array",
			);
		});

		it("reports error for non-numeric thinking budget", () => {
			const result = buildModelConfigFromForm(
				"google",
				formWith({
					google: { thinkingConfig: { thinkingBudget: "abc" } },
				}),
			);
			expect(
				result.fieldErrors["google.thinkingConfig.thinkingBudget"],
			).toContain("must be a valid integer");
		});
	});

	describe("OpenAI-compatible provider", () => {
		it("builds openaicompat provider options", () => {
			const result = buildModelConfigFromForm(
				"openaicompat",
				formWith({
					openaicompat: {
						reasoningEffort: "low",
						user: "compat-user",
					},
				}),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.provider_options?.openaicompat).toEqual({
				reasoning_effort: "low",
				user: "compat-user",
			});
		});

		it("reports error for invalid reasoning effort", () => {
			const result = buildModelConfigFromForm(
				"openaicompat",
				formWith({ openaicompat: { reasoningEffort: "super" } }),
			);
			expect(result.fieldErrors["openaicompat.reasoningEffort"]).toContain(
				"invalid value",
			);
		});

		it("does not set provider_options when all fields empty", () => {
			const result = buildModelConfigFromForm(
				"openaicompat",
				formWith({ temperature: "0.5" }),
			);
			expect(result.modelConfig?.provider_options).toBeUndefined();
		});
	});

	describe("OpenRouter provider", () => {
		it("builds OpenRouter options with reasoning", () => {
			const result = buildModelConfigFromForm(
				"openrouter",
				formWith({
					openrouter: {
						reasoning: {
							enabled: "true",
							effort: "high",
							maxTokens: "500",
							exclude: "false",
						},
					},
				}),
			);
			expect(result.fieldErrors).toEqual({});
			const openrouter = result.modelConfig?.provider_options
				?.openrouter as Record<string, unknown>;
			expect(openrouter.reasoning).toEqual({
				enabled: true,
				effort: "high",
				max_tokens: 500,
				exclude: false,
			});
		});

		it("builds OpenRouter options with parallel tool calls and user", () => {
			const result = buildModelConfigFromForm(
				"openrouter",
				formWith({
					openrouter: {
						parallelToolCalls: "true",
						user: "router-user",
					},
				}),
			);
			expect(result.fieldErrors).toEqual({});
			const openrouter = result.modelConfig?.provider_options
				?.openrouter as Record<string, unknown>;
			expect(openrouter.parallel_tool_calls).toBe(true);
			expect(openrouter.user).toBe("router-user");
		});

		it("builds OpenRouter options with include_usage", () => {
			const result = buildModelConfigFromForm(
				"openrouter",
				formWith({ openrouter: { includeUsage: "true" } }),
			);
			expect(result.fieldErrors).toEqual({});
			const openrouter = result.modelConfig?.provider_options
				?.openrouter as Record<string, unknown>;
			expect(openrouter.include_usage).toBe(true);
		});

		it("reports error for invalid reasoning effort", () => {
			const result = buildModelConfigFromForm(
				"openrouter",
				formWith({
					openrouter: { reasoning: { effort: "turbo" } },
				}),
			);
			expect(result.fieldErrors["openrouter.reasoning.effort"]).toContain(
				"invalid value",
			);
		});

		it("reports error for invalid boolean in reasoning enabled", () => {
			const result = buildModelConfigFromForm(
				"openrouter",
				formWith({
					openrouter: { reasoning: { enabled: "yes" } },
				}),
			);
			expect(result.fieldErrors["openrouter.reasoning.enabled"]).toContain(
				"must be true or false",
			);
		});
	});

	describe("Vercel provider", () => {
		it("builds Vercel options with reasoning", () => {
			const result = buildModelConfigFromForm(
				"vercel",
				formWith({
					vercel: {
						reasoning: {
							enabled: "true",
							effort: "medium",
							maxTokens: "1000",
							exclude: "true",
						},
					},
				}),
			);
			expect(result.fieldErrors).toEqual({});
			const vercel = result.modelConfig?.provider_options?.vercel as Record<
				string,
				unknown
			>;
			expect(vercel.reasoning).toEqual({
				enabled: true,
				effort: "medium",
				max_tokens: 1000,
				exclude: true,
			});
		});

		it("builds Vercel options with parallel tool calls and user", () => {
			const result = buildModelConfigFromForm(
				"vercel",
				formWith({
					vercel: {
						parallelToolCalls: "false",
						user: "vercel-user",
					},
				}),
			);
			expect(result.fieldErrors).toEqual({});
			const vercel = result.modelConfig?.provider_options?.vercel as Record<
				string,
				unknown
			>;
			expect(vercel.parallel_tool_calls).toBe(false);
			expect(vercel.user).toBe("vercel-user");
		});

		it("does not set provider_options when all Vercel fields empty", () => {
			const result = buildModelConfigFromForm(
				"vercel",
				formWith({ temperature: "1.0" }),
			);
			expect(result.modelConfig?.provider_options).toBeUndefined();
		});
	});

	describe("provider normalization", () => {
		it("handles null provider", () => {
			const result = buildModelConfigFromForm(
				null,
				formWith({ temperature: "0.5" }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.temperature).toBe(0.5);
			expect(result.modelConfig?.provider_options).toBeUndefined();
		});

		it("handles undefined provider", () => {
			const result = buildModelConfigFromForm(
				undefined,
				formWith({ temperature: "0.5" }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.temperature).toBe(0.5);
		});

		it("normalizes provider case (e.g. 'OpenAI' → 'openai')", () => {
			const result = buildModelConfigFromForm(
				"OpenAI",
				formWith({ openai: { reasoningEffort: "high" } }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.provider_options?.openai).toBeDefined();
		});

		it("trims provider whitespace", () => {
			const result = buildModelConfigFromForm(
				"  anthropic  ",
				formWith({ anthropic: { effort: "low" } }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.provider_options?.anthropic).toBeDefined();
		});

		it("ignores provider-specific fields for unknown providers", () => {
			const result = buildModelConfigFromForm(
				"unknown-provider",
				formWith({
					temperature: "0.5",
					openai: { reasoningEffort: "high" },
				}),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.temperature).toBe(0.5);
			// The OpenAI field is ignored because the provider is unknown.
			expect(result.modelConfig?.provider_options).toBeUndefined();
		});
	});

	describe("multiple validation errors", () => {
		it("collects errors for multiple invalid fields", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({
					maxOutputTokens: "not-a-number",
					temperature: "hot",
					topP: "invalid",
				}),
			);
			expect(result.fieldErrors.maxOutputTokens).toBeDefined();
			expect(result.fieldErrors.temperature).toBeDefined();
			expect(result.fieldErrors.topP).toBeDefined();
			expect(result.modelConfig).toBeUndefined();
		});

		it("returns fieldErrors without modelConfig when there are errors", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ temperature: "bad" }),
			);
			expect(Object.keys(result.fieldErrors).length).toBeGreaterThan(0);
			expect(result.modelConfig).toBeUndefined();
		});
	});

	describe("whitespace handling", () => {
		it("trims whitespace from numeric fields", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ maxOutputTokens: "  4096  " }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.max_output_tokens).toBe(4096);
		});

		it("trims whitespace from string fields", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ openai: { user: "  user-123  " } }),
			);
			expect(result.fieldErrors).toEqual({});
			const openai = result.modelConfig?.provider_options?.openai as Record<
				string,
				unknown
			>;
			expect(openai.user).toBe("user-123");
		});

		it("treats whitespace-only as empty", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ maxOutputTokens: "   " }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig).toBeUndefined();
		});
	});
});
