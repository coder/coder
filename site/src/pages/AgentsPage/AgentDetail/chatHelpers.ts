import type * as TypesGen from "api/typesGenerated";
import type { ModelSelectorOption } from "components/ai-elements";
import { asNumber, asString } from "components/ai-elements/runtimeTypeUtils";
import type { AgentContextUsage } from "../AgentChatInput";

const asTokenCount = (value: unknown): number | undefined => {
	const parsed = asNumber(value);
	if (parsed === undefined || parsed < 0) {
		return undefined;
	}
	return parsed;
};

const asNonEmptyString = (value: unknown): string | undefined => {
	const next = asString(value).trim();
	return next.length > 0 ? next : undefined;
};

type ChatMessageWithUsage = TypesGen.ChatMessage & {
	readonly input_tokens?: unknown;
	readonly output_tokens?: unknown;
	readonly total_tokens?: unknown;
	readonly reasoning_tokens?: unknown;
	readonly cache_creation_tokens?: unknown;
	readonly cache_read_tokens?: unknown;
	readonly context_limit?: unknown;
};

export const extractContextUsageFromMessage = (
	message: TypesGen.ChatMessage,
): AgentContextUsage | null => {
	const withUsage = message as ChatMessageWithUsage;
	const inputTokens = asTokenCount(withUsage.input_tokens);
	const outputTokens = asTokenCount(withUsage.output_tokens);
	const reasoningTokens = asTokenCount(withUsage.reasoning_tokens);
	const cacheCreationTokens = asTokenCount(withUsage.cache_creation_tokens);
	const cacheReadTokens = asTokenCount(withUsage.cache_read_tokens);
	const contextLimitTokens = asTokenCount(withUsage.context_limit);

	const components = [
		inputTokens,
		outputTokens,
		cacheReadTokens,
		cacheCreationTokens,
		reasoningTokens,
	].filter((value): value is number => value !== undefined);
	const usedTokens =
		components.length > 0
			? components.reduce((total, value) => total + value, 0)
			: undefined;

	const hasUsage =
		usedTokens !== undefined ||
		contextLimitTokens !== undefined ||
		inputTokens !== undefined ||
		outputTokens !== undefined ||
		cacheReadTokens !== undefined ||
		cacheCreationTokens !== undefined ||
		reasoningTokens !== undefined;
	if (!hasUsage) {
		return null;
	}

	return {
		usedTokens,
		contextLimitTokens,
		inputTokens,
		outputTokens,
		cacheReadTokens,
		cacheCreationTokens,
		reasoningTokens,
	};
};

export const getLatestContextUsage = (
	messages: readonly TypesGen.ChatMessage[],
): AgentContextUsage | null => {
	for (let index = messages.length - 1; index >= 0; index -= 1) {
		const usage = extractContextUsageFromMessage(messages[index]);
		if (usage) {
			return usage;
		}
	}
	return null;
};

type ChatWithHierarchyMetadata = TypesGen.Chat & {
	readonly parent_chat_id?: string;
};

export const getParentChatID = (
	chat: TypesGen.Chat | undefined,
): string | undefined => {
	return asNonEmptyString(
		(chat as ChatWithHierarchyMetadata | undefined)?.parent_chat_id,
	);
};

export const resolveModelFromChatConfig = (
	modelConfig: TypesGen.Chat["model_config"],
	modelOptions: readonly ModelSelectorOption[],
): string => {
	if (modelOptions.length === 0) {
		return "";
	}

	if (!modelConfig || typeof modelConfig !== "object") {
		return modelOptions[0]?.id ?? "";
	}

	const typedModelConfig = modelConfig as Record<string, unknown>;
	const model = asString(typedModelConfig.model);
	const provider = asString(typedModelConfig.provider);

	const candidates = [model];
	if (provider && model) {
		candidates.push(`${provider}:${model}`);
	}

	for (const candidate of candidates) {
		const match = modelOptions.find((option) => option.id === candidate);
		if (match) {
			return match.id;
		}
	}

	if (model) {
		const modelMatch = modelOptions.find(
			(option) =>
				option.model === model && (!provider || option.provider === provider),
		);
		if (modelMatch) {
			return modelMatch.id;
		}
	}

	return modelOptions[0]?.id ?? "";
};

export const getWorkspaceAgent = (
	workspace: TypesGen.Workspace | undefined,
	workspaceAgentId: string | undefined,
): TypesGen.WorkspaceAgent | undefined => {
	if (!workspace) {
		return undefined;
	}
	const agents = workspace.latest_build.resources.flatMap(
		(resource) => resource.agents ?? [],
	);
	if (agents.length === 0) {
		return undefined;
	}
	return agents.find((agent) => agent.id === workspaceAgentId) ?? agents[0];
};
