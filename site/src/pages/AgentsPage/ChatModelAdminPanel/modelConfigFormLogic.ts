import type * as TypesGen from "api/typesGenerated";
import { normalizeProvider } from "./helpers";
import {
	modelConfigAnthropicEffortOptions,
	modelConfigReasoningEffortOptions,
	modelConfigTextVerbosityOptions,
} from "./ModelConfigFields";

// ── Form state types ───────────────────────────────────────────

export type ModelConfigFormState = {
	maxOutputTokens: string;
	temperature: string;
	topP: string;
	topK: string;
	presencePenalty: string;
	frequencyPenalty: string;

	openaiReasoningEffort: string;
	openaiParallelToolCalls: string;
	openaiTextVerbosity: string;
	openaiServiceTier: string;
	openaiReasoningSummary: string;
	openaiUser: string;

	anthropicEffort: string;
	anthropicThinkingBudgetTokens: string;
	anthropicSendReasoning: string;
	anthropicDisableParallelToolUse: string;

	googleThinkingBudget: string;
	googleIncludeThoughts: string;
	googleCachedContent: string;
	googleSafetySettingsJSON: string;

	openAICompatReasoningEffort: string;
	openAICompatUser: string;

	openrouterReasoningEnabled: string;
	openrouterReasoningEffort: string;
	openrouterReasoningMaxTokens: string;
	openrouterReasoningExclude: string;
	openrouterParallelToolCalls: string;
	openrouterIncludeUsage: string;
	openrouterUser: string;

	vercelReasoningEnabled: string;
	vercelReasoningEffort: string;
	vercelReasoningMaxTokens: string;
	vercelReasoningExclude: string;
	vercelParallelToolCalls: string;
	vercelUser: string;
};

export type ModelConfigFormBuildResult = {
	modelConfig?: TypesGen.ChatModelCallConfig;
	fieldErrors: Partial<Record<keyof ModelConfigFormState, string>>;
};

export const emptyModelConfigFormState: ModelConfigFormState = {
	maxOutputTokens: "",
	temperature: "",
	topP: "",
	topK: "",
	presencePenalty: "",
	frequencyPenalty: "",

	openaiReasoningEffort: "",
	openaiParallelToolCalls: "",
	openaiTextVerbosity: "",
	openaiServiceTier: "",
	openaiReasoningSummary: "",
	openaiUser: "",

	anthropicEffort: "",
	anthropicThinkingBudgetTokens: "",
	anthropicSendReasoning: "",
	anthropicDisableParallelToolUse: "",

	googleThinkingBudget: "",
	googleIncludeThoughts: "",
	googleCachedContent: "",
	googleSafetySettingsJSON: "",

	openAICompatReasoningEffort: "",
	openAICompatUser: "",

	openrouterReasoningEnabled: "",
	openrouterReasoningEffort: "",
	openrouterReasoningMaxTokens: "",
	openrouterReasoningExclude: "",
	openrouterParallelToolCalls: "",
	openrouterIncludeUsage: "",
	openrouterUser: "",

	vercelReasoningEnabled: "",
	vercelReasoningEffort: "",
	vercelReasoningMaxTokens: "",
	vercelReasoningExclude: "",
	vercelParallelToolCalls: "",
	vercelUser: "",
};

// ── Helpers ────────────────────────────────────────────────────

const hasObjectKeys = (value: Record<string, unknown>): boolean =>
	Object.keys(value).length > 0;

export const parsePositiveInteger = (value: string): number | null => {
	const trimmed = value.trim();
	if (!trimmed) return null;
	const parsed = Number.parseInt(trimmed, 10);
	if (!Number.isFinite(parsed) || parsed <= 0) return null;
	return parsed;
};

export const parseThresholdInteger = (value: string): number | null => {
	const trimmed = value.trim();
	if (!trimmed) return null;
	const parsed = Number.parseInt(trimmed, 10);
	if (!Number.isFinite(parsed) || parsed < 0 || parsed > 100) return null;
	return parsed;
};

// ── Extract model config form state from an existing model ────

