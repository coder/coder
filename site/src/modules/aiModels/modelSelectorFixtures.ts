import type { ModelSelectorOption } from "./ModelSelector";

export const MockModelSelectorOption: ModelSelectorOption = {
	id: "gpt-4o",
	provider: "openai",
	model: "gpt-4o",
	displayName: "gpt-4o",
};

export const MockBedrockInvokeModelSelectorOption: ModelSelectorOption = {
	...MockModelSelectorOption,
	id: "model-config-bedrock",
	provider: "bedrock",
	providerLabel: "AWS Bedrock",
	model: "claude-opus-4-5",
	displayName: "Claude Opus 4.5",
	bedrockInvokeModel: true,
};
