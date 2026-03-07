import type {
	AIBridgeModel,
	AIModelConfig,
	AnthropicEffort,
	AnthropicThinking,
	OpenAIReasoningEffort,
} from "api/queries/aiBridge";
import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectLabel,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { SingleThumbSlider } from "components/Slider/SingleThumbSlider";
import { type FC, useState } from "react";

const DEFAULT_REASONING_EFFORT: OpenAIReasoningEffort = "medium";
const DEFAULT_THINKING_BUDGET_TOKENS = 10_240;
const MIN_THINKING_BUDGET_TOKENS = 1_024;
const MAX_THINKING_BUDGET_TOKENS = 32_768;
const THINKING_BUDGET_STEP_TOKENS = 1_024;

const OPENAI_REASONING_OPTIONS: readonly OpenAIReasoningEffort[] = [
	"low",
	"medium",
	"high",
	"xhigh",
];

const ANTHROPIC_EFFORT_OPTIONS: readonly AnthropicEffort[] = [
	"low",
	"medium",
	"high",
	"max",
];

const CURATED_MODEL_PATTERNS: readonly string[] = [
	// Anthropic — flagship models.
	"claude-opus-4-6",
	"claude-sonnet-4-6",
	"claude-haiku-4-5",
];

type ThinkingModeValue = "disabled" | "adaptive" | "budget";

const toModelKey = (model: AIBridgeModel): string =>
	`${model.provider}:${model.id}`;

const parseModelKey = (
	key: string,
): { provider: AIBridgeModel["provider"]; id: string } => {
	const separatorIndex = key.indexOf(":");
	if (separatorIndex === -1) {
		throw new Error(
			`Invalid model key "${key}". Expected provider:model format.`,
		);
	}

	const provider = key.slice(0, separatorIndex);
	const id = key.slice(separatorIndex + 1);
	if ((provider !== "openai" && provider !== "anthropic") || id.length === 0) {
		throw new Error(
			`Invalid model key "${key}". Unknown provider or empty ID.`,
		);
	}

	return { provider, id };
};

const clampThinkingBudgetTokens = (value: number): number => {
	if (!Number.isFinite(value)) {
		throw new Error("Thinking budget must be a finite number.");
	}

	const roundedToStep =
		Math.round(value / THINKING_BUDGET_STEP_TOKENS) *
		THINKING_BUDGET_STEP_TOKENS;
	return Math.min(
		MAX_THINKING_BUDGET_TOKENS,
		Math.max(MIN_THINKING_BUDGET_TOKENS, roundedToStep),
	);
};

const getThinkingMode = (
	thinking: AnthropicThinking | undefined,
): ThinkingModeValue => {
	if (!thinking || thinking.type === "disabled") {
		return "disabled";
	}
	if (thinking.type === "adaptive") {
		return "adaptive";
	}
	return "budget";
};

export const getDefaultModelConfig = (model: AIBridgeModel): AIModelConfig => {
	const config: AIModelConfig = { model };

	if (model.provider === "openai" && isOpenAIReasoningModel(model.id)) {
		config.reasoningEffort = DEFAULT_REASONING_EFFORT;
	}
	if (model.provider === "anthropic" && isAnthropicEffortModel(model.id)) {
		config.thinking = { type: "adaptive" };
		config.anthropicEffort = "medium";
	} else if (
		model.provider === "anthropic" &&
		isAnthropicBudgetThinkingModel(model.id)
	) {
		config.thinking = { type: "adaptive" };
	}

	return config;
};

/** Returns true for OpenAI models that support reasoning effort. */
const isOpenAIReasoningModel = (modelID: string): boolean => {
	const normalized = modelID.toLowerCase();
	return (
		normalized.startsWith("gpt-5") ||
		normalized.startsWith("o1") ||
		normalized.startsWith("o3") ||
		normalized.startsWith("o4")
	);
};

/** Returns true for Anthropic 4.6 models that support effort+adaptive. */
const isAnthropicEffortModel = (modelID: string): boolean => {
	const normalized = modelID.toLowerCase();
	return (
		normalized.includes("claude-sonnet-4-6") ||
		normalized.includes("claude-opus-4-6")
	);
};

/**
 * Returns true for Anthropic models that support manual thinking budget
 * controls via budget tokens.
 */
const isAnthropicBudgetThinkingModel = (modelID: string): boolean => {
	const normalized = modelID.toLowerCase();
	return (
		(normalized.includes("claude-3-7-sonnet") ||
			normalized.includes("claude-sonnet-4") ||
			normalized.includes("claude-opus-4")) &&
		!isAnthropicEffortModel(normalized)
	);
};

/** Returns true for Anthropic models that support extended thinking. */
const isAnthropicThinkingModel = (modelID: string): boolean => {
	return (
		isAnthropicEffortModel(modelID) || isAnthropicBudgetThinkingModel(modelID)
	);
};