export const extractModelConfigFormState = (
	model: TypesGen.ChatModelConfig,
): ModelConfigFormState => {
	const config = model.model_config;
	if (!config) {
		return { ...emptyModelConfigFormState };
	}

	const str = (v: unknown): string =>
		v !== undefined && v !== null ? String(v) : "";
	const po = config.provider_options ?? {};

	// OpenAI / Azure options.
	const openai = (po.openai ?? {}) as Record<string, unknown>;
	// Anthropic / Bedrock options.
	const anthropic = (po.anthropic ?? {}) as Record<string, unknown>;
	const anthropicThinking = (anthropic.thinking ?? {}) as Record<
		string,
		unknown
	>;
	// Google options.
	const google = (po.google ?? {}) as Record<string, unknown>;
	const googleThinking = (google.thinking_config ?? {}) as Record<
		string,
		unknown
	>;
	// OpenAI-compatible options.
	const openaicompat = (po.openaicompat ?? {}) as Record<string, unknown>;
	// OpenRouter options.
	const openrouter = (po.openrouter ?? {}) as Record<string, unknown>;
	const openrouterReasoning = (openrouter.reasoning ?? {}) as Record<
		string,
		unknown
	>;
	// Vercel options.
	const vercel = (po.vercel ?? {}) as Record<string, unknown>;
	const vercelReasoning = (vercel.reasoning ?? {}) as Record<string, unknown>;

	return {
		maxOutputTokens: str(config.max_output_tokens),
		temperature: str(config.temperature),
		topP: str(config.top_p),
		topK: str(config.top_k),
		presencePenalty: str(config.presence_penalty),
		frequencyPenalty: str(config.frequency_penalty),

		openaiReasoningEffort: str(openai.reasoning_effort),
		openaiParallelToolCalls: str(openai.parallel_tool_calls),
		openaiTextVerbosity: str(openai.text_verbosity),
		openaiServiceTier: str(openai.service_tier),
		openaiReasoningSummary: str(openai.reasoning_summary),
		openaiUser: str(openai.user),

		anthropicEffort: str(anthropic.effort),
		anthropicThinkingBudgetTokens: str(anthropicThinking.budget_tokens),
		anthropicSendReasoning: str(anthropic.send_reasoning),
		anthropicDisableParallelToolUse: str(anthropic.disable_parallel_tool_use),

		googleThinkingBudget: str(googleThinking.thinking_budget),
		googleIncludeThoughts: str(googleThinking.include_thoughts),
		googleCachedContent: str(google.cached_content),
		googleSafetySettingsJSON: google.safety_settings
			? JSON.stringify(google.safety_settings, null, 2)
			: "",

		openAICompatReasoningEffort: str(openaicompat.reasoning_effort),
		openAICompatUser: str(openaicompat.user),

		openrouterReasoningEnabled: str(openrouterReasoning.enabled),
		openrouterReasoningEffort: str(openrouterReasoning.effort),
		openrouterReasoningMaxTokens: str(openrouterReasoning.max_tokens),
		openrouterReasoningExclude: str(openrouterReasoning.exclude),
		openrouterParallelToolCalls: str(openrouter.parallel_tool_calls),
		openrouterIncludeUsage: str(openrouter.include_usage),
		openrouterUser: str(openrouter.user),

		vercelReasoningEnabled: str(vercelReasoning.enabled),
		vercelReasoningEffort: str(vercelReasoning.effort),
		vercelReasoningMaxTokens: str(vercelReasoning.max_tokens),
		vercelReasoningExclude: str(vercelReasoning.exclude),
		vercelParallelToolCalls: str(vercel.parallel_tool_calls),
		vercelUser: str(vercel.user),
	};
};

// ── Form → model config builder ──────────────────────────────

/**
 * Shared builder for openrouter/vercel provider options. Both
 * providers use an identical reasoning + parallel_tool_calls + user
 * structure; only the field-name prefix differs.
 */
