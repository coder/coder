import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	chat as chatQuery,
	organizationChatModelOverrides,
	refreshChatContext,
	userCompactionThresholds,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { resolveChatCompactionThreshold } from "../compactionTriggers";
import type { AgentContextUsage } from "../components/AgentChatInput";
import { getLatestContextUsage } from "../components/ChatConversation/chatHelpers";
import { providerInfoByIDFromDescriptors } from "../utils/modelOptions";

type UseChatContextOptions = {
	chat: TypesGen.Chat;
	messages: readonly TypesGen.ChatMessage[];
	modelCatalog: TypesGen.OrganizationChatModelsResponse | undefined;
	isReadOnly?: boolean;
	selectedModelId?: string;
};

/** Shares the observed pinned snapshot and compaction usage across chat surfaces. */
export const useChatContext = ({
	chat,
	messages,
	modelCatalog,
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
	const models = modelCatalog?.models;
	const model = models?.find((candidate) => candidate.id === modelId);
	// Compaction measures usage against the chat model's own window; an
	// organization override only adds a second trigger point.
	const contextLimitTokens =
		model && Number.isFinite(model.context_limit) && model.context_limit > 0
			? model.context_limit
			: undefined;
	const rawUsage = getLatestContextUsage(messages, contextLimitTokens);
	// User settings belong to the viewer, but compaction uses the chat owner.
	// Wait for both settings so the label never shows a guessed trigger.
	const compactionThreshold =
		isReadOnly || !thresholdsQuery.data || overridesQuery.isPending
			? undefined
			: resolveChatCompactionThreshold(
					modelId,
					thresholdsQuery.data.thresholds,
					models,
					providerInfoByIDFromDescriptors(modelCatalog?.providers),
					overridesQuery,
				);
	const contextUsage: AgentContextUsage | null =
		rawUsage ||
		observedChat.context ||
		contextLimitTokens !== undefined ||
		compactionThreshold !== undefined
			? {
					...rawUsage,
					contextLimitTokens:
						rawUsage?.contextLimitTokens ?? contextLimitTokens,
					compactionThreshold,
					context: observedChat.context,
				}
			: null;
	const [applyResultSnapshot, setApplyResultSnapshot] = useState<{
		chatId: string;
		context: TypesGen.Chat["context"];
	}>();
	const applyMutation = useMutation(
		refreshChatContext(queryClient, observedChat.id),
	);
	// Query structural sharing preserves this reference while the snapshot is unchanged.
	const isApplyResultCurrent =
		applyResultSnapshot?.chatId === observedChat.id &&
		applyResultSnapshot.context === observedChat.context;
	const canApply =
		!observedChat.archived &&
		!isReadOnly &&
		Boolean(observedChat.context?.dirty || observedChat.context?.error);
	const onApplyContext = canApply
		? () => {
				if (applyMutation.isPending) {
					return;
				}
				setApplyResultSnapshot(undefined);
				applyMutation.mutate(undefined, {
					onSuccess: () => {
						setApplyResultSnapshot({
							chatId: observedChat.id,
							context: queryClient.getQueryData<TypesGen.Chat>(
								chatQuery(observedChat.id).queryKey,
							)?.context,
						});
						toast.success("Latest context applied.");
					},
					onError: () => {
						setApplyResultSnapshot({
							chatId: observedChat.id,
							context: observedChat.context,
						});
						toast.error("Failed to apply latest context.");
					},
				});
			}
		: undefined;

	return {
		observedChat,
		contextUsage,
		onApplyContext,
		isApplyingContext: applyMutation.isPending,
		applyContextError: isApplyResultCurrent ? applyMutation.error : null,
		applyContextSuccess: isApplyResultCurrent && applyMutation.isSuccess,
	};
};
