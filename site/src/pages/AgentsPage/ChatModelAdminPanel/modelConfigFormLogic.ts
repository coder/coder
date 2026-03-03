import type * as TypesGen from "api/typesGenerated";
import * as Yup from "yup";
import { normalizeProvider } from "./helpers";
import {
	modelConfigAnthropicEffortOptions,
	modelConfigReasoningEffortOptions,
	modelConfigTextVerbosityOptions,
} from "./ModelConfigFields";

// ── Per-provider form state types ──────────────────────────────

export type OpenAIFormState = {
	reasoningEffort: string;
	parallelToolCalls: string;
	textVerbosity: string;
	serviceTier: string;
	reasoningSummary: string;
	user: string;
};

export type AnthropicFormState = {
	effort: string;
	thinkingBudgetTokens: string;
	sendReasoning: string;
	disableParallelToolUse: string;
};

export type GoogleFormState = {
	thinkingBudget: string;
	includeThoughts: string;
	cachedContent: string;
	safetySettingsJSON: string;
};

export type OpenAICompatFormState = {
	reasoningEffort: string;
	user: string;
};

export type OpenRouterFormState = {
	reasoningEnabled: string;
	reasoningEffort: string;
	reasoningMaxTokens: string;
	reasoningExclude: string;
	parallelToolCalls: string;
	includeUsage: string;
	user: string;
};

export type VercelFormState = {
	reasoningEnabled: string;
	reasoningEffort: string;
	reasoningMaxTokens: string;
	reasoningExclude: string;
	parallelToolCalls: string;
	user: string;
};

// ── Main form state type ───────────────────────────────────────

export type ModelConfigFormState = {
	maxOutputTokens: string;
	temperature: string;
	topP: string;
	topK: string;
	presencePenalty: string;
	frequencyPenalty: string;
	openai: OpenAIFormState;
	anthropic: AnthropicFormState;
	google: GoogleFormState;
	openaicompat: OpenAICompatFormState;
	openrouter: OpenRouterFormState;
	vercel: VercelFormState;
};

export type ModelConfigFormBuildResult = {
	modelConfig?: TypesGen.ChatModelCallConfig;
	fieldErrors: Record<string, string>;
};

// ── Empty defaults ─────────────────────────────────────────────

export const emptyOpenAIFormState: OpenAIFormState = {
	reasoningEffort: "",
	parallelToolCalls: "",
	textVerbosity: "",
	serviceTier: "",
	reasoningSummary: "",
	user: "",
};

export const emptyAnthropicFormState: AnthropicFormState = {
	effort: "",
	thinkingBudgetTokens: "",
	sendReasoning: "",
	disableParallelToolUse: "",
};

export const emptyGoogleFormState: GoogleFormState = {
	thinkingBudget: "",
	includeThoughts: "",
	cachedContent: "",
	safetySettingsJSON: "",
};

export const emptyOpenAICompatFormState: OpenAICompatFormState = {
	reasoningEffort: "",
	user: "",
};

export const emptyOpenRouterFormState: OpenRouterFormState = {
	reasoningEnabled: "",
	reasoningEffort: "",
	reasoningMaxTokens: "",
	reasoningExclude: "",
	parallelToolCalls: "",
	includeUsage: "",
	user: "",
};

export const emptyVercelFormState: VercelFormState = {
	reasoningEnabled: "",
	reasoningEffort: "",
	reasoningMaxTokens: "",
	reasoningExclude: "",
	parallelToolCalls: "",
	user: "",
};

