import type * as TypesGen from "api/typesGenerated";
import { describe, expect, it } from "vitest";
import {
	buildModelConfigFromForm,
	emptyModelConfigFormState,
	extractModelConfigFormState,
	type ModelConfigFormState,
	parsePositiveInteger,
	parseThresholdInteger,
} from "./modelConfigFormLogic";

// ── Helpers ────────────────────────────────────────────────────

/** Return an empty form with the given overrides applied. */
const formWith = (
	overrides: Partial<ModelConfigFormState>,
): ModelConfigFormState => ({
	...emptyModelConfigFormState,
	...overrides,
});

/** Minimal ChatModelConfig with no model_config. */
const baseChatModelConfig: TypesGen.ChatModelConfig = {
	id: "test-id",
	provider: "openai",
	model: "gpt-4",
	display_name: "GPT-4",
	enabled: true,
	context_limit: 128000,
	compression_threshold: 80,
	created_at: "2025-01-01T00:00:00Z",
	updated_at: "2025-01-01T00:00:00Z",
};

// ── parsePositiveInteger ───────────────────────────────────────

describe("parsePositiveInteger", () => {
	it("returns null for empty string", () => {
		expect(parsePositiveInteger("")).toBeNull();
	});

	it("returns null for whitespace-only string", () => {
		expect(parsePositiveInteger("   ")).toBeNull();
	});

	it("parses a valid positive integer", () => {
		expect(parsePositiveInteger("42")).toBe(42);
	});

	it("parses a string with surrounding whitespace", () => {
		expect(parsePositiveInteger("  100  ")).toBe(100);
	});

	it("returns null for zero", () => {
		expect(parsePositiveInteger("0")).toBeNull();
	});

	it("returns null for negative numbers", () => {
		expect(parsePositiveInteger("-5")).toBeNull();
	});

	it("returns null for non-numeric strings", () => {
		expect(parsePositiveInteger("abc")).toBeNull();
	});

	it("returns null for Infinity", () => {
		expect(parsePositiveInteger("Infinity")).toBeNull();
	});

	it("truncates floating point values to integer", () => {
		// parseInt("3.7", 10) → 3 which is > 0.
		expect(parsePositiveInteger("3.7")).toBe(3);
	});
});

// ── parseThresholdInteger ──────────────────────────────────────