function buildReasoningProviderOptions(
	form: ModelConfigFormState,
	prefix: "openrouter" | "vercel",
	parseOptionalBoolean: (
		k: keyof ModelConfigFormState,
		l: string,
		v: string,
	) => boolean | undefined,
	parseOptionalSelect: (
		k: keyof ModelConfigFormState,
		l: string,
		v: string,
		o: readonly string[],
	) => string | undefined,
	parseOptionalInteger: (
		k: keyof ModelConfigFormState,
		l: string,
		v: string,
	) => number | undefined,
): Record<string, unknown> {
	const key = <K extends string>(suffix: K): keyof ModelConfigFormState =>
		`${prefix}${suffix}` as keyof ModelConfigFormState;

	const reasoningEnabled = parseOptionalBoolean(
		key("ReasoningEnabled"),
		"Reasoning enabled",
		form[key("ReasoningEnabled")],
	);
	const reasoningEffort = parseOptionalSelect(
		key("ReasoningEffort"),
		"Reasoning effort",
		form[key("ReasoningEffort")],
		modelConfigReasoningEffortOptions,
	);
	const reasoningMaxTokens = parseOptionalInteger(
		key("ReasoningMaxTokens"),
		"Reasoning max tokens",
		form[key("ReasoningMaxTokens")],
	);
	const reasoningExclude = parseOptionalBoolean(
		key("ReasoningExclude"),
		"Reasoning exclude",
		form[key("ReasoningExclude")],
	);
	const parallelToolCalls = parseOptionalBoolean(
		key("ParallelToolCalls"),
		"Parallel tool calls",
		form[key("ParallelToolCalls")],
	);
	const user = form[key("User")].trim();

	const reasoning: Record<string, unknown> = {
		...(reasoningEnabled !== undefined ? { enabled: reasoningEnabled } : {}),
		...(reasoningEffort ? { effort: reasoningEffort } : {}),
		...(reasoningMaxTokens !== undefined
			? { max_tokens: reasoningMaxTokens }
			: {}),
		...(reasoningExclude !== undefined ? { exclude: reasoningExclude } : {}),
	};
	return {
		...(hasObjectKeys(reasoning as Record<string, unknown>)
			? { reasoning }
			: {}),
		...(parallelToolCalls !== undefined
			? { parallel_tool_calls: parallelToolCalls }
			: {}),
		...(user ? { user } : {}),
	};
}

