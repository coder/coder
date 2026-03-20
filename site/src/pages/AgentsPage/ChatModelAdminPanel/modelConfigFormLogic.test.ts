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
		expect(parsePositiveInteger("  42  ")).toBe(42);
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
		expect(parsePositiveInteger("3.9")).toBe(3);
		expect(parsePositiveInteger("1.1")).toBe(1);
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
		expect(parseThresholdInteger("  70  ")).toBe(70);
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

	it("extracts pricing fields", () => {
		const model: TypesGen.ChatModelConfig = {
			...baseChatModelConfig,
			model_config: {
				cost: {
					input_price_per_million_tokens: "0.15",
					output_price_per_million_tokens: "0.6",
					cache_read_price_per_million_tokens: "0.03",
					cache_write_price_per_million_tokens: "0.3",
				},
			},
		};
		const result = extractModelConfigFormState(model);
		expect(deepGet(result, ["cost", "inputPricePerMillionTokens"])).toBe(
			"0.15",
		);
		expect(deepGet(result, ["cost", "outputPricePerMillionTokens"])).toBe(
			"0.6",
		);
		expect(deepGet(result, ["cost", "cacheReadPricePerMillionTokens"])).toBe(
			"0.03",
		);
		expect(deepGet(result, ["cost", "cacheWritePricePerMillionTokens"])).toBe(
			"0.3",
		);
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
						prompt_cache_key: "my-cache-key",
					},
				},
			},
		};
		const result = extractModelConfigFormState(model);
		const openai = result.openai as Record<string, unknown>;
		expect(openai.reasoningEffort).toBe("high");
		expect(openai.parallelToolCalls).toBe("true");
		expect(openai.textVerbosity).toBe("medium");
		expect(openai.serviceTier).toBe("auto");
		expect(openai.reasoningSummary).toBe("concise");
		expect(openai.user).toBe("test-user");
		expect(openai.promptCacheKey).toBe("my-cache-key");
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
		const anthropic = result.anthropic as Record<string, unknown>;
		expect(anthropic.effort).toBe("high");
		expect(deepGet(anthropic, ["thinking", "budgetTokens"])).toBe("1024");
		expect(anthropic.sendReasoning).toBe("true");
		expect(anthropic.disableParallelToolUse).toBe("false");
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
		const google = result.google as Record<string, unknown>;
		expect(deepGet(google, ["thinkingConfig", "thinkingBudget"])).toBe("2048");
		expect(deepGet(google, ["thinkingConfig", "includeThoughts"])).toBe("true");
		expect(google.cachedContent).toBe("cache-123");
		expect(google.safetySettings).toBe(JSON.stringify(safetySettings, null, 2));
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
		const google = result.google as Record<string, unknown>;
		expect(google.safetySettings).toBe("");
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
		const openaicompat = result.openaicompat as Record<string, unknown>;
		expect(openaicompat.reasoningEffort).toBe("low");
		expect(openaicompat.user).toBe("compat-user");
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
		const openrouter = result.openrouter as Record<string, unknown>;
		expect(deepGet(openrouter, ["reasoning", "enabled"])).toBe("true");
		expect(deepGet(openrouter, ["reasoning", "effort"])).toBe("medium");
		expect(deepGet(openrouter, ["reasoning", "maxTokens"])).toBe("500");
		expect(deepGet(openrouter, ["reasoning", "exclude"])).toBe("false");
		expect(openrouter.parallelToolCalls).toBe("true");
		expect(openrouter.includeUsage).toBe("true");
		expect(openrouter.user).toBe("router-user");
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
		const vercel = result.vercel as Record<string, unknown>;
		expect(deepGet(vercel, ["reasoning", "enabled"])).toBe("false");
		expect(deepGet(vercel, ["reasoning", "effort"])).toBe("high");
		expect(deepGet(vercel, ["reasoning", "maxTokens"])).toBe("1000");
		expect(deepGet(vercel, ["reasoning", "exclude"])).toBe("true");
		expect(vercel.parallelToolCalls).toBe("false");
		expect(vercel.user).toBe("vercel-user");
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
		it("builds config with valid pricing fields", () => {
			const result = buildModelConfigFromForm(
				"openai",
				formWith({
					cost: {
						inputPricePerMillionTokens: "0.15",
						outputPricePerMillionTokens: "0.6",
						cacheReadPricePerMillionTokens: "0.03",
						cacheWritePricePerMillionTokens: "0.3",
					},
				}),
			);
			expect(result.fieldErrors).toEqual({});
			expect(result.modelConfig).toMatchObject({
				cost: {
					input_price_per_million_tokens: "0.15",
					output_price_per_million_tokens: "0.6",
					cache_read_price_per_million_tokens: "0.03",
					cache_write_price_per_million_tokens: "0.3",
				},
			});
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