describe("parseThresholdInteger", () => {
	it("returns null for empty string", () => {
		expect(parseThresholdInteger("")).toBeNull();
	});

	it("returns null for whitespace-only string", () => {
		expect(parseThresholdInteger("   ")).toBeNull();
	});

	it("parses 0 (lower bound)", () => {
		expect(parseThresholdInteger("0")).toBe(0);
	});

	it("parses 100 (upper bound)", () => {
		expect(parseThresholdInteger("100")).toBe(100);
	});

	it("parses a value in range", () => {
		expect(parseThresholdInteger("50")).toBe(50);
	});

	it("returns null for values above 100", () => {
		expect(parseThresholdInteger("101")).toBeNull();
	});

	it("returns null for negative values", () => {
		expect(parseThresholdInteger("-1")).toBeNull();
	});

	it("returns null for non-numeric strings", () => {
		expect(parseThresholdInteger("abc")).toBeNull();
	});

	it("trims whitespace before parsing", () => {
		expect(parseThresholdInteger("  75  ")).toBe(75);
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
		const model: TypesGen.ChatModelConfig = {
			...baseChatModelConfig,
			model_config: {
				max_output_tokens: 4096,
				temperature: 0.7,
				top_p: 0.9,
				top_k: 40,
				presence_penalty: 0.5,
				frequency_penalty: 0.3,
			},
		};
		const result = extractModelConfigFormState(model);
		expect(result.maxOutputTokens).toBe("4096");
		expect(result.temperature).toBe("0.7");
		expect(result.topP).toBe("0.9");
		expect(result.topK).toBe("40");
		expect(result.presencePenalty).toBe("0.5");
		expect(result.frequencyPenalty).toBe("0.3");
	});

	it("extracts OpenAI provider options", () => {
		const model: TypesGen.ChatModelConfig = {
			...baseChatModelConfig,
			model_config: {
				provider_options: {
					openai: {
						reasoning_effort: "high",
						parallel_tool_calls: true,
						text_verbosity: "medium",
						service_tier: "auto",
						reasoning_summary: "concise",
						user: "test-user",
					},
				},
			},
		};
		const result = extractModelConfigFormState(model);
		expect(result.openaiReasoningEffort).toBe("high");
		expect(result.openaiParallelToolCalls).toBe("true");
		expect(result.openaiTextVerbosity).toBe("medium");
		expect(result.openaiServiceTier).toBe("auto");
		expect(result.openaiReasoningSummary).toBe("concise");
		expect(result.openaiUser).toBe("test-user");
	});

	it("extracts Anthropic provider options with thinking", () => {
		const model: TypesGen.ChatModelConfig = {
			...baseChatModelConfig,
			model_config: {
				provider_options: {
					anthropic: {
						effort: "high",
						thinking: { budget_tokens: 1024 },
						send_reasoning: true,
						disable_parallel_tool_use: false,
					},
				},
			},
		};
		const result = extractModelConfigFormState(model);
		expect(result.anthropicEffort).toBe("high");
		expect(result.anthropicThinkingBudgetTokens).toBe("1024");
		expect(result.anthropicSendReasoning).toBe("true");
		expect(result.anthropicDisableParallelToolUse).toBe("false");
	});

	it("extracts Google provider options with safety settings", () => {
		const safetySettings = [{ category: "harm", threshold: "block" }];
		const model: TypesGen.ChatModelConfig = {
			...baseChatModelConfig,
			model_config: {
				provider_options: {
					google: {
						thinking_config: {
							thinking_budget: 2048,
							include_thoughts: true,
						},
						cached_content: "cache-123",
						safety_settings: safetySettings,
					},
				},
			},
		};
		const result = extractModelConfigFormState(model);
		expect(result.googleThinkingBudget).toBe("2048");
		expect(result.googleIncludeThoughts).toBe("true");
		expect(result.googleCachedContent).toBe("cache-123");
		expect(result.googleSafetySettingsJSON).toBe(
			JSON.stringify(safetySettings, null, 2),
		);
	});

	it("returns empty string for google safety settings when absent", () => {
		const model: TypesGen.ChatModelConfig = {
			...baseChatModelConfig,
			model_config: {
				provider_options: {
					google: {},
				},
			},
		};
		const result = extractModelConfigFormState(model);
		expect(result.googleSafetySettingsJSON).toBe("");
	});

	it("extracts OpenAI-compatible provider options", () => {
		const model: TypesGen.ChatModelConfig = {
			...baseChatModelConfig,
			model_config: {
				provider_options: {
					openaicompat: {
						reasoning_effort: "low",
						user: "compat-user",
					},
				},
			},
		};
		const result = extractModelConfigFormState(model);
		expect(result.openAICompatReasoningEffort).toBe("low");
		expect(result.openAICompatUser).toBe("compat-user");
	});

	it("extracts OpenRouter provider options", () => {
		const model: TypesGen.ChatModelConfig = {
			...baseChatModelConfig,
			model_config: {
				provider_options: {
					openrouter: {
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
				},
			},
		};
		const result = extractModelConfigFormState(model);
		expect(result.openrouterReasoningEnabled).toBe("true");
		expect(result.openrouterReasoningEffort).toBe("medium");
		expect(result.openrouterReasoningMaxTokens).toBe("500");
		expect(result.openrouterReasoningExclude).toBe("false");
		expect(result.openrouterParallelToolCalls).toBe("true");
		expect(result.openrouterIncludeUsage).toBe("true");
		expect(result.openrouterUser).toBe("router-user");
	});

	it("extracts Vercel provider options", () => {
		const model: TypesGen.ChatModelConfig = {
			...baseChatModelConfig,
			model_config: {
				provider_options: {
					vercel: {
						reasoning: {
							enabled: false,
							effort: "high",
							max_tokens: 1000,
							exclude: true,
						},
						parallel_tool_calls: false,
						user: "vercel-user",
					},
				},
			},
		};
		const result = extractModelConfigFormState(model);
		expect(result.vercelReasoningEnabled).toBe("false");
		expect(result.vercelReasoningEffort).toBe("high");
		expect(result.vercelReasoningMaxTokens).toBe("1000");
		expect(result.vercelReasoningExclude).toBe("true");
		expect(result.vercelParallelToolCalls).toBe("false");
		expect(result.vercelUser).toBe("vercel-user");
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
		expect(result.openaiReasoningEffort).toBe("");
		expect(result.anthropicEffort).toBe("");
		expect(result.googleThinkingBudget).toBe("");
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
		it("builds config with valid maxOutputTokens", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ maxOutputTokens: "4096" }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.max_output_tokens).toBe(4096);
		});

		it("builds config with valid temperature", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ temperature: "0.7" }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.temperature).toBe(0.7);
		});

		it("builds config with valid topP", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ topP: "0.95" }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.top_p).toBe(0.95);
		});

		it("builds config with valid topK", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ topK: "40" }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.top_k).toBe(40);
		});

		it("builds config with presencePenalty and frequencyPenalty", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ presencePenalty: "0.5", frequencyPenalty: "0.3" }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.presence_penalty).toBe(0.5);
			expect(result.modelConfig?.frequency_penalty).toBe(0.3);
		});

		it("reports error for non-numeric maxOutputTokens", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ maxOutputTokens: "abc" }),
			);
			expect(result.fieldErrors.maxOutputTokens).toBe(
				"Max output tokens must be a valid number.",
			);
			expect(result.modelConfig).toBeUndefined();
		});

		it("reports error for non-numeric temperature", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ temperature: "hot" }),
			);
			expect(result.fieldErrors.temperature).toBe(
				"Temperature must be a valid number.",
			);
		});

		it("reports error for non-numeric topP", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ topP: "not-a-number" }),
			);
			expect(result.fieldErrors.topP).toBe("Top P must be a valid number.");
		});

		it("reports error for non-numeric topK", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ topK: "xyz" }),
			);
			expect(result.fieldErrors.topK).toBe("Top K must be a valid number.");
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

	describe("OpenAI / Azure provider", () => {
		it("builds OpenAI provider options with reasoning effort", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ openaiReasoningEffort: "high" }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.provider_options?.openai).toEqual({
				reasoning_effort: "high",
			});
		});

		it("builds Azure provider options (same as OpenAI)", () => {
			const result = buildModelConfigFromForm(
				"azure",
				formWith({ openaiParallelToolCalls: "true" }),
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
					openaiReasoningEffort: "medium",
					openaiParallelToolCalls: "false",
					openaiTextVerbosity: "low",
					openaiServiceTier: "auto",
					openaiReasoningSummary: "concise",
					openaiUser: "user-123",
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
		});

		it("reports error for invalid reasoning effort option", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ openaiReasoningEffort: "invalid_value" }),
			);
			expect(result.fieldErrors.openaiReasoningEffort).toBe(
				"Reasoning effort has an invalid value.",
			);
		});

		it("reports error for invalid parallel tool calls boolean", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ openaiParallelToolCalls: "maybe" }),
			);
			expect(result.fieldErrors.openaiParallelToolCalls).toBe(
				"Parallel tool calls must be true or false.",
			);
		});

		it("reports error for invalid text verbosity option", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({ openaiTextVerbosity: "invalid" }),
			);
			expect(result.fieldErrors.openaiTextVerbosity).toBe(
				"Text verbosity has an invalid value.",
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
				formWith({ anthropicEffort: "high" }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.provider_options?.anthropic).toEqual({
				effort: "high",
			});
		});

		it("builds Bedrock provider options (same as Anthropic)", () => {
			const result = buildModelConfigFromForm(
				"bedrock",
				formWith({ anthropicSendReasoning: "true" }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.provider_options?.anthropic).toEqual({
				send_reasoning: true,
			});
		});

		it("builds Anthropic options with thinking budget", () => {
			const result = buildModelConfigFromForm(
				"anthropic",
				formWith({ anthropicThinkingBudgetTokens: "2048" }),
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
					anthropicEffort: "max",
					anthropicThinkingBudgetTokens: "1024",
					anthropicSendReasoning: "false",
					anthropicDisableParallelToolUse: "true",
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
				formWith({ anthropicEffort: "ultra" }),
			);
			expect(result.fieldErrors.anthropicEffort).toBe(
				"Output effort has an invalid value.",
			);
		});

		it("reports error for non-numeric thinking budget tokens", () => {
			const result = buildModelConfigFromForm(
				"anthropic",
				formWith({ anthropicThinkingBudgetTokens: "lots" }),
			);
			expect(result.fieldErrors.anthropicThinkingBudgetTokens).toBe(
				"Thinking budget tokens must be a valid number.",
			);
		});
	});

	describe("Google provider", () => {
		it("builds Google provider options with thinking budget", () => {
			const result = buildModelConfigFromForm(
				"google",
				formWith({ googleThinkingBudget: "4096" }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.provider_options?.google).toEqual({
				thinking_config: { thinking_budget: 4096 },
			});
		});

		it("builds Google options with include_thoughts", () => {
			const result = buildModelConfigFromForm(
				"google",
				formWith({ googleIncludeThoughts: "true" }),
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
					googleThinkingBudget: "2048",
					googleIncludeThoughts: "false",
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
				formWith({ googleCachedContent: "cache-abc" }),
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
				formWith({ googleSafetySettingsJSON: JSON.stringify(settings) }),
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
				formWith({ googleSafetySettingsJSON: "not-json" }),
			);
			expect(result.fieldErrors.googleSafetySettingsJSON).toBe(
				"Safety settings JSON must be valid JSON.",
			);
		});

		it("reports error when safety settings JSON is an object (not array)", () => {
			const result = buildModelConfigFromForm(
				"google",
				formWith({ googleSafetySettingsJSON: '{"key":"value"}' }),
			);
			expect(result.fieldErrors.googleSafetySettingsJSON).toBe(
				"Safety settings JSON must be an array.",
			);
		});

		it("reports error for non-numeric thinking budget", () => {
			const result = buildModelConfigFromForm(
				"google",
				formWith({ googleThinkingBudget: "abc" }),
			);
			expect(result.fieldErrors.googleThinkingBudget).toBe(
				"Thinking budget must be a valid number.",
			);
		});
	});

	describe("OpenAI-compatible provider", () => {
		it("builds openaicompat provider options", () => {
			const result = buildModelConfigFromForm(
				"openaicompat",
				formWith({
					openAICompatReasoningEffort: "low",
					openAICompatUser: "compat-user",
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
				formWith({ openAICompatReasoningEffort: "super" }),
			);
			expect(result.fieldErrors.openAICompatReasoningEffort).toBe(
				"Reasoning effort has an invalid value.",
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
					openrouterReasoningEnabled: "true",
					openrouterReasoningEffort: "high",
					openrouterReasoningMaxTokens: "500",
					openrouterReasoningExclude: "false",
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
					openrouterParallelToolCalls: "true",
					openrouterUser: "router-user",
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
				formWith({ openrouterIncludeUsage: "true" }),
			);
			expect(result.fieldErrors).toEqual({});
			const openrouter = result.modelConfig?.provider_options
				?.openrouter as Record<string, unknown>;
			expect(openrouter.include_usage).toBe(true);
		});

		it("reports error for invalid reasoning effort", () => {
			const result = buildModelConfigFromForm(
				"openrouter",
				formWith({ openrouterReasoningEffort: "turbo" }),
			);
			expect(result.fieldErrors.openrouterReasoningEffort).toBe(
				"Reasoning effort has an invalid value.",
			);
		});

		it("reports error for invalid boolean in reasoning enabled", () => {
			const result = buildModelConfigFromForm(
				"openrouter",
				formWith({ openrouterReasoningEnabled: "yes" }),
			);
			expect(result.fieldErrors.openrouterReasoningEnabled).toBe(
				"Reasoning enabled must be true or false.",
			);
		});
	});

	describe("Vercel provider", () => {
		it("builds Vercel options with reasoning", () => {
			const result = buildModelConfigFromForm(
				"vercel",
				formWith({
					vercelReasoningEnabled: "true",
					vercelReasoningEffort: "medium",
					vercelReasoningMaxTokens: "1000",
					vercelReasoningExclude: "true",
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
					vercelParallelToolCalls: "false",
					vercelUser: "vercel-user",
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
				formWith({ openaiReasoningEffort: "high" }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.provider_options?.openai).toBeDefined();
		});

		it("trims provider whitespace", () => {
			const result = buildModelConfigFromForm(
				"  anthropic  ",
				formWith({ anthropicEffort: "low" }),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig?.provider_options?.anthropic).toBeDefined();
		});

		it("ignores provider-specific fields for unknown providers", () => {
			const result = buildModelConfigFromForm(
				"unknown-provider",
				formWith({
					temperature: "0.5",
					openaiReasoningEffort: "high",
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
				formWith({ openaiUser: "  user-123  " }),
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