export const buildModelConfigFromForm = (
	provider: string | null | undefined,
	form: ModelConfigFormState,
): ModelConfigFormBuildResult => {
	const fieldErrors: Partial<Record<keyof ModelConfigFormState, string>> = {};

	const parseOptionalInteger = (
		fieldKey: keyof ModelConfigFormState,
		label: string,
		value: string,
	): number | undefined => {
		const trimmed = value.trim();
		if (!trimmed) return undefined;
		const parsed = Number.parseInt(trimmed, 10);
		if (!Number.isFinite(parsed)) {
			fieldErrors[fieldKey] = `${label} must be a valid number.`;
			return undefined;
		}
		return parsed;
	};

	const parseOptionalNumber = (
		fieldKey: keyof ModelConfigFormState,
		label: string,
		value: string,
	): number | undefined => {
		const trimmed = value.trim();
		if (!trimmed) return undefined;
		const parsed = Number(trimmed);
		if (!Number.isFinite(parsed)) {
			fieldErrors[fieldKey] = `${label} must be a valid number.`;
			return undefined;
		}
		return parsed;
	};

	const parseOptionalBoolean = (
		fieldKey: keyof ModelConfigFormState,
		label: string,
		value: string,
	): boolean | undefined => {
		const trimmed = value.trim();
		if (!trimmed) return undefined;
		if (trimmed !== "true" && trimmed !== "false") {
			fieldErrors[fieldKey] = `${label} must be true or false.`;
			return undefined;
		}
		return trimmed === "true";
	};

	const parseOptionalJSON = (
		fieldKey: keyof ModelConfigFormState,
		label: string,
		value: string,
	): unknown | undefined => {
		const trimmed = value.trim();
		if (!trimmed) return undefined;
		try {
			return JSON.parse(trimmed);
		} catch {
			fieldErrors[fieldKey] = `${label} must be valid JSON.`;
			return undefined;
		}
	};

	const parseOptionalSelect = (
		fieldKey: keyof ModelConfigFormState,
		label: string,
		value: string,
		options: readonly string[],
	): string | undefined => {
		const trimmed = value.trim();
		if (!trimmed) return undefined;
		if (!options.includes(trimmed)) {
			fieldErrors[fieldKey] = `${label} has an invalid value.`;
			return undefined;
		}
		return trimmed;
	};

	const maxOutputTokens = parseOptionalInteger(
		"maxOutputTokens",
		"Max output tokens",
		form.maxOutputTokens,
	);
	const temperature = parseOptionalNumber(
		"temperature",
		"Temperature",
		form.temperature,
	);
	const topP = parseOptionalNumber("topP", "Top P", form.topP);
	const topK = parseOptionalInteger("topK", "Top K", form.topK);
	const presencePenalty = parseOptionalNumber(
		"presencePenalty",
		"Presence penalty",
		form.presencePenalty,
	);
	const frequencyPenalty = parseOptionalNumber(
		"frequencyPenalty",
		"Frequency penalty",
		form.frequencyPenalty,
	);

	let providerOptions: TypesGen.ChatModelProviderOptions | undefined;
	const normalizedProvider = normalizeProvider(provider ?? "");

	switch (normalizedProvider) {
		case "openai":
		case "azure": {
			const reasoningEffort = parseOptionalSelect(
				"openaiReasoningEffort",
				"Reasoning effort",
				form.openaiReasoningEffort,
				modelConfigReasoningEffortOptions,
			);
			const parallelToolCalls = parseOptionalBoolean(
				"openaiParallelToolCalls",
				"Parallel tool calls",
				form.openaiParallelToolCalls,
			);
			const textVerbosity = parseOptionalSelect(
				"openaiTextVerbosity",
				"Text verbosity",
				form.openaiTextVerbosity,
				modelConfigTextVerbosityOptions,
			);
			const serviceTier = form.openaiServiceTier.trim();
			const reasoningSummary = form.openaiReasoningSummary.trim();
			const user = form.openaiUser.trim();
			const openaiOptions: Record<string, unknown> = {
				...(reasoningEffort ? { reasoning_effort: reasoningEffort } : {}),
				...(parallelToolCalls !== undefined
					? { parallel_tool_calls: parallelToolCalls }
					: {}),
				...(textVerbosity ? { text_verbosity: textVerbosity } : {}),
				...(serviceTier ? { service_tier: serviceTier } : {}),
				...(reasoningSummary ? { reasoning_summary: reasoningSummary } : {}),
				...(user ? { user } : {}),
			};
			if (hasObjectKeys(openaiOptions as Record<string, unknown>)) {
				providerOptions = { openai: openaiOptions };
			}
			break;
		}
		case "anthropic":
		case "bedrock": {
			const budgetTokens = parseOptionalInteger(
				"anthropicThinkingBudgetTokens",
				"Thinking budget tokens",
				form.anthropicThinkingBudgetTokens,
			);
			const sendReasoning = parseOptionalBoolean(
				"anthropicSendReasoning",
				"Send reasoning",
				form.anthropicSendReasoning,
			);
			const effort = parseOptionalSelect(
				"anthropicEffort",
				"Output effort",
				form.anthropicEffort,
				modelConfigAnthropicEffortOptions,
			);
			const disableParallelToolUse = parseOptionalBoolean(
				"anthropicDisableParallelToolUse",
				"Disable parallel tool use",
				form.anthropicDisableParallelToolUse,
			);
			const anthropicOptions: Record<string, unknown> = {
				...(effort ? { effort } : {}),
				...(budgetTokens !== undefined
					? { thinking: { budget_tokens: budgetTokens } }
					: {}),
				...(sendReasoning !== undefined
					? { send_reasoning: sendReasoning }
					: {}),
				...(disableParallelToolUse !== undefined
					? { disable_parallel_tool_use: disableParallelToolUse }
					: {}),
			};
			if (hasObjectKeys(anthropicOptions as Record<string, unknown>)) {
				providerOptions = { anthropic: anthropicOptions };
			}
			break;
		}
		case "google": {
			const thinkingBudget = parseOptionalInteger(
				"googleThinkingBudget",
				"Thinking budget",
				form.googleThinkingBudget,
			);
			const includeThoughts = parseOptionalBoolean(
				"googleIncludeThoughts",
				"Include thoughts",
				form.googleIncludeThoughts,
			);
			const cachedContent = form.googleCachedContent.trim();
			const safetySettings = parseOptionalJSON(
				"googleSafetySettingsJSON",
				"Safety settings JSON",
				form.googleSafetySettingsJSON,
			);
			let typedSafetySettings: unknown[] | undefined;
			if (safetySettings !== undefined) {
				if (Array.isArray(safetySettings)) {
					typedSafetySettings = safetySettings;
				} else {
					fieldErrors.googleSafetySettingsJSON =
						"Safety settings JSON must be an array.";
				}
			}
			const googleOptions: Record<string, unknown> = {
				...(thinkingBudget !== undefined || includeThoughts !== undefined
					? {
							thinking_config: {
								...(thinkingBudget !== undefined
									? { thinking_budget: thinkingBudget }
									: {}),
								...(includeThoughts !== undefined
									? { include_thoughts: includeThoughts }
									: {}),
							},
						}
					: {}),
				...(cachedContent ? { cached_content: cachedContent } : {}),
				...(typedSafetySettings
					? { safety_settings: typedSafetySettings }
					: {}),
			};
			if (hasObjectKeys(googleOptions as Record<string, unknown>)) {
				providerOptions = { google: googleOptions };
			}
			break;
		}
		case "openaicompat": {
			const reasoningEffort = parseOptionalSelect(
				"openAICompatReasoningEffort",
				"Reasoning effort",
				form.openAICompatReasoningEffort,
				modelConfigReasoningEffortOptions,
			);
			const user = form.openAICompatUser.trim();
			const opts: Record<string, unknown> = {
				...(reasoningEffort ? { reasoning_effort: reasoningEffort } : {}),
				...(user ? { user } : {}),
			};
			if (hasObjectKeys(opts as Record<string, unknown>)) {
				providerOptions = { openaicompat: opts };
			}
			break;
		}
		case "openrouter": {
			const opts = buildReasoningProviderOptions(
				form,
				"openrouter",
				parseOptionalBoolean,
				parseOptionalSelect,
				parseOptionalInteger,
			);
			// OpenRouter additionally supports include_usage.
			const includeUsage = parseOptionalBoolean(
				"openrouterIncludeUsage",
				"Include usage",
				form.openrouterIncludeUsage,
			);
			if (includeUsage !== undefined) {
				opts.include_usage = includeUsage;
			}
			if (hasObjectKeys(opts as Record<string, unknown>)) {
				providerOptions = { openrouter: opts };
			}
			break;
		}
		case "vercel": {
			const opts = buildReasoningProviderOptions(
				form,
				"vercel",
				parseOptionalBoolean,
				parseOptionalSelect,
				parseOptionalInteger,
			);
			if (hasObjectKeys(opts as Record<string, unknown>)) {
				providerOptions = { vercel: opts };
			}
			break;
		}
	}

	if (Object.keys(fieldErrors).length > 0) {
		return { fieldErrors };
	}
	const modelConfig: TypesGen.ChatModelCallConfig = {
		...(maxOutputTokens !== undefined
			? { max_output_tokens: maxOutputTokens }
			: {}),
		...(temperature !== undefined ? { temperature } : {}),
		...(topP !== undefined ? { top_p: topP } : {}),
		...(topK !== undefined ? { top_k: topK } : {}),
		...(presencePenalty !== undefined
			? { presence_penalty: presencePenalty }
			: {}),
		...(frequencyPenalty !== undefined
			? { frequency_penalty: frequencyPenalty }
			: {}),
		...(providerOptions ? { provider_options: providerOptions } : {}),
	};
	if (!hasObjectKeys(modelConfig as Record<string, unknown>)) {
		return { fieldErrors: {} };
	}
	return { modelConfig, fieldErrors: {} };
};