export const emptyModelConfigFormState: ModelConfigFormState = {
	maxOutputTokens: "",
	temperature: "",
	topP: "",
	topK: "",
	presencePenalty: "",
	frequencyPenalty: "",
	openai: { ...emptyOpenAIFormState },
	anthropic: { ...emptyAnthropicFormState },
	google: { ...emptyGoogleFormState },
	openaicompat: { ...emptyOpenAICompatFormState },
	openrouter: { ...emptyOpenRouterFormState },
	vercel: { ...emptyVercelFormState },
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
		return structuredClone(emptyModelConfigFormState);
	}

	const toFormString = (v: unknown): string =>
		v !== undefined && v !== null ? String(v) : "";

	const po = config.provider_options;
	const openai = po?.openai;
	const anthropic = po?.anthropic;
	const google = po?.google;
	const openaicompat = po?.openaicompat;
	const openrouter = po?.openrouter;
	const vercel = po?.vercel;

	return {
		maxOutputTokens: toFormString(config.max_output_tokens),
		temperature: toFormString(config.temperature),
		topP: toFormString(config.top_p),
		topK: toFormString(config.top_k),
		presencePenalty: toFormString(config.presence_penalty),
		frequencyPenalty: toFormString(config.frequency_penalty),
		openai: {
			reasoningEffort: toFormString(openai?.reasoning_effort),
			parallelToolCalls: toFormString(openai?.parallel_tool_calls),
			textVerbosity: toFormString(openai?.text_verbosity),
			serviceTier: toFormString(openai?.service_tier),
			reasoningSummary: toFormString(openai?.reasoning_summary),
			user: toFormString(openai?.user),
		},
		anthropic: {
			effort: toFormString(anthropic?.effort),
			thinkingBudgetTokens: toFormString(anthropic?.thinking?.budget_tokens),
			sendReasoning: toFormString(anthropic?.send_reasoning),
			disableParallelToolUse: toFormString(
				anthropic?.disable_parallel_tool_use,
			),
		},
		google: {
			thinkingBudget: toFormString(google?.thinking_config?.thinking_budget),
			includeThoughts: toFormString(google?.thinking_config?.include_thoughts),
			cachedContent: toFormString(google?.cached_content),
			safetySettingsJSON: google?.safety_settings
				? JSON.stringify(google.safety_settings, null, 2)
				: "",
		},
		openaicompat: {
			reasoningEffort: toFormString(openaicompat?.reasoning_effort),
			user: toFormString(openaicompat?.user),
		},
		openrouter: {
			reasoningEnabled: toFormString(openrouter?.reasoning?.enabled),
			reasoningEffort: toFormString(openrouter?.reasoning?.effort),
			reasoningMaxTokens: toFormString(openrouter?.reasoning?.max_tokens),
			reasoningExclude: toFormString(openrouter?.reasoning?.exclude),
			parallelToolCalls: toFormString(openrouter?.parallel_tool_calls),
			includeUsage: toFormString(openrouter?.include_usage),
			user: toFormString(openrouter?.user),
		},
		vercel: {
			reasoningEnabled: toFormString(vercel?.reasoning?.enabled),
			reasoningEffort: toFormString(vercel?.reasoning?.effort),
			reasoningMaxTokens: toFormString(vercel?.reasoning?.max_tokens),
			reasoningExclude: toFormString(vercel?.reasoning?.exclude),
			parallelToolCalls: toFormString(vercel?.parallel_tool_calls),
			user: toFormString(vercel?.user),
		},
	};
};

// ── Unified form values type ─────────────────────────────────

export type ModelFormValues = {
	model: string;
	displayName: string;
	contextLimit: string;
	compressionThreshold: string;
	isDefault: boolean;
	config: ModelConfigFormState;
};

/**
 * Build initial form values from an editing model or defaults.
 */
export const buildInitialModelFormValues = (
	editingModel?: TypesGen.ChatModelConfig,
): ModelFormValues => ({
	model: editingModel?.model ?? "",
	displayName: editingModel?.display_name ?? "",
	contextLimit: editingModel ? String(editingModel.context_limit) : "",
	compressionThreshold: editingModel
		? String(editingModel.compression_threshold)
		: "",
	isDefault: editingModel?.is_default ?? false,
	config: editingModel
		? extractModelConfigFormState(editingModel)
		: structuredClone(emptyModelConfigFormState),
});

// ── Parsing utilities ─────────────────────────────────────────

type FieldErrors = Record<string, string>;

// ── Yup transforms ──────────────────────────────────────────

function yupOptionalInteger(label: string) {
	return Yup.string().test(
		"optional-integer",
		`${label} must be a valid number.`,
		(value) => {
			const trimmed = value?.trim();
			if (!trimmed) return true;
			return Number.isFinite(Number.parseInt(trimmed, 10));
		},
	);
}

function yupOptionalNumber(label: string) {
	return Yup.string().test(
		"optional-number",
		`${label} must be a valid number.`,
		(value) => {
			const trimmed = value?.trim();
			if (!trimmed) return true;
			return Number.isFinite(Number(trimmed));
		},
	);
}

