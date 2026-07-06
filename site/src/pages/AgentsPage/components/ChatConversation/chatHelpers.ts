import type * as TypesGen from "#/api/typesGenerated";
import { getWorkspaceAgents } from "#/utils/workspace";
import type { AgentContextUsage } from "../AgentChatInput";
import type { ModelSelectorOption } from "../ChatElements";
import { asString } from "../ChatElements/runtimeTypeUtils";
import { asNonEmptyString } from "./blockUtils";

export const extractContextUsageFromMessage = (
	message: TypesGen.ChatMessage,
): AgentContextUsage | null => {
	const usage = message.usage;
	if (!usage) {
		return null;
	}

	const inputTokens = usage.input_tokens;
	const outputTokens = usage.output_tokens;
	const reasoningTokens = usage.reasoning_tokens;
	const cacheCreationTokens = usage.cache_creation_tokens;
	const cacheReadTokens = usage.cache_read_tokens;
	const contextLimitTokens = usage.context_limit;

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
	modelConfig: unknown,
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

	if (model) {
		const match = modelOptions.find((option) => option.id === model);
		if (match) {
			return match.id;
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
	const agents = getWorkspaceAgents(workspace);
	if (agents.length === 0) {
		return undefined;
	}
	return agents.find((agent) => agent.id === workspaceAgentId) ?? agents[0];
};