/**
 * Returns true for GPT-5 models at version 5.2 or higher. Uses regex
 * parsing rather than prefix matching so future minor versions (5.3,
 * 5.4, …) are automatically included.
 */
const isGpt52OrHigher = (modelId: string): boolean => {
	const match = modelId.toLowerCase().match(/^gpt-5\.(\d+)/);
	if (!match) return false;
	return Number.parseInt(match[1], 10) >= 2;
};

export const isCuratedModel = (modelId: string): boolean => {
	if (isGpt52OrHigher(modelId)) return true;
	const normalized = modelId.toLowerCase();
	return CURATED_MODEL_PATTERNS.some((pattern) =>
		normalized.startsWith(pattern),
	);
};

interface ModelConfigBarProps {
	modelConfig: AIModelConfig;
	availableModels: readonly AIBridgeModel[];
	onModelConfigChange: (config: AIModelConfig) => void;
}

export const ModelConfigBar: FC<ModelConfigBarProps> = ({
	modelConfig,
	availableModels,
	onModelConfigChange,
}) => {
	const selectedModel = modelConfig.model;
	const [showAllModels, setShowAllModels] = useState(false);
	const selectedModelKey = toModelKey(selectedModel);
	const displayModels = [...availableModels]
		.filter(
			(model) =>
				showAllModels ||
				isCuratedModel(model.id) ||
				toModelKey(model) === selectedModelKey,
		)
		.sort((a, b) => a.id.localeCompare(b.id));
	const openAIModels = displayModels.filter(
		(model) => model.provider === "openai",
	);
	const anthropicModels = displayModels.filter(
		(model) => model.provider === "anthropic",
	);
	const showOpenAIReasoning =
		selectedModel.provider === "openai" &&
		isOpenAIReasoningModel(selectedModel.id);
	const showAnthropicThinking =
		selectedModel.provider === "anthropic" &&
		isAnthropicThinkingModel(selectedModel.id);
	const showAnthropicEffort =
		selectedModel.provider === "anthropic" &&
		isAnthropicEffortModel(selectedModel.id);
	const showAnthropicBudgetThinking =
		selectedModel.provider === "anthropic" &&
		isAnthropicBudgetThinkingModel(selectedModel.id);

	const reasoningEffort =
		modelConfig.reasoningEffort ?? DEFAULT_REASONING_EFFORT;
	const thinkingMode = getThinkingMode(modelConfig.thinking);
	const selectedThinkingMode: ThinkingModeValue =
		showAnthropicEffort && thinkingMode === "budget"
			? "adaptive"
			: thinkingMode;
	const thinkingBudgetTokens =
		modelConfig.thinking?.type === "enabled"
			? clampThinkingBudgetTokens(modelConfig.thinking.budgetTokens)
			: DEFAULT_THINKING_BUDGET_TOKENS;
	const anthropicEffort = modelConfig.anthropicEffort ?? "medium";

	const handleModelChange = (nextModelKey: string) => {
		const parsedModel = parseModelKey(nextModelKey);
		const nextModel = availableModels.find(
			(model) =>
				model.provider === parsedModel.provider && model.id === parsedModel.id,
		);
		if (!nextModel) {
			throw new Error(`Selected model "${nextModelKey}" is not available.`);
		}
		onModelConfigChange(getDefaultModelConfig(nextModel));
	};

	const handleReasoningEffortChange = (nextReasoningEffort: string) => {
		if (
			!OPENAI_REASONING_OPTIONS.includes(
				nextReasoningEffort as OpenAIReasoningEffort,
			)
		) {
			throw new Error(
				`Invalid OpenAI reasoning effort "${nextReasoningEffort}" selected.`,
			);
		}
		onModelConfigChange({
			...modelConfig,
			reasoningEffort: nextReasoningEffort as OpenAIReasoningEffort,
		});
	};

	const handleThinkingModeChange = (nextThinkingMode: string) => {
		switch (nextThinkingMode) {
			case "disabled":
				onModelConfigChange({
					...modelConfig,
					thinking: { type: "disabled" },
				});
				return;
			case "adaptive":
				onModelConfigChange({
					...modelConfig,
					thinking: { type: "adaptive" },
				});
				return;
			case "budget":
				if (!showAnthropicBudgetThinking) {
					throw new Error(
						"Thinking budget is only supported for Anthropic budget-thinking models.",
					);
				}
				onModelConfigChange({
					...modelConfig,
					thinking: {
						type: "enabled",
						budgetTokens: thinkingBudgetTokens,
					},
				});
				return;
			default:
				throw new Error(
					`Unknown Anthropic thinking mode "${nextThinkingMode}".`,
				);
		}
	};

	const handleEffortChange = (nextEffort: string) => {
		if (!ANTHROPIC_EFFORT_OPTIONS.includes(nextEffort as AnthropicEffort)) {
			throw new Error(`Unknown Anthropic effort "${nextEffort}".`);
		}
		onModelConfigChange({
			...modelConfig,
			anthropicEffort: nextEffort as AnthropicEffort,
		});
	};

	return (
		<div className="px-3 py-1.5">
			<div className="flex flex-wrap items-end gap-2">
				<div className="min-w-[180px] flex-1">
					<div className="mb-0.5 text-2xs text-content-secondary">Model</div>
					<Select
						value={selectedModelKey}
						onValueChange={handleModelChange}
						onOpenChange={(open) => {
							// Reset to curated list when the dropdown closes so
							// the next open always starts compact.
							if (!open) {
								setShowAllModels(false);
							}
						}}
					>
						<SelectTrigger className="h-8 text-xs">
							<SelectValue placeholder="Select a model" />
						</SelectTrigger>
						<SelectContent>
							{openAIModels.length > 0 && (
								<SelectGroup>
									<SelectLabel>OpenAI</SelectLabel>
									{openAIModels.map((model) => (
										<SelectItem
											key={toModelKey(model)}
											value={toModelKey(model)}
										>
											{model.id}
										</SelectItem>
									))}
								</SelectGroup>
							)}
							{anthropicModels.length > 0 && (
								<SelectGroup>
									<SelectLabel>Anthropic</SelectLabel>
									{anthropicModels.map((model) => (
										<SelectItem
											key={toModelKey(model)}
											value={toModelKey(model)}
										>
											{model.id}
										</SelectItem>
									))}
								</SelectGroup>
							)}
							{/* Toggle at the bottom of the list. Uses
							    onPointerDown preventDefault to stop Radix from
							    closing the popover on click. */}
							<button
								type="button"
								className="w-full cursor-pointer border-none bg-transparent px-2 py-1.5 text-left text-xs text-content-link hover:underline"
								onPointerDown={(e) => e.preventDefault()}
								onClick={() => setShowAllModels((prev) => !prev)}
							>
								{showAllModels ? "Show fewer models" : "Show all models"}
							</button>
						</SelectContent>
					</Select>
				</div>

				{showOpenAIReasoning && (
					<div className="w-[120px]">
						<div className="mb-0.5 text-2xs text-content-secondary">
							Reasoning
						</div>
						<Select
							value={reasoningEffort}
							onValueChange={handleReasoningEffortChange}
						>
							<SelectTrigger className="h-8 text-xs">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								{OPENAI_REASONING_OPTIONS.map((option) => (
									<SelectItem key={option} value={option}>
										{option}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>
				)}

				{showAnthropicThinking && (
					<>
						<div className="w-[130px]">
							<div className="mb-0.5 text-2xs text-content-secondary">
								Thinking
							</div>
							<Select
								value={selectedThinkingMode}
								onValueChange={handleThinkingModeChange}
							>
								<SelectTrigger className="h-8 text-xs">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="disabled">Disabled</SelectItem>
									<SelectItem value="adaptive">Adaptive</SelectItem>
									{showAnthropicBudgetThinking && (
										<SelectItem value="budget">Budget</SelectItem>
									)}
								</SelectContent>
							</Select>
						</div>
						{showAnthropicBudgetThinking &&
							selectedThinkingMode === "budget" && (
								<div className="min-w-[180px] flex-1">
									<div className="mb-0.5 flex items-center justify-between text-2xs text-content-secondary">
										<span>Thinking budget</span>
										<span>{thinkingBudgetTokens.toLocaleString()} tokens</span>
									</div>
									<SingleThumbSlider
										value={[thinkingBudgetTokens]}
										min={MIN_THINKING_BUDGET_TOKENS}
										max={MAX_THINKING_BUDGET_TOKENS}
										step={THINKING_BUDGET_STEP_TOKENS}
										onValueChange={(value) => {
											const nextBudgetTokens = value[0];
											if (typeof nextBudgetTokens !== "number") {
												throw new Error(
													"Expected a single slider value for thinking budget.",
												);
											}
											onModelConfigChange({
												...modelConfig,
												thinking: {
													type: "enabled",
													budgetTokens:
														clampThinkingBudgetTokens(nextBudgetTokens),
												},
											});
										}}
									/>
								</div>
							)}
					</>
				)}
				{showAnthropicEffort && (
					<div className="w-[100px]">
						<div className="mb-0.5 text-2xs text-content-secondary">Effort</div>
						<Select value={anthropicEffort} onValueChange={handleEffortChange}>
							<SelectTrigger className="h-8 text-xs">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								{ANTHROPIC_EFFORT_OPTIONS.map((option) => (
									<SelectItem key={option} value={option}>
										{option}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
					</div>
				)}
			</div>
		</div>
	);
};