function yupOptionalBoolean(label: string) {
	return Yup.string().test(
		"optional-boolean",
		`${label} must be true or false.`,
		(value) => {
			const trimmed = value?.trim();
			if (!trimmed) return true;
			return trimmed === "true" || trimmed === "false";
		},
	);
}

function yupOptionalSelect(label: string, options: readonly string[]) {
	return Yup.string().test(
		"optional-select",
		`${label} has an invalid value.`,
		(value) => {
			const trimmed = value?.trim();
			if (!trimmed) return true;
			return options.includes(trimmed);
		},
	);
}

function yupOptionalJSONArray(label: string) {
	return Yup.string().test("optional-json-array", "", function validate(value) {
		const trimmed = value?.trim();
		if (!trimmed) return true;
		let parsed: unknown;
		try {
			parsed = JSON.parse(trimmed);
		} catch {
			return this.createError({
				message: `${label} must be valid JSON.`,
			});
		}
		if (!Array.isArray(parsed)) {
			return this.createError({
				message: `${label} must be an array.`,
			});
		}
		return true;
	});
}

// ── Per-provider Yup schemas ─────────────────────────────────

const topLevelSchema = Yup.object({
	maxOutputTokens: yupOptionalInteger("Max output tokens"),
	temperature: yupOptionalNumber("Temperature"),
	topP: yupOptionalNumber("Top P"),
	topK: yupOptionalInteger("Top K"),
	presencePenalty: yupOptionalNumber("Presence penalty"),
	frequencyPenalty: yupOptionalNumber("Frequency penalty"),
});

const openaiSchema = Yup.object({
	reasoningEffort: yupOptionalSelect(
		"Reasoning effort",
		modelConfigReasoningEffortOptions,
	),
	parallelToolCalls: yupOptionalBoolean("Parallel tool calls"),
	textVerbosity: yupOptionalSelect(
		"Text verbosity",
		modelConfigTextVerbosityOptions,
	),
});

const anthropicSchema = Yup.object({
	effort: yupOptionalSelect("Output effort", modelConfigAnthropicEffortOptions),
	thinkingBudgetTokens: yupOptionalInteger("Thinking budget tokens"),
	sendReasoning: yupOptionalBoolean("Send reasoning"),
	disableParallelToolUse: yupOptionalBoolean("Disable parallel tool use"),
});

const googleSchema = Yup.object({
	thinkingBudget: yupOptionalInteger("Thinking budget"),
	includeThoughts: yupOptionalBoolean("Include thoughts"),
	safetySettingsJSON: yupOptionalJSONArray("Safety settings JSON"),
});

const openaiCompatSchema = Yup.object({
	reasoningEffort: yupOptionalSelect(
		"Reasoning effort",
		modelConfigReasoningEffortOptions,
	),
});

const reasoningProviderSchema = Yup.object({
	reasoningEnabled: yupOptionalBoolean("Reasoning enabled"),
	reasoningEffort: yupOptionalSelect(
		"Reasoning effort",
		modelConfigReasoningEffortOptions,
	),
	reasoningMaxTokens: yupOptionalInteger("Reasoning max tokens"),
	reasoningExclude: yupOptionalBoolean("Reasoning exclude"),
	parallelToolCalls: yupOptionalBoolean("Parallel tool calls"),
});

const openrouterExtraSchema = Yup.object({
	includeUsage: yupOptionalBoolean("Include usage"),
});

// ── Yup error collection ─────────────────────────────────────

function collectYupErrors(
	schema: Yup.ObjectSchema<Record<string, unknown>>,
	data: Record<string, unknown>,
	fieldErrors: FieldErrors,
	prefix?: string,
): void {
	try {
		schema.validateSync(data, { abortEarly: false });
	} catch (err) {
		if (err instanceof Yup.ValidationError) {
			for (const inner of err.inner) {
				const key = prefix ? `${prefix}.${inner.path}` : (inner.path ?? "");
				fieldErrors[key] = inner.message;
			}
		}
	}
}

// ── Post-validation transform helpers ────────────────────────
// These assume validation has already passed. Empty strings
// yield undefined so callers can conditionally include fields.

function toInt(s: string): number | undefined {
	const trimmed = s.trim();
	if (!trimmed) return undefined;
	return Number.parseInt(trimmed, 10);
}

