import type {
	ChatModelConfig,
	CreateChatModelConfigRequest,
	UpdateChatModelConfigRequest,
} from "api/api";
import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Input } from "components/Input/Input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { ArrowLeftIcon, Loader2Icon, PlusIcon, SaveIcon } from "lucide-react";
import {
	type FC,
	type FormEvent,
	useEffect,
	useId,
	useMemo,
	useState,
} from "react";
import { cn } from "utils/cn";
import type { ProviderState } from "./ChatModelAdminPanel";
import {
	ModelConfigFields,
	modelConfigAnthropicEffortOptions,
	modelConfigReasoningEffortOptions,
	modelConfigTextVerbosityOptions,
} from "./ModelConfigFields";
import { ProviderIcon } from "./ProviderIcon";

// ── Form state types (owned by this component) ────────────────

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

const emptyModelConfigFormState: ModelConfigFormState = {
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

// ── Schema reference data ──────────────────────────────────────

type ProviderModelConfigSchemaReference = {
	modelConfig: TypesGen.ChatModelCallConfig;
	notes?: readonly string[];
};

const modelConfigSchemaByProvider: Record<
	string,
	ProviderModelConfigSchemaReference
> = {
	openai: {
		modelConfig: {
			max_output_tokens: 32000,
			temperature: 0.2,
			top_p: 0.95,
			top_k: 40,
			presence_penalty: 0,
			frequency_penalty: 0,
			provider_options: {
				openai: {
					reasoning_effort: "high",
					parallel_tool_calls: true,
					text_verbosity: "low",
					service_tier: "auto",
					user: "end-user-id",
				},
			},
		},
		notes: [
			"Responses API models may also use reasoning_summary and include.",
		],
	},
	azure: {
		modelConfig: {
			max_output_tokens: 32000,
			provider_options: {
				openai: {
					reasoning_effort: "high",
					parallel_tool_calls: true,
					user: "end-user-id",
				},
			},
		},
		notes: ["Azure uses OpenAI provider option keys in Fantasy."],
	},
	anthropic: {
		modelConfig: {
			max_output_tokens: 32000,
			provider_options: {
				anthropic: {
					effort: "medium",
					thinking: { budget_tokens: 4000 },
					send_reasoning: true,
					disable_parallel_tool_use: false,
				},
			},
		},
	},
	bedrock: {
		modelConfig: {
			max_output_tokens: 32000,
			provider_options: {
				anthropic: {
					effort: "medium",
					thinking: { budget_tokens: 4000 },
					send_reasoning: true,
					disable_parallel_tool_use: false,
				},
			},
		},
		notes: ["Bedrock uses Anthropic option keys in Fantasy."],
	},
	google: {
		modelConfig: {
			max_output_tokens: 32000,
			provider_options: {
				google: {
					thinking_config: {
						thinking_budget: 1024,
						include_thoughts: true,
					},
					safety_settings: [
						{
							category: "HARM_CATEGORY_DANGEROUS_CONTENT",
							threshold: "BLOCK_ONLY_HIGH",
						},
					],
				},
			},
		},
	},
	openaicompat: {
		modelConfig: {
			max_output_tokens: 32000,
			provider_options: {
				openaicompat: {
					reasoning_effort: "medium",
					user: "end-user-id",
				},
			},
		},
	},
	openrouter: {
		modelConfig: {
			max_output_tokens: 32000,
			provider_options: {
				openrouter: {
					reasoning: {
						enabled: true,
						effort: "medium",
						max_tokens: 2048,
						exclude: false,
					},
					parallel_tool_calls: true,
					include_usage: true,
					user: "end-user-id",
				},
			},
		},
	},
	vercel: {
		modelConfig: {
			max_output_tokens: 32000,
			provider_options: {
				vercel: {
					reasoning: {
						enabled: true,
						effort: "medium",
						max_tokens: 2048,
						exclude: false,
					},
					parallel_tool_calls: true,
					user: "end-user-id",
				},
			},
		},
	},
};

// ── Helpers ────────────────────────────────────────────────────

const hasObjectKeys = (value: Record<string, unknown>): boolean =>
	Object.keys(value).length > 0;

const parsePositiveInteger = (value: string): number | null => {
	const trimmed = value.trim();
	if (!trimmed) return null;
	const parsed = Number.parseInt(trimmed, 10);
	if (!Number.isFinite(parsed) || parsed <= 0) return null;
	return parsed;
};

const parseThresholdInteger = (value: string): number | null => {
	const trimmed = value.trim();
	if (!trimmed) return null;
	const parsed = Number.parseInt(trimmed, 10);
	if (!Number.isFinite(parsed) || parsed < 0 || parsed > 100) return null;
	return parsed;
};

const getModelConfigSchemaReference = (
	providerState: ProviderState | null,
) => {
	const providerLabel = providerState?.label ?? "Provider";
	const normalizedProvider = (providerState?.provider ?? "")
		.trim()
		.toLowerCase();
	const providerConfigSchema =
		modelConfigSchemaByProvider[normalizedProvider];
	const modelConfigTemplate =
		providerConfigSchema?.modelConfig ?? {};
	const notes = providerConfigSchema?.notes ?? [
		"No provider-specific options are documented for this provider yet.",
	];

	const schema: TypesGen.CreateChatModelConfigRequest = {
		provider: normalizedProvider || "<provider>",
		model: "<model-id>",
		context_limit: 200000,
		compression_threshold: 70,
		model_config: modelConfigTemplate,
	};

	return {
		providerLabel,
		notes,
		schemaJSON: JSON.stringify(schema, null, 2),
	};
};

// ── Extract model config form state from an existing model ────

const extractModelConfigFormState = (
	model: ChatModelConfig,
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
	const vercelReasoning = (vercel.reasoning ?? {}) as Record<
		string,
		unknown
	>;

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
		anthropicDisableParallelToolUse: str(
			anthropic.disable_parallel_tool_use,
		),

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

const buildModelConfigFromForm = (
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
	const normalizedProvider = (provider ?? "").trim().toLowerCase();

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
				...(reasoningSummary
					? { reasoning_summary: reasoningSummary }
					: {}),
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
			if (
				hasObjectKeys(anthropicOptions as Record<string, unknown>)
			) {
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
				...(thinkingBudget !== undefined ||
				includeThoughts !== undefined
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
				...(cachedContent
					? { cached_content: cachedContent }
					: {}),
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
			const reasoningEnabled = parseOptionalBoolean(
				"openrouterReasoningEnabled",
				"Reasoning enabled",
				form.openrouterReasoningEnabled,
			);
			const reasoningEffort = parseOptionalSelect(
				"openrouterReasoningEffort",
				"Reasoning effort",
				form.openrouterReasoningEffort,
				modelConfigReasoningEffortOptions,
			);
			const reasoningMaxTokens = parseOptionalInteger(
				"openrouterReasoningMaxTokens",
				"Reasoning max tokens",
				form.openrouterReasoningMaxTokens,
			);
			const reasoningExclude = parseOptionalBoolean(
				"openrouterReasoningExclude",
				"Reasoning exclude",
				form.openrouterReasoningExclude,
			);
			const parallelToolCalls = parseOptionalBoolean(
				"openrouterParallelToolCalls",
				"Parallel tool calls",
				form.openrouterParallelToolCalls,
			);
			const includeUsage = parseOptionalBoolean(
				"openrouterIncludeUsage",
				"Include usage",
				form.openrouterIncludeUsage,
			);
			const user = form.openrouterUser.trim();
			const reasoning: Record<string, unknown> = {
				...(reasoningEnabled !== undefined
					? { enabled: reasoningEnabled }
					: {}),
				...(reasoningEffort ? { effort: reasoningEffort } : {}),
				...(reasoningMaxTokens !== undefined
					? { max_tokens: reasoningMaxTokens }
					: {}),
				...(reasoningExclude !== undefined
					? { exclude: reasoningExclude }
					: {}),
			};
			const opts: Record<string, unknown> = {
				...(hasObjectKeys(reasoning as Record<string, unknown>)
					? { reasoning }
					: {}),
				...(parallelToolCalls !== undefined
					? { parallel_tool_calls: parallelToolCalls }
					: {}),
				...(includeUsage !== undefined
					? { include_usage: includeUsage }
					: {}),
				...(user ? { user } : {}),
			};
			if (hasObjectKeys(opts as Record<string, unknown>)) {
				providerOptions = { openrouter: opts };
			}
			break;
		}
		case "vercel": {
			const reasoningEnabled = parseOptionalBoolean(
				"vercelReasoningEnabled",
				"Reasoning enabled",
				form.vercelReasoningEnabled,
			);
			const reasoningEffort = parseOptionalSelect(
				"vercelReasoningEffort",
				"Reasoning effort",
				form.vercelReasoningEffort,
				modelConfigReasoningEffortOptions,
			);
			const reasoningMaxTokens = parseOptionalInteger(
				"vercelReasoningMaxTokens",
				"Reasoning max tokens",
				form.vercelReasoningMaxTokens,
			);
			const reasoningExclude = parseOptionalBoolean(
				"vercelReasoningExclude",
				"Reasoning exclude",
				form.vercelReasoningExclude,
			);
			const parallelToolCalls = parseOptionalBoolean(
				"vercelParallelToolCalls",
				"Parallel tool calls",
				form.vercelParallelToolCalls,
			);
			const user = form.vercelUser.trim();
			const reasoning: Record<string, unknown> = {
				...(reasoningEnabled !== undefined
					? { enabled: reasoningEnabled }
					: {}),
				...(reasoningEffort ? { effort: reasoningEffort } : {}),
				...(reasoningMaxTokens !== undefined
					? { max_tokens: reasoningMaxTokens }
					: {}),
				...(reasoningExclude !== undefined
					? { exclude: reasoningExclude }
					: {}),
			};
			const opts: Record<string, unknown> = {
				...(hasObjectKeys(reasoning as Record<string, unknown>)
					? { reasoning }
					: {}),
				...(parallelToolCalls !== undefined
					? { parallel_tool_calls: parallelToolCalls }
					: {}),
				...(user ? { user } : {}),
			};
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

// ── Component ──────────────────────────────────────────────────

type ModelFormProps = {
	/** When set, the form is in "edit" mode for the given model. */
	editingModel?: ChatModelConfig;
	providerStates: readonly ProviderState[];
	selectedProvider: string | null;
	selectedProviderState: ProviderState | null;
	onSelectedProviderChange: (provider: string) => void;
	modelConfigsUnavailable: boolean;
	isSaving: boolean;
	onCreateModel: (req: CreateChatModelConfigRequest) => Promise<unknown>;
	onUpdateModel: (
		modelConfigId: string,
		req: UpdateChatModelConfigRequest,
	) => Promise<unknown>;
	onCancel: () => void;
};

export const ModelForm: FC<ModelFormProps> = ({
	editingModel,
	providerStates,
	selectedProvider,
	selectedProviderState,
	onSelectedProviderChange,
	modelConfigsUnavailable,
	isSaving,
	onCreateModel,
	onUpdateModel,
	onCancel,
}) => {
	const isEditing = Boolean(editingModel);

	const providerSelectInputId = useId();
	const modelInputId = useId();
	const displayNameInputId = useId();
	const contextLimitInputId = useId();
	const compressionThresholdInputId = useId();
	const modelConfigInputId = useId();

	const [model, setModel] = useState(editingModel?.model ?? "");
	const [displayName, setDisplayName] = useState(
		editingModel?.display_name ?? "",
	);
	const [contextLimit, setContextLimit] = useState(
		editingModel ? String(editingModel.context_limit) : "",
	);
	const [compressionThreshold, setCompressionThreshold] = useState(
		editingModel ? String(editingModel.compression_threshold) : "70",
	);
	const [modelConfigForm, setModelConfigForm] =
		useState<ModelConfigFormState>(() =>
			editingModel
				? extractModelConfigFormState(editingModel)
				: { ...emptyModelConfigFormState },
		);

	// Reset form fields when the selected provider changes (add
	// mode only — in edit mode the provider is fixed).
	useEffect(() => {
		if (!isEditing) {
			setModelConfigForm({ ...emptyModelConfigFormState });
		}
	}, [isEditing, selectedProviderState?.provider]);

	const canManageModels = Boolean(
		selectedProviderState?.providerConfig &&
			selectedProviderState.hasEffectiveAPIKey,
	);

	const modelConfigFormBuildResult = useMemo(
		() =>
			buildModelConfigFromForm(
				selectedProviderState?.provider,
				modelConfigForm,
			),
		[selectedProviderState?.provider, modelConfigForm],
	);

	const hasFieldErrors =
		Object.keys(modelConfigFormBuildResult.fieldErrors).length > 0;

	const modelConfigSchemaReference = useMemo(
		() => getModelConfigSchemaReference(selectedProviderState),
		[selectedProviderState],
	);

	// ── Provider select (shared across all form states) ───────

	const providerSelect = (
		<div className="grid gap-1.5">
			<label
				htmlFor={providerSelectInputId}
				className="text-[13px] font-medium text-content-primary"
			>
				Provider
			</label>
			<Select
				value={selectedProvider ?? ""}
				onValueChange={onSelectedProviderChange}
				disabled={isEditing || providerStates.length === 0}
			>
				<SelectTrigger
					id={providerSelectInputId}
					className="h-10 max-w-[240px] text-[13px]"
				>
					<SelectValue placeholder="Select provider" />
				</SelectTrigger>
				<SelectContent>
					{providerStates.map((ps) => (
						<SelectItem key={ps.provider} value={ps.provider}>
							<span className="flex items-center gap-2">
								<ProviderIcon
									provider={ps.provider}
									className="h-4 w-4"
								/>
								{ps.label}
							</span>
						</SelectItem>
					))}
				</SelectContent>
			</Select>
		</div>
	);

	// No provider selected or configs unavailable.
	if (!selectedProviderState || modelConfigsUnavailable) {
		return (
			<div className="flex h-full flex-col">
				<div className="flex items-center gap-2 border-b border-border px-6 py-4">
					<Button
						variant="subtle"
						size="icon"
						className="h-8 w-8 shrink-0"
						onClick={onCancel}
					>
						<ArrowLeftIcon className="h-4 w-4" />
						<span className="sr-only">Back</span>
					</Button>
					<h3 className="m-0 text-base font-semibold text-content-primary">
						{isEditing ? "Edit model" : "Add model"}
					</h3>
				</div>
				<div className="space-y-3 p-6">{providerSelect}</div>
			</div>
		);
	}

	// Provider can't manage models.
	if (!canManageModels && !isEditing) {
		return (
			<div className="flex h-full flex-col">
				<div className="flex items-center gap-2 border-b border-border px-6 py-4">
					<Button
						variant="subtle"
						size="icon"
						className="h-8 w-8 shrink-0"
						onClick={onCancel}
					>
						<ArrowLeftIcon className="h-4 w-4" />
						<span className="sr-only">Back</span>
					</Button>
					<h3 className="m-0 text-base font-semibold text-content-primary">
						Add model
					</h3>
				</div>
				<div className="space-y-3 p-6">
					{providerSelect}
					<p className="text-[13px] text-content-secondary">
						{!selectedProviderState.providerConfig
							? "Create a managed provider config on the Providers tab before adding models."
							: "Set an API key for this provider on the Providers tab before adding models."}
					</p>
				</div>
			</div>
		);
	}

	// ── Full form ─────────────────────────────────────────────

	const parsedContextLimit = parsePositiveInteger(contextLimit);
	const parsedCompressionThreshold =
		parseThresholdInteger(compressionThreshold);
	const contextLimitError =
		contextLimit.trim() && parsedContextLimit === null
			? "Context limit must be a positive integer."
			: undefined;
	const compressionThresholdError =
		compressionThreshold.trim() && parsedCompressionThreshold === null
			? "Compression threshold must be a number between 0 and 100."
			: undefined;

	const handleSubmit = async (event: FormEvent) => {
		event.preventDefault();
		if (isSaving) return;

		const trimmedModel = model.trim();
		if (!trimmedModel) return;
		if (parsedContextLimit === null) return;
		if (parsedCompressionThreshold === null) return;
		if (hasFieldErrors) return;

		const trimmedDisplayName = displayName.trim();
		const builtModelConfig = modelConfigFormBuildResult.modelConfig;

		if (isEditing && editingModel) {
			const req: UpdateChatModelConfigRequest = {};
			if (trimmedModel !== editingModel.model) {
				req.model = trimmedModel;
			}
			if (trimmedDisplayName !== (editingModel.display_name ?? "")) {
				req.display_name = trimmedDisplayName;
			}
			if (parsedContextLimit !== editingModel.context_limit) {
				req.context_limit = parsedContextLimit;
			}
			if (
				parsedCompressionThreshold !==
				editingModel.compression_threshold
			) {
				req.compression_threshold = parsedCompressionThreshold;
			}
			// Always send model_config so it can be cleared or updated.
			req.model_config = builtModelConfig;

			await onUpdateModel(editingModel.id, req);
			onCancel();
		} else {
			if (!selectedProviderState?.providerConfig) return;

			const req: CreateChatModelConfigRequest = {
				provider: selectedProviderState.provider,
				model: trimmedModel,
				context_limit: parsedContextLimit,
				compression_threshold: parsedCompressionThreshold,
			};
			if (trimmedDisplayName) {
				req.display_name = trimmedDisplayName;
			}
			if (builtModelConfig) {
				req.model_config = builtModelConfig;
			}

			await onCreateModel(req);
			setModel("");
			setDisplayName("");
			setContextLimit("");
			setCompressionThreshold("70");
			setModelConfigForm({ ...emptyModelConfigFormState });
			onCancel();
		}
	};

	return (
		<div className="flex h-full flex-col">
			{/* Header bar with back button */}
			<div className="flex items-center justify-between gap-3 border-b border-border px-6 py-4">
				<div className="flex items-center gap-2">
					<Button
						variant="subtle"
						size="icon"
						className="h-8 w-8 shrink-0"
						onClick={onCancel}
					>
						<ArrowLeftIcon className="h-4 w-4" />
						<span className="sr-only">Back</span>
					</Button>
					<h3 className="m-0 text-base font-semibold text-content-primary">
						{isEditing ? "Edit model" : "Add model"}
					</h3>
					{selectedProviderState && (
						<span className="inline-flex items-center gap-1.5 rounded-md border border-border bg-surface-secondary/40 px-2 py-0.5 text-xs text-content-secondary">
							<ProviderIcon
								provider={selectedProviderState.provider}
								className="h-3.5 w-3.5"
								active
							/>
							{selectedProviderState.label}
						</span>
					)}
				</div>
			</div>

			{/* Form body */}
			<form
				className="flex min-h-0 flex-1 flex-col"
				onSubmit={(event) => void handleSubmit(event)}
			>
				<div className="flex-1 space-y-5 overflow-y-auto p-6">
					{/* Model identity */}
					<div className="space-y-3">
						<div>
							<p className="m-0 text-[13px] font-medium text-content-primary">
								Model identity
							</p>
							<p className="m-0 text-xs text-content-secondary">
								Select provider and model naming details.
							</p>
						</div>
						<div className="grid gap-3 md:grid-cols-3">
							{providerSelect}
							<div className="grid gap-1.5">
								<label
									htmlFor={modelInputId}
									className="text-[13px] font-medium text-content-primary"
								>
									Model ID
								</label>
								<Input
									id={modelInputId}
									className="h-10 text-[13px]"
									placeholder="gpt-5, claude-sonnet-4-5, etc."
									value={model}
									onChange={(e) =>
										setModel(e.target.value)
									}
									disabled={isSaving}
								/>
							</div>
							<div className="grid gap-1.5">
								<label
									htmlFor={displayNameInputId}
									className="text-[13px] font-medium text-content-primary"
								>
									Display name{" "}
									<span className="font-normal text-content-secondary">
										(optional)
									</span>
								</label>
								<Input
									id={displayNameInputId}
									className="h-10 text-[13px]"
									placeholder="Friendly label"
									value={displayName}
									onChange={(e) =>
										setDisplayName(e.target.value)
									}
									disabled={isSaving}
								/>
							</div>
						</div>
					</div>

					{/* Runtime limits */}
					<div className="space-y-3">
						<div>
							<p className="m-0 text-[13px] font-medium text-content-primary">
								Runtime limits
							</p>
							<p className="m-0 text-xs text-content-secondary">
								Leave values blank to use backend defaults.
							</p>
						</div>
						<div className="grid gap-3 md:grid-cols-2">
							<div className="grid gap-1.5">
								<label
									htmlFor={contextLimitInputId}
									className="text-[13px] font-medium text-content-primary"
								>
									Context limit
								</label>
								<Input
									id={contextLimitInputId}
									className={cn(
										"h-10 text-[13px]",
										contextLimitError &&
											"border-content-destructive",
									)}
									placeholder="200000"
									value={contextLimit}
									onChange={(e) =>
										setContextLimit(e.target.value)
									}
									disabled={isSaving}
								/>
								{contextLimitError && (
									<p className="m-0 text-xs text-content-destructive">
										{contextLimitError}
									</p>
								)}
							</div>
							<div className="grid gap-1.5">
								<label
									htmlFor={compressionThresholdInputId}
									className="text-[13px] font-medium text-content-primary"
								>
									Compression threshold
								</label>
								<Input
									id={compressionThresholdInputId}
									className={cn(
										"h-10 text-[13px]",
										compressionThresholdError &&
											"border-content-destructive",
									)}
									placeholder="70"
									value={compressionThreshold}
									onChange={(e) =>
										setCompressionThreshold(
											e.target.value,
										)
									}
									disabled={isSaving}
								/>
								{compressionThresholdError && (
									<p className="m-0 text-xs text-content-destructive">
										{compressionThresholdError}
									</p>
								)}
							</div>
						</div>
					</div>

					{/* Model call config fields */}
					<ModelConfigFields
						provider={selectedProviderState.provider}
						form={modelConfigForm}
						fieldErrors={modelConfigFormBuildResult.fieldErrors}
						onChange={(key, value) =>
							setModelConfigForm((prev) => ({
								...prev,
								[key]: value,
							}))
						}
						disabled={isSaving}
						inputIdPrefix={modelConfigInputId}
					/>

					{/* Schema reference */}
					<details className="group rounded-xl border border-border-default/80 bg-surface-secondary/20 shadow-sm">
						<summary className="cursor-pointer select-none px-4 py-3 text-[13px] font-medium text-content-secondary hover:text-content-primary">
							Model config schema reference (
							{modelConfigSchemaReference.providerLabel})
						</summary>
						<div className="space-y-2 border-t border-border/60 px-4 pb-4 pt-3">
							<p className="m-0 text-xs text-content-secondary">
								Reference JSON for{" "}
								<code>create/update chat model config</code>{" "}
								payloads.
							</p>
							{modelConfigSchemaReference.notes.map((note) => (
								<p
									key={note}
									className="m-0 text-xs text-content-secondary"
								>
									{note}
								</p>
							))}
							<pre
								data-testid="chat-model-config-schema"
								className="max-h-60 overflow-auto rounded-md border border-border-default/80 bg-surface-primary/80 p-2 font-mono text-[11px] leading-relaxed text-content-secondary"
							>
								{modelConfigSchemaReference.schemaJSON}
							</pre>
						</div>
					</details>
				</div>

				{/* Sticky footer actions */}
				<div className="flex items-center justify-end gap-2 border-t border-border bg-surface-primary px-6 py-4">
					<Button
						size="sm"
						variant="outline"
						type="button"
						onClick={onCancel}
					>
						Cancel
					</Button>
					<Button
						size="sm"
						type="submit"
						disabled={
							isSaving ||
							!model.trim() ||
							parsedContextLimit === null ||
							parsedCompressionThreshold === null ||
							hasFieldErrors
						}
					>
						{isSaving ? (
							<Loader2Icon className="h-4 w-4 animate-spin" />
						) : isEditing ? (
							<SaveIcon className="h-4 w-4" />
						) : (
							<PlusIcon className="h-4 w-4" />
						)}
						{isEditing ? "Save changes" : "Add model"}
					</Button>
				</div>
			</form>
		</div>
	);
};
