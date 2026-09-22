import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import {
	chat as chatQuery,
	organizationChatModelOverrides,
	refreshChatContext,
	userCompactionThresholds,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type { AgentContextUsage } from "../components/AgentChatInput";
import { getLatestContextUsage } from "../components/ChatConversation/chatHelpers";
import {
	resolveCompactionContextLimit,
	resolveCompactionThreshold,
} from "../utils/modelOptions";

type UseChatContextOptions = {
	chat: TypesGen.Chat;
	messages: readonly TypesGen.ChatMessage[];
	models: readonly TypesGen.ChatModel[] | undefined;
	isReadOnly?: boolean;
	selectedModelId?: string;
};

/** Shares the observed pinned snapshot and compaction usage across chat surfaces. */
export const useChatContext = ({
	chat,
	messages,
	models,
	isReadOnly = false,
	selectedModelId,
}: UseChatContextOptions) => {
	const queryClient = useQueryClient();
	const { data: observedChat = chat } = useQuery({
		...chatQuery(chat.id),
		initialData: chat,
	});
	const thresholdsQuery = useQuery(userCompactionThresholds());
	const overridesQuery = useQuery(
		organizationChatModelOverrides(observedChat.organization_id),
	);
	const modelId =
		!observedChat.archived && !isReadOnly && selectedModelId
			? selectedModelId
			: observedChat.last_model_config_id;
	const model = models?.find((candidate) => candidate.id === modelId);
	const compactionOverride = overridesQuery.data?.overrides.find(
		(override) => override.context === "compaction",
	);
	const compactionModel = models?.find(
		(candidate) => candidate.id === compactionOverride?.model_config_id,
	);
	// An unavailable override catalog cannot establish the effective window.
	const contextLimit =
		model &&
		models &&
		overridesQuery.data &&
		(!compactionOverride || compactionModel)
			? resolveCompactionContextLimit(
					model,
					models,
					new Map(
						compactionOverride
							? [
									[
										observedChat.organization_id,
										compactionOverride.model_config_id,
									],
								]
							: [],
					),
				)
			: undefined;
	const contextLimitTokens =
		contextLimit && contextLimit > 0 ? contextLimit : undefined;
	const rawUsage = getLatestContextUsage(messages, contextLimitTokens);
	const compressionThreshold = resolveCompactionThreshold(
		modelId,
		thresholdsQuery.data?.thresholds,
		models,
	);
	const contextUsage: AgentContextUsage | null =
		rawUsage ||
		observedChat.context ||
		contextLimitTokens !== undefined ||
		compressionThreshold !== undefined
			? {
					...rawUsage,
					contextLimitTokens,
					compressionThreshold,
					context: observedChat.context,
				}
			: null;
	const applyMutation = useMutation(
		refreshChatContext(queryClient, observedChat.id),
	);
	const canApply =
		!observedChat.archived &&
		!isReadOnly &&
		Boolean(observedChat.context?.dirty || observedChat.context?.error);
	const onApplyContext = canApply
		? () => {
				if (applyMutation.isPending) {
					return;
				}
				applyMutation.mutate(undefined, {
					onSuccess: () => toast.success("Latest context applied."),
					onError: () => toast.error("Failed to apply latest context."),
				});
			}
		: undefined;

	return {
		observedChat,
		contextUsage,
		onApplyContext,
		isApplyingContext: applyMutation.isPending,
		applyContextError: applyMutation.error,
		applyContextSuccess: applyMutation.isSuccess,
	};
};