function toNum(s: string): number | undefined {
	const trimmed = s.trim();
	if (!trimmed) return undefined;
	return Number(trimmed);
}

function toBool(s: string): boolean | undefined {
	const trimmed = s.trim();
	if (!trimmed) return undefined;
	return trimmed === "true";
}

function toTrimmedString(s: string): string | undefined {
	const trimmed = s.trim();
	return trimmed || undefined;
}

function toJSON(s: string): unknown | undefined {
	const trimmed = s.trim();
	if (!trimmed) return undefined;
	return JSON.parse(trimmed);
}

// ── Per-provider option builders ──────────────────────────────

/**
 * Build OpenAI/Azure provider options from form state.
 * Validation has already passed; uses transform helpers only.
 */
function buildOpenAIOptions(form: OpenAIFormState): Record<string, unknown> {
	const reasoningEffort = toTrimmedString(form.reasoningEffort);
	const parallelToolCalls = toBool(form.parallelToolCalls);
	const textVerbosity = toTrimmedString(form.textVerbosity);
	const serviceTier = toTrimmedString(form.serviceTier);
	const reasoningSummary = toTrimmedString(form.reasoningSummary);
	const user = toTrimmedString(form.user);

	return {
		...(reasoningEffort ? { reasoning_effort: reasoningEffort } : {}),
		...(parallelToolCalls !== undefined
			? { parallel_tool_calls: parallelToolCalls }
			: {}),
		...(textVerbosity ? { text_verbosity: textVerbosity } : {}),
		...(serviceTier ? { service_tier: serviceTier } : {}),
		...(reasoningSummary ? { reasoning_summary: reasoningSummary } : {}),
		...(user ? { user } : {}),
	};
}

/**
 * Build Anthropic/Bedrock provider options from form state.
 * Validation has already passed; uses transform helpers only.
 */
function buildAnthropicOptions(
	form: AnthropicFormState,
): Record<string, unknown> {
	const effort = toTrimmedString(form.effort);
	const budgetTokens = toInt(form.thinkingBudgetTokens);
	const sendReasoning = toBool(form.sendReasoning);
	const disableParallelToolUse = toBool(form.disableParallelToolUse);

	return {
		...(effort ? { effort } : {}),
		...(budgetTokens !== undefined
			? { thinking: { budget_tokens: budgetTokens } }
			: {}),
		...(sendReasoning !== undefined ? { send_reasoning: sendReasoning } : {}),
		...(disableParallelToolUse !== undefined
			? { disable_parallel_tool_use: disableParallelToolUse }
			: {}),
	};
}

/**
 * Build Google provider options from form state.
 * Validation has already passed; uses transform helpers only.
 */
function buildGoogleOptions(form: GoogleFormState): Record<string, unknown> {
	const thinkingBudget = toInt(form.thinkingBudget);
	const includeThoughts = toBool(form.includeThoughts);
	const cachedContent = toTrimmedString(form.cachedContent);
	const safetySettings = toJSON(form.safetySettingsJSON) as
		| unknown[]
		| undefined;

	return {
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
		...(safetySettings ? { safety_settings: safetySettings } : {}),
	};
}

/**
 * Build OpenAI-compatible provider options from form state.
 * Validation has already passed; uses transform helpers only.
 */
function buildOpenAICompatOptions(
	form: OpenAICompatFormState,
): Record<string, unknown> {
	const reasoningEffort = toTrimmedString(form.reasoningEffort);
	const user = toTrimmedString(form.user);

	return {
		...(reasoningEffort ? { reasoning_effort: reasoningEffort } : {}),
		...(user ? { user } : {}),
	};
}

/**
 * Shared builder for OpenRouter/Vercel provider options. Both
 * providers use an identical reasoning + parallel_tool_calls + user
 * structure. Validation has already passed.
 */
function buildReasoningProviderOptions(form: {
	reasoningEnabled: string;
	reasoningEffort: string;
	reasoningMaxTokens: string;
	reasoningExclude: string;
	parallelToolCalls: string;
	user: string;
}): Record<string, unknown> {
	const reasoningEnabled = toBool(form.reasoningEnabled);
	const reasoningEffort = toTrimmedString(form.reasoningEffort);
	const reasoningMaxTokens = toInt(form.reasoningMaxTokens);
	const reasoningExclude = toBool(form.reasoningExclude);
	const parallelToolCalls = toBool(form.parallelToolCalls);
	const user = toTrimmedString(form.user);

	const reasoning: Record<string, unknown> = {
		...(reasoningEnabled !== undefined ? { enabled: reasoningEnabled } : {}),
		...(reasoningEffort ? { effort: reasoningEffort } : {}),
		...(reasoningMaxTokens !== undefined
			? { max_tokens: reasoningMaxTokens }
			: {}),
		...(reasoningExclude !== undefined ? { exclude: reasoningExclude } : {}),
	};

	return {
		...(hasObjectKeys(reasoning) ? { reasoning } : {}),
		...(parallelToolCalls !== undefined
			? { parallel_tool_calls: parallelToolCalls }
			: {}),
		...(user ? { user } : {}),
	};
}

// ── Form → model config builder ──────────────────────────────

export const buildModelConfigFromForm = (
	provider: string | null | undefined,
	form: ModelConfigFormState,
): ModelConfigFormBuildResult => {
	const fieldErrors: FieldErrors = {};

	// Validate top-level fields.
	collectYupErrors(topLevelSchema, form, fieldErrors);

	// Validate provider-specific fields.
	const normalizedProvider = normalizeProvider(provider ?? "");

	switch (normalizedProvider) {
		case "openai":
		case "azure":
			collectYupErrors(openaiSchema, form.openai, fieldErrors, "openai");
			break;
		case "anthropic":
		case "bedrock":
			collectYupErrors(
				anthropicSchema,
				form.anthropic,
				fieldErrors,
				"anthropic",
			);
			break;
		case "google":
			collectYupErrors(googleSchema, form.google, fieldErrors, "google");
			break;
		case "openaicompat":
			collectYupErrors(
				openaiCompatSchema,
				form.openaicompat,
				fieldErrors,
				"openaicompat",
			);
			break;
		case "openrouter":
			collectYupErrors(
				reasoningProviderSchema,
				form.openrouter,
				fieldErrors,
				"openrouter",
			);
			collectYupErrors(
				openrouterExtraSchema,
				form.openrouter,
				fieldErrors,
				"openrouter",
			);
			break;
		case "vercel":
			collectYupErrors(
				reasoningProviderSchema,
				form.vercel,
				fieldErrors,
				"vercel",
			);
			break;
	}

	if (Object.keys(fieldErrors).length > 0) {
		return { fieldErrors };
	}

	// Transform top-level fields.
	const maxOutputTokens = toInt(form.maxOutputTokens);
	const temperature = toNum(form.temperature);
	const topP = toNum(form.topP);
	const topK = toInt(form.topK);
	const presencePenalty = toNum(form.presencePenalty);
	const frequencyPenalty = toNum(form.frequencyPenalty);

	// Build provider-specific options.
	let providerOptions: TypesGen.ChatModelProviderOptions | undefined;

	switch (normalizedProvider) {
		case "openai":
		case "azure": {
			const opts = buildOpenAIOptions(form.openai);
			if (hasObjectKeys(opts)) {
				providerOptions = { openai: opts };
			}
			break;
		}
		case "anthropic":
		case "bedrock": {
			const opts = buildAnthropicOptions(form.anthropic);
			if (hasObjectKeys(opts)) {
				providerOptions = { anthropic: opts };
			}
			break;
		}
		case "google": {
			const opts = buildGoogleOptions(form.google);
			if (hasObjectKeys(opts)) {
				providerOptions = { google: opts };
			}
			break;
		}
		case "openaicompat": {
			const opts = buildOpenAICompatOptions(form.openaicompat);
			if (hasObjectKeys(opts)) {
				providerOptions = { openaicompat: opts };
			}
			break;
		}
		case "openrouter": {
			const opts = buildReasoningProviderOptions(form.openrouter);
			const includeUsage = toBool(form.openrouter.includeUsage);
			if (includeUsage !== undefined) {
				opts.include_usage = includeUsage;
			}
			if (hasObjectKeys(opts)) {
				providerOptions = { openrouter: opts };
			}
			break;
		}
		case "vercel": {
			const opts = buildReasoningProviderOptions(form.vercel);
			if (hasObjectKeys(opts)) {
				providerOptions = { vercel: opts };
			}
			break;
		}
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
